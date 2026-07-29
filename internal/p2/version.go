package p2

import (
	"cmp"
	"strconv"
	"strings"
)

const qualifierPlaceholder = "qualifier"

type version struct {
	Major     int
	Minor     int
	Micro     int
	Qualifier string
}

// leadingNumber splits a segment into its numeric prefix and whatever follows,
// so a Maven-shaped "0-SNAPSHOT" micro still orders by its number rather than
// collapsing to zero.
func leadingNumber(segment string) (int, string) {
	end := 0
	for end < len(segment) && segment[end] >= '0' && segment[end] <= '9' {
		end++
	}
	value, _ := strconv.Atoi(segment[:end])
	return value, segment[end:]
}

func parseVersion(raw string) version {
	segments := strings.SplitN(strings.TrimSpace(raw), ".", 4)
	parsed := version{}

	numbers := []*int{&parsed.Major, &parsed.Minor, &parsed.Micro}
	for index, target := range numbers {
		if index >= len(segments) {
			return parsed
		}
		value, rest := leadingNumber(segments[index])
		*target = value
		if rest != "" {
			parsed.Qualifier = strings.Join(append([]string{rest}, segments[index+1:]...), ".")
			return parsed
		}
	}

	if len(segments) == 4 {
		parsed.Qualifier = segments[3]
	}
	return parsed
}

// CompareVersions orders OSGi versions: the three numeric segments numerically,
// then the qualifier bytewise, which is what Equinox's Version does.
func CompareVersions(a, b string) int {
	left, right := parseVersion(a), parseVersion(b)

	if result := cmp.Compare(left.Major, right.Major); result != 0 {
		return result
	}
	if result := cmp.Compare(left.Minor, right.Minor); result != 0 {
		return result
	}
	if result := cmp.Compare(left.Micro, right.Micro); result != 0 {
		return result
	}
	return strings.Compare(left.Qualifier, right.Qualifier)
}

// Prerelease has no meaning in p2 itself. It reports the two qualifiers that
// mark a build nobody meant to release: a Maven snapshot, and a Tycho
// placeholder that was never substituted.
func Prerelease(raw string) bool {
	qualifier := strings.ToLower(parseVersion(raw).Qualifier)
	return strings.Contains(qualifier, "snapshot") || qualifier == qualifierPlaceholder
}
