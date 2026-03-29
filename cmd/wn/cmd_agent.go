package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp [project_root]",
		Short: "Run MCP server on stdio (for Cursor and other MCP clients)",
		Long:  "Starts the Model Context Protocol server over stdin/stdout. Optional project_root is the directory containing .wn; when provided (or when WN_ROOT is set), the server is locked to that project and the per-request \"root\" parameter is ignored. No continuous process—exits when the client disconnects.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runMCP,
	}
}

func runMCP(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		wn.SetMCPFixedRoot(args[0])
	} else if r := os.Getenv("WN_ROOT"); r != "" {
		wn.SetMCPFixedRoot(r)
	}
	server := wn.NewMCPServer()
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		return err
	}
	return nil
}

func newSummaryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "summary",
		Short: "Show a summary dashboard: counts by status and tag",
		Long: `Show aggregate counts of work items by status and by tag.

Example output:
  status      count
  undone          12
  blocked          3
  review           2
  done            47

  tag              undone  blocked   review
  agent                 5        1        1
  backend               4        2        0
  (no tags)             3        0        1

Useful for a quick project health check without scrolling through all items.`,
		Args: cobra.NoArgs,
		RunE: runSummary,
	}
}

func runSummary(_ *cobra.Command, _ []string) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	allItems, err := cc.Store.List()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	blockedSet := wn.BlockedSet(allItems)

	statusCounts := make(map[string]int)
	for _, it := range allItems {
		status := wn.ItemListStatus(it, now, blockedSet[it.ID])
		statusCounts[status]++
	}
	statusOrder := []string{"undone", "blocked", "claimed", "prompt", "review", "done", "closed", "suspend"}
	fmt.Printf("  %-10s  %s\n", "status", "count")
	for _, s := range statusOrder {
		if n := statusCounts[s]; n > 0 {
			fmt.Printf("  %-10s  %d\n", s, n)
		}
	}

	type tagRow struct{ undone, blocked, review int }
	tagMap := make(map[string]*tagRow)
	var noTagRow tagRow
	for _, it := range allItems {
		status := wn.ItemListStatus(it, now, blockedSet[it.ID])
		if status != "undone" && status != "blocked" && status != "review" {
			continue
		}
		if len(it.Tags) == 0 {
			switch status {
			case "undone":
				noTagRow.undone++
			case "blocked":
				noTagRow.blocked++
			case "review":
				noTagRow.review++
			}
			continue
		}
		for _, tag := range it.Tags {
			if tagMap[tag] == nil {
				tagMap[tag] = &tagRow{}
			}
			switch status {
			case "undone":
				tagMap[tag].undone++
			case "blocked":
				tagMap[tag].blocked++
			case "review":
				tagMap[tag].review++
			}
		}
	}

	hasActive := len(tagMap) > 0 || noTagRow.undone+noTagRow.blocked+noTagRow.review > 0
	if !hasActive {
		return nil
	}

	tags := make([]string, 0, len(tagMap))
	tagColWidth := len("(no tags)")
	for t := range tagMap {
		tags = append(tags, t)
		if len(t) > tagColWidth {
			tagColWidth = len(t)
		}
	}
	sort.Strings(tags)

	fmt.Println()
	fmt.Printf("  %-*s  %7s  %7s  %7s\n", tagColWidth, "tag", "undone", "blocked", "review")
	for _, tag := range tags {
		tr := tagMap[tag]
		fmt.Printf("  %-*s  %7d  %7d  %7d\n", tagColWidth, tag, tr.undone, tr.blocked, tr.review)
	}
	if noTagRow.undone+noTagRow.blocked+noTagRow.review > 0 {
		fmt.Printf("  %-*s  %7d  %7d  %7d\n", tagColWidth, "(no tags)", noTagRow.undone, noTagRow.blocked, noTagRow.review)
	}
	return nil
}

type promptFlags struct {
	message string
}

