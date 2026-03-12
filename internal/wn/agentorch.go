package wn

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

const branchSlugMaxLen = 30

// BranchSlug returns a short branch-safe label from a description (e.g. first line).
// Lowercase, non-alphanumeric replaced with hyphen, consecutive hyphens collapsed, truncated to branchSlugMaxLen.
func BranchSlug(description string) string {
	line := FirstLine(description)
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return ""
	}
	// Replace non-alphanumeric with hyphen
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := re.ReplaceAllString(line, "-")
	slug = strings.Trim(slug, "-")
	// Collapse multiple hyphens (already done by ReplaceAllString)
	if len(slug) > branchSlugMaxLen {
		slug = slug[:branchSlugMaxLen]
		slug = strings.TrimRight(slug, "-")
	}
	return slug
}

// ClaimNextItem atomically selects the next available item (by UndoneItems, optional tag filter, then TopoOrder),
// sets it as current under the meta lock, and claims it for the given duration.
// Selection: dependencies are honored (prerequisites first); within each tier, Order field is the tiebreaker (lower = earlier).
// If tag is non-empty, only items that have that tag are considered. claimBy is optional (e.g. worker id).
// Returns the claimed item, or nil if the queue is empty.
func ClaimNextItem(store Store, root string, claimFor time.Duration, claimBy string, tag string) (*Item, error) {
	undone, err := UndoneItems(store)
	if err != nil {
		return nil, err
	}
	undone = FilterByTag(undone, tag)
	ordered, acyclic := TopoOrder(undone)
	if !acyclic || len(ordered) == 0 {
		return nil, nil
	}
	next := ordered[0]
	if err := WithMetaLock(root, func(m Meta) (Meta, error) {
		m.CurrentID = next.ID
		return m, nil
	}); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	until := now.Add(claimFor)
	err = store.UpdateItem(next.ID, func(it *Item) (*Item, error) {
		it.InProgressUntil = until
		it.InProgressBy = claimBy
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "in_progress", Msg: claimFor.String()})
		return it, nil
	})
	if err != nil {
		return nil, err
	}
	// Return updated item
	return store.Get(next.ID)
}

// ClaimItem claims the given item by id (sets current and InProgressUntil/InProgressBy).
// Use when running a specific item (e.g. --work-id or --current) instead of claiming next.
func ClaimItem(store Store, root string, itemID string, claimFor time.Duration, claimBy string) error {
	_, err := store.Get(itemID)
	if err != nil {
		return err
	}
	if err := WithMetaLock(root, func(m Meta) (Meta, error) {
		m.CurrentID = itemID
		return m, nil
	}); err != nil {
		return err
	}
	now := time.Now().UTC()
	until := now.Add(claimFor)
	return store.UpdateItem(itemID, func(it *Item) (*Item, error) {
		it.InProgressUntil = until
		it.InProgressBy = claimBy
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "in_progress", Msg: claimFor.String()})
		return it, nil
	})
}

// AgentOrchOpts configures the agent orchestrator loop.
type AgentOrchOpts struct {
	Root          string        // project root (contains .wn)
	ClaimFor      time.Duration // claim duration per item
	ClaimBy       string        // optional worker id
	Delay         time.Duration // delay between runs (after each item)
	Poll          time.Duration // poll interval when queue empty
	MaxTasks      int           // max tasks to process before exiting (0 = indefinite)
	WorkID        string        // if non-empty, run only this item then exit (use with --work-id or --current)
	AgentCmd      string        // command template, e.g. `cursor agent --print "{{.Prompt}}"`
	PromptTpl     string        // prompt template, e.g. "{{.Description}}"
	WorktreesBase string        // base path for worktrees
	LeaveWorktree bool          // if true, leave worktree after run; else remove
	DefaultBranch string        // override default branch (empty = detect)
	BranchPrefix  string        // prefix for generated branch names (e.g. "keith/"); not applied when reusing branch note
	Tag           string        // if non-empty, only consider items that have this tag
	FailIfEmpty   bool          // if true, return error immediately when queue is empty instead of polling
	Async         bool          // if true, dispatch cmd without waiting; skip commit/release (for wn launch)
	Audit         io.Writer     // timestamped command log (can be nil)
}

