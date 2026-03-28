package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kjhaber/wn/internal/wn"
	"github.com/spf13/cobra"
)

// --- note command and subcommands add, list, edit, rm ---

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Add, list, edit, remove, show, or search notes (attachments) on a work item",
	Long:  "Notes attach text by logical name (e.g. pr-url, issue-number). Use 'wn note add <name> [id] -m \"...\"', 'wn note list [id]', 'wn note show [id] <name>', 'wn note edit [id] <name> -m \"...\"', 'wn note rm [id] <name>', and 'wn note search <name> [value]'. Names are alphanumeric, slash, underscore, or hyphen, up to 32 chars.",
}

var noteAddCmd = &cobra.Command{
	Use:   "add <name> [id]",
	Short: "Add or update a note by name on a work item",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runNoteAdd,
}
var noteAddMessage string

func init() {
	noteAddCmd.Flags().StringVarP(&noteAddMessage, "message", "m", "", "Note text (or open $EDITOR if omitted)")
	noteCmd.AddCommand(noteAddCmd, noteListCmd, noteShowCmd, noteEditCmd, noteRmCmd, noteSearchCmd)
}

var noteSearchCmd = &cobra.Command{
	Use:   "search <name> [value]",
	Short: "Search all work items for those having a note with the given name (and optionally value)",
	Long: `Search all work items for those that have a note named <name>.
If <value> is given, only items where the note body exactly matches <value> are returned.

Each matching item is printed as: <id>  <first line of description>

Flags:
  --first   Return only the oldest matching item (by created time).
  --latest  Return only the most recently updated matching item.
`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runNoteSearch,
}
var noteSearchFirst bool
var noteSearchLatest bool
var noteSearchIDOnly bool

func init() {
	noteSearchCmd.Flags().BoolVar(&noteSearchFirst, "first", false, "Return only the oldest matching item (by created time)")
	noteSearchCmd.Flags().BoolVar(&noteSearchLatest, "latest", false, "Return only the most recently updated matching item")
	noteSearchCmd.Flags().BoolVar(&noteSearchIDOnly, "id-only", false, "Print only the item ID(s), one per line")
}

func runNoteSearch(cmd *cobra.Command, args []string) error {
	if noteSearchFirst && noteSearchLatest {
		return fmt.Errorf("--first and --latest are mutually exclusive")
	}
	name := args[0]
	value := ""
	if len(args) > 1 {
		value = args[1]
	}
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	items, err := store.List()
	if err != nil {
		return err
	}

	var matches []*wn.Item
	for _, it := range items {
		idx := it.NoteIndexByName(name)
		if idx < 0 {
			continue
		}
		if value != "" && it.Notes[idx].Body != value {
			continue
		}
		matches = append(matches, it)
	}

	if len(matches) == 0 {
		return fmt.Errorf("no items found with note %q", name)
	}

	if noteSearchFirst {
		oldest := matches[0]
		for _, it := range matches[1:] {
			if it.Created.Before(oldest.Created) {
				oldest = it
			}
		}
		matches = []*wn.Item{oldest}
	} else if noteSearchLatest {
		newest := matches[0]
		for _, it := range matches[1:] {
			if it.Updated.After(newest.Updated) {
				newest = it
			}
		}
		matches = []*wn.Item{newest}
	}

	for _, it := range matches {
		if noteSearchIDOnly {
			fmt.Println(it.ID)
			continue
		}
		firstLine := it.Description
		if idx := strings.IndexByte(firstLine, '\n'); idx >= 0 {
			firstLine = firstLine[:idx]
		}
		fmt.Printf("%s  %s\n", it.ID, firstLine)
	}
	return nil
}

func runNoteAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	if !wn.ValidNoteName(name) {
		return fmt.Errorf("invalid note name %q (alphanumeric, slash, underscore, hyphen, 1-32 chars; or wn:<name> for special notes)", name)
	}
	if err := wn.ValidateSpecialNote(name); err != nil {
		return err
	}
	body := noteAddMessage
	if body == "" {
		switch name {
		case wn.NoteNameBranch:
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("wn:branch: %w", err)
			}
			branch, err := wn.CurrentBranchInDir(cwd)
			if err != nil {
				return fmt.Errorf("wn:branch: could not detect current git branch: %w", err)
			}
			body = branch
		case wn.NoteNameCommit:
			return fmt.Errorf("wn:commit requires a commit hash (use -m <hash>)")
		default:
			var err error
			body, err = wn.EditWithEditor("")
			if err != nil {
				return err
			}
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("empty note")
			}
		}
	}
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	if name == wn.NoteNameCommit {
		exists, commitErr := wn.CommitExists(root, strings.TrimSpace(body))
		if commitErr != nil {
			return fmt.Errorf("wn:commit: %w", commitErr)
		}
		if !exists {
			return fmt.Errorf("wn:commit: commit %q not found in this repository", strings.TrimSpace(body))
		}
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	explicitID := ""
	if len(args) > 1 {
		explicitID = args[1]
	}
	id, err := wn.ResolveItemID(meta.CurrentID, explicitID)
	if err != nil {
		return fmt.Errorf("no id provided and no current task")
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		if it.Notes == nil {
			it.Notes = []wn.Note{}
		}
		idx := it.NoteIndexByName(name)
		trimmed := strings.TrimSpace(body)
		if idx >= 0 {
			it.Notes[idx].Body = trimmed
		} else {
			it.Notes = append(it.Notes, wn.Note{Name: name, Created: now, Body: trimmed})
		}
		it.Updated = now
		return it, nil
	})
}

