package catalog

import (
	"cmp"
	"regexp"
	"strings"
	"time"
)

var versionClaim = regexp.MustCompile(`(?i)^(?:v|version\s*)?([0-9]+(?:\.[0-9]+)*)(?:\s*[,;-]?\s*build\s*([0-9]+))?$`)
var buildClaim = regexp.MustCompile(`(?i)^build\s*([0-9]+)$`)

type releaseClaim struct {
	family  string
	numbers []string
	build   string
}

func claim(s string) releaseClaim {
	s = strings.TrimSpace(s)
	// Source labels, DLC counts and repack revisions are not game versions.
	if strings.Contains(s, "/") {
		return releaseClaim{}
	}
	if i := strings.IndexAny(s, "+("); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return releaseClaim{family: "date", numbers: strings.Split(s, "-")}
	}
	if m := buildClaim.FindStringSubmatch(s); m != nil {
		return releaseClaim{family: "build", numbers: []string{m[1]}}
	}
	if m := versionClaim.FindStringSubmatch(s); m != nil {
		n := strings.Split(m[1], ".")
		for len(n) > 1 && numeric(n[len(n)-1]) == "0" {
			n = n[:len(n)-1]
		}
		return releaseClaim{family: "version", numbers: n, build: m[2]}
	}
	return releaseClaim{}
}

func numeric(s string) string {
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	return s
}

// CompareReleases only orders claims expressed in the same version system.
// A repack date, a build number and a game version cannot stand in for each other.
func CompareReleases(a, b string) (int, bool) {
	x, y := claim(a), claim(b)
	if x.family == "" || x.family != y.family {
		return 0, false
	}
	order := compareNumbers(x.numbers, y.numbers)
	if x.family != "version" {
		return order, true
	}
	if order != 0 {
		if x.build != "" && y.build != "" {
			buildOrder := compareNumbers([]string{x.build}, []string{y.build})
			if buildOrder != 0 && (buildOrder < 0) != (order < 0) {
				return 0, false
			}
		}
		return order, true
	}
	if (x.build == "") != (y.build == "") {
		return 0, false
	}
	if x.build != "" {
		return compareNumbers([]string{x.build}, []string{y.build}), true
	}

	return 0, true
}

// CompareVersions is a deterministic display order. Use CompareReleases for updates.
func CompareVersions(a, b string) int {
	x, y := claim(a), claim(b)
	if x.family == "" && y.family == "" {
		return 0
	}
	if x.family == "" {
		return -1
	}
	if y.family == "" {
		return 1
	}
	if x.family != y.family {
		return strings.Compare(x.family, y.family)
	}
	if d := compareNumbers(x.numbers, y.numbers); d != 0 {
		return d
	}
	return compareNumbers([]string{x.build}, []string{y.build})
}

func compareNumbers(a, b []string) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		u, v := "0", "0"
		if i < len(a) {
			u = numeric(a[i])
		}
		if i < len(b) {
			v = numeric(b[i])
		}
		if d := cmp.Or(cmp.Compare(len(u), len(v)), strings.Compare(u, v)); d != 0 {
			return d
		}
	}
	return 0
}
