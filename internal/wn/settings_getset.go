package wn

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SettingsGetValue reads a dot-notation key from the effective merged settings and returns its value as a string.
// For string values, the raw string is returned (no JSON quotes). For other types (bool, number, object, array),
// the value is returned as compact JSON.
func SettingsGetValue(s Settings, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("key must not be empty")
	}

	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}

	val, err := navigateGet(m, strings.Split(key, "."))
	if err != nil {
		return "", fmt.Errorf("key %q not found: %w", key, err)
	}

	// Return raw string for string values; JSON encoding for everything else.
	if s, ok := val.(string); ok {
		return s, nil
	}
	out, err := json.Marshal(val)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func navigateGet(m map[string]any, parts []string) (any, error) {
	key := parts[0]
	val, ok := m[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	if len(parts) == 1 {
		return val, nil
	}
	nested, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("key %q is not an object", key)
	}
	return navigateGet(nested, parts[1:])
}

// SettingsSetValue sets a dot-notation key in the settings file at path.
// The value string is parsed as JSON if it is valid JSON (e.g. "true", "42", "{...}");
// otherwise it is stored as a plain string.
// If the file does not exist, it is created. Parent directories are created as needed.
func SettingsSetValue(path string, key string, value string) error {
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}

	var m map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		m = make(map[string]any)
	} else {
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}

	// Parse the value: try JSON first; fall back to treating as plain string.
	var parsedValue any
	if err := json.Unmarshal([]byte(value), &parsedValue); err != nil {
		parsedValue = value
	}

	navigateSet(m, strings.Split(key, "."), parsedValue)

	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0644)
}

func navigateSet(m map[string]any, parts []string, value any) {
	key := parts[0]
	if len(parts) == 1 {
		m[key] = value
		return
	}
	nested, ok := m[key].(map[string]any)
	if !ok {
		nested = make(map[string]any)
		m[key] = nested
	}
	navigateSet(nested, parts[1:], value)
}
