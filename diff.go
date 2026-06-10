package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// A single attribute-level diff line inside an expanded resource node.
type diffLine struct {
	sym    byte // '+', '-', '~'
	text   string
	forces bool // attribute forces replacement
}

const maxDiffValueLen = 100
const maxDiffLines = 400

// flatten walks an arbitrary decoded-JSON value, producing dotted leaf paths.
func flatten(prefix string, v any, out map[string]any) {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 {
			out[prefix] = t
			return
		}
		for k, vv := range t {
			flatten(joinPath(prefix, k), vv, out)
		}
	case []any:
		if len(t) == 0 {
			out[prefix] = t
			return
		}
		for i, vv := range t {
			flatten(fmt.Sprintf("%s[%d]", prefix, i), vv, out)
		}
	default:
		out[prefix] = v
	}
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// truePaths collects paths whose leaf value is `true` (used for
// after_unknown and *_sensitive structures).
func truePaths(v any) map[string]bool {
	flat := map[string]any{}
	flatten("", v, flat)
	out := map[string]bool{}
	for k, vv := range flat {
		if b, ok := vv.(bool); ok && b {
			out[k] = true
		}
	}
	return out
}

func renderVal(v any) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	s := string(b)
	s = strings.ReplaceAll(s, "\\n", "↵")
	if len(s) > maxDiffValueLen {
		s = s[:maxDiffValueLen-1] + "…"
	}
	return s
}

func replacePathPrefixes(rps [][]any) []string {
	var out []string
	for _, rp := range rps {
		var parts []string
		for _, step := range rp {
			switch s := step.(type) {
			case string:
				parts = append(parts, s)
			case float64:
				if len(parts) > 0 {
					parts[len(parts)-1] = fmt.Sprintf("%s[%d]", parts[len(parts)-1], int(s))
				}
			}
		}
		out = append(out, strings.Join(parts, "."))
	}
	return out
}

// diffResource computes attribute-level diff lines for a resource change.
func diffResource(ch *ChangeRepr) []diffLine {
	before := map[string]any{}
	after := map[string]any{}
	flatten("", ch.Before, before)
	flatten("", ch.After, after)
	unknown := truePaths(ch.AfterUnknown)
	sensB := truePaths(ch.BeforeSensitive)
	sensA := truePaths(ch.AfterSensitive)
	forces := replacePathPrefixes(ch.ReplacePaths)

	keySet := map[string]bool{}
	for k := range before {
		keySet[k] = true
	}
	for k := range after {
		keySet[k] = true
	}
	for k := range unknown {
		keySet[k] = true
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	forcesRepl := func(path string) bool {
		for _, p := range forces {
			if p != "" && (path == p || strings.HasPrefix(path, p+".") || strings.HasPrefix(path, p+"[")) {
				return true
			}
		}
		return false
	}

	var lines []diffLine
	for _, k := range keys {
		if len(lines) >= maxDiffLines {
			lines = append(lines, diffLine{sym: '~', text: "… (diff truncated)"})
			break
		}
		b, hasB := before[k]
		a, hasA := after[k]
		unk := unknown[k]

		oldStr := renderVal(b)
		newStr := renderVal(a)
		if sensB[k] {
			oldStr = "(sensitive)"
		}
		if sensA[k] {
			newStr = "(sensitive)"
		}
		if unk {
			newStr = "(known after apply)"
		}

		switch {
		case hasB && (hasA || unk):
			if !unk && reflect.DeepEqual(a, b) {
				continue // unchanged
			}
			if sensB[k] && sensA[k] && reflect.DeepEqual(a, b) {
				continue
			}
			lines = append(lines, diffLine{sym: '~', text: fmt.Sprintf("%s: %s → %s", k, oldStr, newStr), forces: forcesRepl(k)})
		case !hasB && (hasA || unk):
			lines = append(lines, diffLine{sym: '+', text: fmt.Sprintf("%s: %s", k, newStr)})
		case hasB:
			lines = append(lines, diffLine{sym: '-', text: fmt.Sprintf("%s: %s", k, oldStr)})
		}
	}
	return lines
}
