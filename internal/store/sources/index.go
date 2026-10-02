package sources

import (
	"sort"
	"strings"
)

type Group struct {
	TitleKey    string   `json:"titleKey"`
	Title       string   `json:"title"`
	EntryIDs    []string `json:"entryIds"`
	NeedsReview bool     `json:"needsReview"`
}

// GroupEntries groups exact normalized titles only. This is a browsing hint, not an identity match.
func GroupEntries(entries []Entry) []Group {
	byKey := map[string]*Group{}
	for _, entry := range entries {
		key := entry.TitleKey
		if key == "" {
			key = entry.ID
		}
		group := byKey[key]
		if group == nil {
			group = &Group{TitleKey: key, Title: entry.Title, NeedsReview: true}
			byKey[key] = group
		}
		group.EntryIDs = append(group.EntryIDs, entry.ID)
	}
	groups := make([]Group, 0, len(byKey))
	for _, group := range byKey {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].TitleKey < groups[j].TitleKey })
	return groups
}

// Search keeps the input ordering and requires each query word to match the claimed title.
func Search(entries []Entry, query string) []Entry {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return entries
	}
	out := make([]Entry, 0)
	for _, entry := range entries {
		title := strings.ToLower(entry.RawTitle)
		match := true
		for _, word := range words {
			if !strings.Contains(title, word) {
				match = false
				break
			}
		}
		if match {
			out = append(out, entry)
		}
	}
	return out
}