var noteShowCmd = &cobra.Command{
	Use:   "show [id] <name>",
	Short: "Print the body of a named note",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runNoteShow,
}

func runNoteShow(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	var id string
	var nameArg string
	if len(args) == 2 {
		id, nameArg = args[0], args[1]
	} else {
		id, err = wn.ResolveItemID(meta.CurrentID, "")
		if err != nil {
			return fmt.Errorf("no id provided and no current task")
		}
		nameArg = args[0]
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	item, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	idx := item.NoteIndexByName(nameArg)
	if idx < 0 {
		return fmt.Errorf("no note named %q", nameArg)
	}
	fmt.Println(item.Notes[idx].Body)
	return nil
}

var noteListCmd = &cobra.Command{
	Use:   "list [id]",
	Short: "List notes on a work item (ordered by create time)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runNoteList,
}

func runNoteList(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
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
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	item, err := store.Get(id)
	if err != nil {
		return fmt.Errorf("item %s not found", id)
	}
	for _, n := range item.Notes {
		fmt.Printf("%s\t%s\t%s\n", n.Name, n.Created.Format("2006-01-02 15:04:05"), n.Body)
	}
	return nil
}

var noteEditCmd = &cobra.Command{
	Use:   "edit [id] <name>",
	Short: "Edit a note by name",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runNoteEdit,
}
var noteEditMessage string

func init() {
	noteEditCmd.Flags().StringVarP(&noteEditMessage, "message", "m", "", "New note text (or open $EDITOR with current body if omitted)")
}

func runNoteEdit(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	var id string
	nameArg := ""
	if len(args) == 2 {
		id, nameArg = args[0], args[1]
	} else {
		id, err = wn.ResolveItemID(meta.CurrentID, "")
		if err != nil {
			return fmt.Errorf("no id provided and no current task")
		}
		nameArg = args[0]
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	body := noteEditMessage
	if body == "" {
		item, err := store.Get(id)
		if err != nil {
			return fmt.Errorf("item %s not found", id)
		}
		idx := item.NoteIndexByName(nameArg)
		if idx < 0 {
			return fmt.Errorf("no note named %q", nameArg)
		}
		var errEdit error
		body, errEdit = wn.EditWithEditor(item.Notes[idx].Body)
		if errEdit != nil {
			return errEdit
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("empty note")
		}
		body = strings.TrimSpace(body)
	} else {
		body = strings.TrimSpace(body)
	}
	now := time.Now().UTC()
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		idx := it.NoteIndexByName(nameArg)
		if idx < 0 {
			return nil, fmt.Errorf("no note named %q", nameArg)
		}
		it.Notes[idx].Body = body
		it.Updated = now
		return it, nil
	})
}

var noteRmCmd = &cobra.Command{
	Use:   "rm [id] <name>",
	Short: "Remove a note by name",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runNoteRm,
}

func runNoteRm(cmd *cobra.Command, args []string) error {
	root, err := wn.FindRootForCLI()
	if err != nil {
		return err
	}
	meta, err := wn.ReadMeta(root)
	if err != nil {
		return err
	}
	var id string
	nameArg := ""
	if len(args) == 2 {
		id, nameArg = args[0], args[1]
	} else {
		id, err = wn.ResolveItemID(meta.CurrentID, "")
		if err != nil {
			return fmt.Errorf("no id provided and no current task")
		}
		nameArg = args[0]
	}
	store, err := wn.NewFileStore(root)
	if err != nil {
		return err
	}
	return store.UpdateItem(id, func(it *wn.Item) (*wn.Item, error) {
		idx := it.NoteIndexByName(nameArg)
		if idx < 0 {
			return nil, fmt.Errorf("no note named %q", nameArg)
		}
		it.Notes = append(it.Notes[:idx], it.Notes[idx+1:]...)
		it.Updated = time.Now().UTC()
		return it, nil
	})
}