// PromptData is passed to the prompt template.
type PromptData struct {
	ItemID      string
	Description string
	FirstLine   string
	Worktree    string
	Branch      string
}

// ExpandPromptTemplate executes the prompt template with item and optional worktree/branch.
func ExpandPromptTemplate(tpl string, item *Item, worktree, branch string) (string, error) {
	if tpl == "" {
		return item.Description, nil
	}
	data := PromptData{
		ItemID:      item.ID,
		Description: item.Description,
		FirstLine:   FirstLine(item.Description),
		Worktree:    worktree,
		Branch:      branch,
	}
	tm, err := template.New("prompt").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tm.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// shellEscapeForDoubleQuoted escapes a string for safe embedding inside a
// double-quoted string in sh. Escapes \, ", `, and $ so the result can be used
// in templates like `cursor agent "{{.Prompt}}"` without breaking sh -c.
// Backticks and $ must be escaped to prevent command substitution and variable
// expansion (e.g. work item descriptions like `--wid <id>` or "cost $5").
func shellEscapeForDoubleQuoted(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "$", "\\$")
	return s
}

// shellEscapeForShWord wraps a string in single quotes and escapes internal
// single quotes as '\”, producing a single sh word that evaluates to the
// original string. Safe for ItemID, Worktree, Branch when used in templates
// that pass the result to sh -c.
func shellEscapeForShWord(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// resumeFlag returns "--resume <sessionID>" if sessionID is non-empty, else "".
func resumeFlag(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	return "--resume " + sessionID
}

// ExpandCommandTemplate executes the command template; prompt is the result of the prompt template.
// Prompt is escaped for double-quoted context; ItemID, Worktree, and Branch are escaped as
// single-quoted shell words so descriptions, imported IDs, and branch notes cannot inject
// commands when the result is passed to sh -c.
// sessionID is the Claude Code session ID for resume support (from the "claude-session" note);
// ResumeFlag is "--resume <sessionID>" if sessionID is non-empty, else "".
func ExpandCommandTemplate(tpl string, prompt, itemID, worktree, branch, sessionID string) (string, error) {
	escapedPrompt := shellEscapeForDoubleQuoted(prompt)
	data := struct {
		Prompt     string
		ItemID     string
		Worktree   string
		Branch     string
		ResumeFlag string
		SessionID  string
	}{escapedPrompt, shellEscapeForShWord(itemID), shellEscapeForShWord(worktree), shellEscapeForShWord(branch), resumeFlag(sessionID), sessionID}
	tm, err := template.New("cmd").Parse(tpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tm.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func auditLogAgent(w io.Writer, mainRoot, worktreePath, expandedCmd string) {
	if w == nil {
		return
	}
	ts := time.Now().UTC().Format("2006-01-02 15:04:05")
	// Truncate very long prompt in log
	cmdForLog := expandedCmd
	if len(cmdForLog) > 500 {
		cmdForLog = cmdForLog[:497] + "..."
	}
	fmt.Fprintf(w, "%s exec (Dir=%s WN_ROOT=%s): %s\n", ts, worktreePath, mainRoot, cmdForLog)
}

// worktreeDirForBranch returns the directory name for a worktree given the main
// worktree dirname and a branch name. Slashes in branchName (e.g. from a
// "keith/" prefix) are replaced with underscores so that filepath.Join does
// not create an unintended subdirectory level.
func worktreeDirForBranch(mainDirname, branchName string) string {
	return mainDirname + "-" + strings.ReplaceAll(branchName, "/", "_")
}

// resolveBranchName returns the branch name for the item: note "branch" if set, else prefix+wn-<id>-<slug>.
// branchPrefix is applied only when generating a new name (e.g. "keith/" -> "keith/wn-abc123-add-feature").
func resolveBranchName(item *Item, branchPrefix string) string {
	if idx := item.NoteIndexByName("branch"); idx >= 0 && strings.TrimSpace(item.Notes[idx].Body) != "" {
		return strings.TrimSpace(item.Notes[idx].Body)
	}
	slug := BranchSlug(item.Description)
	base := "wn-" + item.ID
	if slug != "" {
		base = base + "-" + slug
	}
	return branchPrefix + base
}

// addItemNote adds or updates a note by name on the item via the store.
func addItemNote(store Store, itemID, name, body string) error {
	now := time.Now().UTC()
	return store.UpdateItem(itemID, func(it *Item) (*Item, error) {
		if it.Notes == nil {
			it.Notes = []Note{}
		}
		idx := it.NoteIndexByName(name)
		body = strings.TrimSpace(body)
		if idx >= 0 {
			it.Notes[idx].Body = body
		} else {
			it.Notes = append(it.Notes, Note{Name: name, Created: now, Body: body})
		}
		it.Updated = now
		return it, nil
	})
}

// releaseItemClaim clears in-progress and sets review-ready (same as wn release).
func releaseItemClaim(store Store, itemID string) error {
	now := time.Now().UTC()
	return store.UpdateItem(itemID, func(it *Item) (*Item, error) {
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.ReviewReady = true
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "released"})
		return it, nil
	})
}

// clearItemClaim clears in-progress without setting review-ready (used when item is blocked post-run).
func clearItemClaim(store Store, itemID string) error {
	now := time.Now().UTC()
	return store.UpdateItem(itemID, func(it *Item) (*Item, error) {
		it.InProgressUntil = time.Time{}
		it.InProgressBy = ""
		it.Updated = now
		it.Log = append(it.Log, LogEntry{At: now, Kind: "claim_cleared"})
		return it, nil
	})
}

// FindItemByBranch searches all items for one whose "branch" note matches the given branch name.
// Returns (nil, nil) if no matching item is found.
func FindItemByBranch(store Store, branch string) (*Item, error) {
	all, err := store.List()
	if err != nil {
		return nil, err
	}
	for _, it := range all {
		idx := it.NoteIndexByName("branch")
		if idx >= 0 && strings.TrimSpace(it.Notes[idx].Body) == branch {
			return it, nil
		}
	}
	return nil, nil
}

// SetupItemWorktree creates the branch and worktree for item, records the branch note,
// and returns the resolved worktree path and branch name. On error the item claim is NOT
// released; caller is responsible for cleanup.
func SetupItemWorktree(store Store, root string, item *Item, worktreesBase, mainDirname, branchPrefix string, audit io.Writer) (worktreePath, branchName string, err error) {
	branchName = resolveBranchName(item, branchPrefix)
	reuseBranch := item.NoteIndexByName("branch") >= 0 && strings.TrimSpace(item.Notes[item.NoteIndexByName("branch")].Body) != ""
	createBranch := !reuseBranch
	if reuseBranch {
		exists, checkErr := BranchExists(root, branchName)
		if checkErr != nil {
			return "", "", fmt.Errorf("check branch %s: %w", branchName, checkErr)
		}
		if !exists {
			createBranch = true // branch was deleted (e.g. cleanup); recreate it
		}
	}
	worktreeDirName := worktreeDirForBranch(mainDirname, branchName)
	worktreePathArg := filepath.Join(worktreesBase, worktreeDirName)
	worktreePath, err = EnsureWorktree(root, worktreePathArg, branchName, createBranch, audit)
	if err != nil {
		return "", "", fmt.Errorf("worktree %s: %w", branchName, err)
	}
	if noteErr := addItemNote(store, item.ID, "branch", branchName); noteErr != nil {
		return "", "", fmt.Errorf("add branch note: %w", noteErr)
	}
	return worktreePath, branchName, nil
}

// itemSessionID returns the claude-session note body for the item, or "" if not set.
func itemSessionID(item *Item) string {
	idx := item.NoteIndexByName(NoteNameClaudeSession)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(item.Notes[idx].Body)
}

// runOneItem runs the full flow for one item: worktree, note, subagent, commit, release, optional remove worktree.
func runOneItem(store Store, opts AgentOrchOpts, item *Item, mainRoot, worktreesBase, mainDirname, promptTpl, agentCmd string) error {
	worktreePath, branchName, err := SetupItemWorktree(store, opts.Root, item, worktreesBase, mainDirname, opts.BranchPrefix, opts.Audit)
	if err != nil {
		_ = releaseItemClaim(store, item.ID)
		return err
	}
	sessionID := itemSessionID(item)
	prompt, err := ExpandPromptTemplate(promptTpl, item, worktreePath, branchName)
	if err != nil {
		_ = releaseItemClaim(store, item.ID)
		return fmt.Errorf("prompt template: %w", err)
	}
	expandedCmd, err := ExpandCommandTemplate(agentCmd, prompt, item.ID, worktreePath, branchName, sessionID)
	if err != nil {
		_ = releaseItemClaim(store, item.ID)
		return fmt.Errorf("command template: %w", err)
	}
	auditLogAgent(opts.Audit, mainRoot, worktreePath, expandedCmd)
	cmd := exec.Command("sh", "-c", expandedCmd)
	cmd.Dir = worktreePath
	cmd.Env = append(os.Environ(), "WN_ROOT="+mainRoot)

	if opts.Async {
		// Fire and forget: agent runs in another context (e.g. tmux window).
		// Item stays claimed; agent or user releases it later via MCP or wn release.
		_ = cmd.Start()
		return nil
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run() // ignore exit code; we release claim either way
	commitMsg := "wn " + item.ID + ": " + FirstLine(item.Description)
	if err := CommitWorktreeChanges(worktreePath, commitMsg, opts.Audit); err != nil {
		if opts.Audit != nil {
			fmt.Fprintf(opts.Audit, "%s commit worktree changes failed: %v\n", time.Now().UTC().Format("2006-01-02 15:04:05"), err)
		}
	}
	// Post-run: if item is now blocked (e.g. agent created prompt deps), clear claim only.
	// Otherwise release normally (sets review-ready).
	allItems, listErr := store.List()
	if listErr == nil && BlockedSet(allItems)[item.ID] {
		_ = clearItemClaim(store, item.ID)
	} else {
		_ = releaseItemClaim(store, item.ID)
	}
	if !opts.LeaveWorktree {
		if err := RemoveWorktree(opts.Root, worktreePath, opts.Audit); err != nil {
			if opts.Audit != nil {
				fmt.Fprintf(opts.Audit, "%s remove worktree failed: %v\n", time.Now().UTC().Format("2006-01-02 15:04:05"), err)
			}
		}
	}
	return nil
}

// RunAgentOrch runs the orchestrator loop until ctx is cancelled, or runs a single item and exits if opts.WorkID is set.
func RunAgentOrch(ctx context.Context, opts AgentOrchOpts) error {
	store, err := NewFileStore(opts.Root)
	if err != nil {
		return err
	}
	mainRoot, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}
	worktreesBase := opts.WorktreesBase
	if worktreesBase == "" {
		worktreesBase = filepath.Dir(opts.Root) // default: sibling of main worktree (peer dirs)
	}
	mainDirname := filepath.Base(opts.Root)
	agentCmd := opts.AgentCmd
	if agentCmd == "" {
		return fmt.Errorf("agent_cmd is required")
	}
	if opts.DefaultBranch == "" {
		if _, err = DefaultBranch(opts.Root); err != nil {
			return fmt.Errorf("default branch: %w", err)
		}
	}
	promptTpl := opts.PromptTpl
	if promptTpl == "" {
		promptTpl = "{{.Description}}"
	}

	// Single item mode: run one item then exit
	if opts.WorkID != "" {
		item, err := store.Get(opts.WorkID)
		if err != nil {
			return fmt.Errorf("work item %s: %w", opts.WorkID, err)
		}
		if item.Done {
			return fmt.Errorf("work item %s is already done", opts.WorkID)
		}
		if err := ClaimItem(store, opts.Root, item.ID, opts.ClaimFor, opts.ClaimBy); err != nil {
			return err
		}
		return runOneItem(store, opts, item, mainRoot, worktreesBase, mainDirname, promptTpl, agentCmd)
	}

	processed := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		item, err := ClaimNextItem(store, opts.Root, opts.ClaimFor, opts.ClaimBy, opts.Tag)
		if err != nil {
			return err
		}
		if item == nil {
			if opts.FailIfEmpty {
				return fmt.Errorf("no items in queue")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(opts.Poll):
			}
			continue
		}
		if err := runOneItem(store, opts, item, mainRoot, worktreesBase, mainDirname, promptTpl, agentCmd); err != nil {
			return err
		}
		processed++
		if opts.MaxTasks > 0 && processed >= opts.MaxTasks {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opts.Delay):
		}
	}
}
