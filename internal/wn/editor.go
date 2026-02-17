package wn

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ErrEditorUnset = errors.New("EDITOR is not set; use -m to provide a message or set EDITOR for interactive edit")

// RunEditorOnFile runs $EDITOR with the given file path. Returns ErrEditorUnset if EDITOR is not set.
func RunEditorOnFile(path string) error {
	editor := os.Getenv("EDITOR")
	editor = strings.TrimSpace(editor)
	if editor == "" {
		return ErrEditorUnset
	}
	parts := splitEditorArgs(editor)
	args := append(parts[1:], path)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// EditWithEditor opens the user's $EDITOR with initial content and returns
// the edited content. Returns ErrEditorUnset if EDITOR is not set.
func EditWithEditor(initial string) (string, error) {
	editor := os.Getenv("EDITOR")
	editor = strings.TrimSpace(editor)
	if editor == "" {
		return "", ErrEditorUnset
	}
	f, err := os.CreateTemp("", "wn-edit-*.txt")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.WriteString(initial); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	f.Close()
	path, err := filepath.Abs(f.Name())
	if err != nil {
		return "", err
	}
	// EDITOR can be "vim" or "vim -f" or "code --wait"
	parts := splitEditorArgs(editor)
	bin := parts[0]
	args := append(parts[1:], path)
	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(data), "\n"), nil
}

func splitEditorArgs(editor string) []string {
	var parts []string
	var b strings.Builder
	quote := false
	for _, r := range editor {
		if r == '"' || r == '\'' {
			quote = !quote
			continue
		}
		if !quote && (r == ' ' || r == '\t') {
			if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}
