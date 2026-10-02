package catalog

import (
	"strconv"
	"strings"
	"unicode"
)

// CompareVersions orders version strings the way people write them:
// "v1.10" after "v1.9", "Build 15302" after "Build 9876", "2026-05-01"
// after "2026-04-30". Only the numbers count, read left to right, and
// what follows " +" or "(" is left out ("v1.2 + 3 DLCs"). It returns -1,
// 0 or 1; an empty or numberless version comes before any other.
func CompareVersions(a, b string) int {
	na, nb := numbers(a), numbers(b)
	for i := range min(len(na), len(nb)) {
		if na[i] != nb[i] {
			if na[i] < nb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(na) < len(nb):
		return -1
	case len(na) > len(nb):
		return 1
	}
	return 0
}

func numbers(v string) []uint64 {
	if i := strings.IndexAny(v, "+("); i >= 0 {
		v = v[:i]
	}
	var out []uint64
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return !unicode.IsDigit(r) }) {
		n, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			n = ^uint64(0) // too long to be anything but huge
		}
		out = append(out, n)
	}
	return out
}
