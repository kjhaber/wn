package wn

import (
	"errors"
	"regexp"
	"unicode/utf8"
)

var (
	ErrTagEmpty   = errors.New("tag cannot be empty")
	ErrTagTooLong = errors.New("tag too long (max 32 characters)")
	ErrTagInvalid = errors.New("tag must be alphanumeric with only dash and underscore")
)

// TagMaxLen is the maximum length of a tag (32).
const TagMaxLen = 32

// TagPattern allows alphanumeric, dash, underscore.
var tagRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateTag returns an error if the tag is invalid.
func ValidateTag(tag string) error {
	if tag == "" {
		return ErrTagEmpty
	}
	if utf8.RuneCountInString(tag) > TagMaxLen {
		return ErrTagTooLong
	}
	if !tagRegex.MatchString(tag) {
		return ErrTagInvalid
	}
	return nil
}
