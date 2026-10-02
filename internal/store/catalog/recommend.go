package catalog

import (
	"fmt"
	"slices"
	"strings"
)

// Prefs are what the recommendation weighs, besides the offers themselves.
type Prefs struct {
	Language string         // the language the person wants; "" for any
	Trust    map[string]int // feed URL → -2 (less) … 2 (more); 0 when unset
	Blocked  map[string]int // feed name → downloads from it the safety checks blocked
}

// Recommendation is the offer to get, and why.
type Recommendation struct {
	Offer int      `json:"offer"` // index into Entry.Offers
	Why   []string `json:"why"`
}

// silent are installer types Seaglass installs without their questions.
var silent = map[string]bool{"inno": true, "nsis": true, "msi": true, "archive": true, "portable": true}

// Recommend picks the offer to get: the newest version, unless another
// one has the person's language, comes from a feed they trust more, or
// the newest one's feed had downloads blocked. Ties go to the newer, then
// the smaller download.
func Recommend(e Entry, p Prefs) Recommendation {
	if len(e.Offers) == 0 {
		return Recommendation{Offer: -1}
	}
	type scored struct {
		i     int
		score int
		why   []string
	}
	var all []scored
	for i, o := range e.Offers {
		s := scored{i: i}
		if CompareVersions(o.Version, e.Offers[0].Version) == 0 {
			s.score += 4
			if len(e.Offers) > 1 {
				s.why = append(s.why, "The newest version")
			}
		}
		if p.Language != "" && len(o.Languages) > 0 {
			if slices.ContainsFunc(o.Languages, func(l string) bool { return strings.EqualFold(l, p.Language) }) {
				s.score += 3
				s.why = append(s.why, "Has "+p.Language)
			} else {
				s.score -= 3
			}
		}
		switch t := p.Trust[o.FeedURL]; {
		case t > 0:
			s.score += 3 * t
			s.why = append(s.why, "From "+o.FeedName+", a feed you trust more")
		case t < 0:
			s.score += 3 * t
		}
		if n := min(p.Blocked[o.FeedName], 3); n > 0 {
			s.score -= 4 * n
		}
		if o.SHA256 != "" {
			s.score++
			s.why = append(s.why, "Can be checked against the feed's checksum")
		}
		if silent[o.InstallerType] {
			s.score++
			s.why = append(s.why, "Installs without the installer's questions")
		}
		all = append(all, s)
	}
	best := all[0]
	for _, s := range all[1:] {
		if s.score > best.score || (s.score == best.score && e.Offers[s.i].SizeBytes > 0 && e.Offers[s.i].SizeBytes < e.Offers[best.i].SizeBytes && CompareVersions(e.Offers[s.i].Version, e.Offers[best.i].Version) == 0) {
			best = s
		}
	}
	why := best.why
	if best.i != 0 {
		newest := e.Offers[0]
		var reasons []string
		if p.Language != "" && len(newest.Languages) > 0 && !slices.ContainsFunc(newest.Languages, func(l string) bool { return strings.EqualFold(l, p.Language) }) {
			reasons = append(reasons, "it doesn't have "+p.Language)
		}
		if p.Blocked[newest.FeedName] > 0 {
			reasons = append(reasons, fmt.Sprintf("downloads from %s were blocked by the safety checks", newest.FeedName))
		}
		if p.Trust[newest.FeedURL] < p.Trust[e.Offers[best.i].FeedURL] {
			reasons = append(reasons, newest.FeedName+" is a feed you trust less")
		}
		if len(reasons) > 0 {
			why = append(why, fmt.Sprintf("Not the newest (%s): %s", orDash(newest.Version), strings.Join(reasons, "; ")))
		}
	}
	if len(why) == 0 {
		why = []string{"The only version offered"}
	}
	return Recommendation{Offer: best.i, Why: why}
}

func orDash(s string) string {
	if s == "" {
		return "no version given"
	}
	return s
}
