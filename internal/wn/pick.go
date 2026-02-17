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
