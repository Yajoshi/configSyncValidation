package monitor

import "strings"

// The helpers below read the loosely typed maps that Kubernetes objects are
// decoded into. They never mutate their input and never panic on odd shapes.

func lookup(m map[string]interface{}, path ...string) (interface{}, bool) {
	var cur interface{} = m
	for _, p := range path {
		mm, ok := cur.(map[string]interface{})
		if !ok {
			return nil, false
		}
		cur, ok = mm[p]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func str(m map[string]interface{}, path ...string) string {
	v, _ := lookup(m, path...)
	s, _ := v.(string)
	return s
}

func list(m map[string]interface{}, path ...string) []interface{} {
	v, _ := lookup(m, path...)
	l, _ := v.([]interface{})
	return l
}

func asInt(m map[string]interface{}, path ...string) (int64, bool) {
	v, ok := lookup(m, path...)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}

func asMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

// oneLine collapses whitespace and truncates to max runes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-3]) + "..."
	}
	return s
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

func commitMatches(have, want string) bool {
	if have == "" || want == "" {
		return false
	}
	return strings.HasPrefix(have, want) || strings.HasPrefix(want, have)
}
