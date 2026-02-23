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

// ClaimNextItem atomically selects the next available item (by UndoneItems + TopoOrder),
// sets it as current under the meta lock, and claims it for the given duration.
// Returns the claimed item, or nil if the queue is empty. claimBy is optional (e.g. worker id).
func ClaimNextItem(store Store, root string, claimFor time.Duration, claimBy string) (*Item, error) {
	undone, err := UndoneItems(store)
	if err != nil {
		return nil, err
	}
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

// AgentOrchOpts configures the agent orchestrator loop.
type AgentOrchOpts struct {
	Root          string        // project root (contains .wn)
	ClaimFor      time.Duration // claim duration per item
	ClaimBy       string        // optional worker id
	Delay         time.Duration // delay between runs (after each item)
	Poll          time.Duration // poll interval when queue empty
	MaxTasks      int           // max tasks to process before exiting (0 = indefinite)
	AgentCmd      string        // command template, e.g. `cursor agent --print "{{.Prompt}}"`
	PromptTpl     string        // prompt template, e.g. "{{.Description}}"
	WorktreesBase string        // base path for worktrees
	LeaveWorktree bool          // if true, leave worktree after run; else remove
	DefaultBranch string        // override default branch (empty = detect)
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

// ExpandCommandTemplate executes the command template; prompt is the result of the prompt template.
func ExpandCommandTemplate(tpl string, prompt, itemID, worktree, branch string) (string, error) {
	data := struct {
		Prompt   string
		ItemID   string
		Worktree string
		Branch   string
	}{prompt, itemID, worktree, branch}
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

// resolveBranchName returns the branch name for the item: note "branch" if set, else wn-<id>-<slug>.
func resolveBranchName(item *Item) string {
	if idx := item.NoteIndexByName("branch"); idx >= 0 && strings.TrimSpace(item.Notes[idx].Body) != "" {
		return strings.TrimSpace(item.Notes[idx].Body)
	}
	slug := BranchSlug(item.Description)
	if slug == "" {
		return "wn-" + item.ID
	}
	return "wn-" + item.ID + "-" + slug
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

// RunAgentOrch runs the orchestrator loop until ctx is cancelled.
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
		worktreesBase = filepath.Join(opts.Root, ".wn", "worktrees")
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
	agentCmd := opts.AgentCmd
	if agentCmd == "" {
		return fmt.Errorf("agent_cmd is required")
	}

	processed := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		item, err := ClaimNextItem(store, opts.Root, opts.ClaimFor, opts.ClaimBy)
		if err != nil {
			return err
		}
		if item == nil {
			// Queue empty: wait then retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(opts.Poll):
			}
			continue
		}
		// One iteration: worktree, note, run agent, release, optional cleanup, delay
		branchName := resolveBranchName(item)
		reuseBranch := item.NoteIndexByName("branch") >= 0 && strings.TrimSpace(item.Notes[item.NoteIndexByName("branch")].Body) != ""
		createBranch := !reuseBranch

		worktreePath, err := EnsureWorktree(opts.Root, worktreesBase, branchName, createBranch, opts.Audit)
		if err != nil {
			_ = releaseItemClaim(store, item.ID)
			return fmt.Errorf("worktree %s: %w", branchName, err)
		}
		if err := addItemNote(store, item.ID, "branch", branchName); err != nil {
			_ = releaseItemClaim(store, item.ID)
			return fmt.Errorf("add branch note: %w", err)
		}
		prompt, err := ExpandPromptTemplate(promptTpl, item, worktreePath, branchName)
		if err != nil {
			_ = releaseItemClaim(store, item.ID)
			return fmt.Errorf("prompt template: %w", err)
		}
		expandedCmd, err := ExpandCommandTemplate(agentCmd, prompt, item.ID, worktreePath, branchName)
		if err != nil {
			_ = releaseItemClaim(store, item.ID)
			return fmt.Errorf("command template: %w", err)
		}
		auditLogAgent(opts.Audit, mainRoot, worktreePath, expandedCmd)
		cmd := exec.Command("sh", "-c", expandedCmd)
		cmd.Dir = worktreePath
		cmd.Env = append(os.Environ(), "WN_ROOT="+mainRoot)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run() // ignore exit code; we release claim either way
		_ = releaseItemClaim(store, item.ID)
		if !opts.LeaveWorktree {
			if err := RemoveWorktree(opts.Root, worktreePath, opts.Audit); err != nil {
				// Log but don't fail the loop
				if opts.Audit != nil {
					fmt.Fprintf(opts.Audit, "%s remove worktree failed: %v\n", time.Now().UTC().Format("2006-01-02 15:04:05"), err)
				}
			}
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
