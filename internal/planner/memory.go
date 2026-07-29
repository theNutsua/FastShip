package planner

import (
	"fmt"
	"strconv"
	"strings"
)

// parseMemory converts a human memory string like "512MB" or "1GB" into a
// byte count for the engine.
// The config uses human strings because "512MB" is friendlier to write
// than 536870912. The engine uses bytes because they are unambiguous.
// This is the one place that translation happens.
func parseMemory(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty memory value")
	}

	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "KB")
	}

	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q (try \"512MB\" or \"1GB\")", s)
	}

	return n * multiplier, nil
}