func newPromptCmd() *cobra.Command {
	flags := &promptFlags{}
	cmd := &cobra.Command{
		Use:   "prompt [parent-id]",
		Short: "Create a prompt item (question for user) and add as dependency of parent",
		Long: `Creates a new prompt-state work item (a question for the user) and adds it as a
dependency of the parent item. The parent item becomes blocked until the user responds.

If parent-id is omitted, the current work item is used.
Use -m to provide the question inline, or $EDITOR will be opened.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrompt(cmd, args, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "Question text (or open $EDITOR if omitted)")
	return cmd
}

func runPrompt(_ *cobra.Command, args []string, f *promptFlags) error {
	msg := f.message
	if msg == "" {
		var err error
		msg, err = wn.EditWithEditor("")
		if err != nil {
			return err
		}
		if strings.TrimSpace(msg) == "" {
			return fmt.Errorf("empty question")
		}
	}
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	explicitID := ""
	if len(args) > 0 {
		explicitID = args[0]
	}
	parentID, err := wn.ResolveItemID(meta.CurrentID, explicitID)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	if _, err := cc.Store.Get(parentID); err != nil {
		return err
	}
	promptID, err := wn.GenerateID(cc.Store)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	promptItem := &wn.Item{
		ID:          promptID,
		Description: strings.TrimSpace(msg),
		Created:     now,
		Updated:     now,
		PromptReady: true,
		Log:         []wn.LogEntry{{At: now, Kind: "created"}, {At: now, Kind: "prompt_ready"}},
	}
	if err := cc.Store.Put(promptItem); err != nil {
		return err
	}
	items, err := cc.Store.List()
	if err != nil {
		return err
	}
	if wn.WouldCreateCycle(items, parentID, promptID) {
		_ = cc.Store.Delete(promptID)
		return fmt.Errorf("circular dependency would result")
	}
	if err := cc.Store.UpdateItem(parentID, func(it *wn.Item) (*wn.Item, error) {
		it.DependsOn = append(it.DependsOn, promptID)
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "depend_added", Msg: promptID})
		return it, nil
	}); err != nil {
		return err
	}
	fmt.Printf("created prompt %s; %s is now blocked\n", promptID, parentID)
	return nil
}

type respondFlags struct {
	message string
}

func newRespondCmd() *cobra.Command {
	flags := &respondFlags{}
	cmd := &cobra.Command{
		Use:   "respond [prompt-id]",
		Short: "Respond to a prompt item (marks it done and stores the response)",
		Long: `Marks a prompt-state work item as done and stores the response as a note.
This unblocks the parent item that was waiting for the response.

If prompt-id is omitted, the current work item is used.
Use -m to provide the answer inline, or $EDITOR will be opened.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRespond(cmd, args, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.message, "message", "m", "", "Response text (or open $EDITOR if omitted)")
	return cmd
}

func runRespond(_ *cobra.Command, args []string, f *respondFlags) error {
	cc, err := newCmdCtx("")
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(cc.Root)
	if err != nil {
		return err
	}
	explicitID := ""
	if len(args) > 0 {
		explicitID = args[0]
	}
	id, err := wn.ResolveItemID(meta.CurrentID, explicitID)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	item, err := cc.Store.Get(id)
	if err != nil {
		return err
	}
	if !item.PromptReady {
		return fmt.Errorf("item %s is not in prompt state", id)
	}
	msg := f.message
	if msg == "" {
		var err error
		msg, err = wn.EditWithEditor("")
		if err != nil {
			return err
		}
		if strings.TrimSpace(msg) == "" {
			return fmt.Errorf("empty response")
		}
	}
	now := time.Now().UTC()
	if err := cc.Store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		it.Done = true
		it.DoneStatus = wn.DoneStatusDone
		it.PromptReady = false
		it.Updated = now
		it.Log = append(it.Log, wn.LogEntry{At: now, Kind: "done", Msg: msg})
		if it.Notes == nil {
			it.Notes = []wn.Note{}
		}
		idx := it.NoteIndexByName(wn.NoteNameResponse)
		trimmed := strings.TrimSpace(msg)
		if idx >= 0 {
			it.Notes[idx].Body = trimmed
		} else {
			it.Notes = append(it.Notes, wn.Note{Name: wn.NoteNameResponse, Created: now, Body: trimmed})
		}
		return it, nil
	}); err != nil {
		return err
	}
	fmt.Printf("responded to %s; prompt marked done\n", id)
	return nil
}
