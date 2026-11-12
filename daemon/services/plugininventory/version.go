package plugininventory

import (
	"cmp"
	"strconv"
	"strings"
)

// compareOSVersions follows PHP version_compare for Unraid version bounds.
// Plugin update versions use bytewise comparison independently of these bounds.
func compareOSVersions(a, b string) int {
	if a == "" || b == "" {
		return cmp.Compare(a, b)
	}
	left, right := versionParts(a), versionParts(b)
	for i := range min(len(left), len(right)) {
		if order := compareVersionParts(left[i], right[i]); order != 0 {
			return order
		}
	}
	if len(left) > len(right) {
		return compareVersionTail(left[len(right):])
	}
	if len(right) > len(left) {
		return -compareVersionTail(right[len(left):])
	}
	return 0
}

func versionParts(version string) []string {
	if strings.HasPrefix(version, "#") {
		return strings.Split(version, ".")
	}
	var parts []string
	start := 0
	for i := range len(version) {
		c := version[i]
		if !digit(c) && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			if start < i {
				parts = append(parts, version[start:i])
			}
			start = i + 1
		} else if i > start && digit(c) != digit(version[i-1]) {
			parts = append(parts, version[start:i])
			start = i
		}
	}
	if start < len(version) {
		parts = append(parts, version[start:])
	}
	return parts
}

func digit(c byte) bool { return c >= '0' && c <= '9' }

func compareVersionParts(a, b string) int {
	if a != "" && b != "" && digit(a[0]) && digit(b[0]) {
		x, _ := strconv.ParseInt(a, 10, 64)
		y, _ := strconv.ParseInt(b, 10, 64)
		return cmp.Compare(x, y)
	}
	return cmp.Compare(versionPartRank(a), versionPartRank(b))
}

func compareVersionTail(parts []string) int {
	for _, part := range parts {
		if part != "" && digit(part[0]) {
			return 1
		}
		if order := compareVersionParts(part, "#"); order != 0 {
			return order
		}
	}
	return 0
}

func versionPartRank(part string) int {
	switch {
	case part != "" && digit(part[0]), strings.HasPrefix(part, "#"):
		return 4
	case strings.HasPrefix(part, "dev"):
		return 0
	case strings.HasPrefix(part, "a"):
		return 1
	case strings.HasPrefix(part, "b"):
		return 2
	case strings.HasPrefix(part, "RC"), strings.HasPrefix(part, "rc"):
		return 3
	case strings.HasPrefix(part, "p"):
		return 5
	default:
		return -1
	}
}
