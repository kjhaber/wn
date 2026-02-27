package wn

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var daysSuffixRe = regexp.MustCompile(`(\d+)[dD]`)

// ParseDurationWithDays parses duration strings like "5d", "2h", "15m",
// and combinations such as "2d6h". The "d" suffix is interpreted as 24h.
func ParseDurationWithDays(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("duration must not be empty")
	}
	// Expand day suffixes into hours so we can rely on time.ParseDuration.
	expanded := daysSuffixRe.ReplaceAllStringFunc(s, func(match string) string {
		m := daysSuffixRe.FindStringSubmatch(match)
		if len(m) != 2 {
			return match
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return match
		}
		return fmt.Sprintf("%dh", n*24)
	})
	d, err := time.ParseDuration(expanded)
	if err != nil {
		return 0, err
	}
	return d, nil
}
