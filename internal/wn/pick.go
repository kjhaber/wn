package wn

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// PickInteractive lets the user choose one item from the list. Returns the chosen item ID,
// or "" if cancelled. Uses fzf if available, otherwise a numbered list on stdin.
func PickInteractive(items []*Item) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	if _, err := exec.LookPath("fzf"); err == nil {
		return pickFzf(items)
	}
	return pickNumbered(items)
}

// PickMultiInteractive lets the user choose zero or more items (e.g. with fzf --multi).
// Returns selected item IDs, or nil if cancelled. Uses fzf if available, otherwise a numbered list.
func PickMultiInteractive(items []*Item) ([]string, error) {
	return pickMulti(items, nil)
}

// PickMultiInteractiveWithTags is like PickMultiInteractive but formats each line with the item's
// tags so the user can see which items have which tags (e.g. when toggling a tag with "wn tag -i").
func PickMultiInteractiveWithTags(items []*Item) ([]string, error) {
	return pickMulti(items, func(it *Item) string {
		if len(it.Tags) == 0 {
			return ""
		}
		return "  [" + strings.Join(it.Tags, ", ") + "]"
	})
}

func pickMulti(items []*Item, suffix func(*Item) string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if _, err := exec.LookPath("fzf"); err == nil {
		return pickMultiFzf(items, suffix)
	}
	return pickMultiNumbered(items, suffix)
}

func pickFzf(items []*Item) (string, error) {
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = fmt.Sprintf("%s: %s", it.ID, FirstLine(it.Description))
	}
	cmd := exec.Command("fzf", "--no-multi")
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		// fzf exits 1 when no selection (e.g. Esc)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return "", nil
	}
	before, _, _ := strings.Cut(line, ":")
	return strings.TrimSpace(before), nil
}

func pickNumbered(items []*Item) (string, error) {
	for i, it := range items {
		fmt.Printf("  %d. %s: %s\n", i+1, it.ID, FirstLine(it.Description))
	}
	fmt.Print("Number (or Enter to cancel): ")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return "", sc.Err()
	}
	text := strings.TrimSpace(sc.Text())
	if text == "" {
		return "", nil
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > len(items) {
		return "", fmt.Errorf("invalid choice")
	}
	return items[n-1].ID, nil
}

func pickMultiFzf(items []*Item, suffix func(*Item) string) ([]string, error) {
	lines := make([]string, len(items))
	for i, it := range items {
		line := fmt.Sprintf("%s: %s", it.ID, FirstLine(it.Description))
		if suffix != nil {
			line += suffix(it)
		}
		lines[i] = line
	}
	cmd := exec.Command("fzf", "--multi")
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}
	var ids []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		before, _, _ := strings.Cut(line, ":")
		ids = append(ids, strings.TrimSpace(before))
	}
	return ids, nil
}

func pickMultiNumbered(items []*Item, suffix func(*Item) string) ([]string, error) {
	for i, it := range items {
		line := fmt.Sprintf("  %d. %s: %s", i+1, it.ID, FirstLine(it.Description))
		if suffix != nil {
			line += suffix(it)
		}
		fmt.Println(line)
	}
	fmt.Print("Numbers separated by space (or Enter to cancel): ")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return nil, sc.Err()
	}
	text := strings.TrimSpace(sc.Text())
	if text == "" {
		return nil, nil
	}
	var ids []string
	for _, s := range strings.Fields(text) {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > len(items) {
			return nil, fmt.Errorf("invalid choice %q", s)
		}
		ids = append(ids, items[n-1].ID)
	}
	return ids, nil
}
