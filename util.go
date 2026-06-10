package main

import (
	"fmt"
	"strings"
	"time"
)

// shorten compresses a resource address to fit max display cells:
// module. → m., then middle-ellipsis if still too long.
func shorten(addr string, max int) string {
	if max < 8 {
		max = 8
	}
	s := strings.ReplaceAll(addr, "module.", "m.")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	head := max / 3
	tail := max - head - 1
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

func fmtDur(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

func truncRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
