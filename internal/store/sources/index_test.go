package sources

import "testing"

func TestGroupingPreservesVersionsAndSeparatesSequels(t *testing.T) {
	entries := []Entry{
		{ID: "one", Title: "Example Game", TitleKey: titleKey("Example Game"), Version: "v1.0"},
		{ID: "two", Title: "Example Game", TitleKey: titleKey("Example Game"), Version: "v2.0"},
		{ID: "three", Title: "Example Game 2", TitleKey: titleKey("Example Game 2")},
	}
	groups := GroupEntries(entries)
	if len(groups) != 2 || len(groups[0].EntryIDs) != 2 || !groups[0].NeedsReview {
		t.Fatalf("%+v", groups)
	}
}

func TestSearchRequiresAllWordsWithoutChangingEntryOrder(t *testing.T) {
	entries := []Entry{{ID: "one", RawTitle: "Example Game v1.0"}, {ID: "two", RawTitle: "Other Game"}, {ID: "three", RawTitle: "Example Game v2.0"}}
	got := Search(entries, "EXAMPLE game")
	if len(got) != 2 || got[0].ID != "one" || got[1].ID != "three" {
		t.Fatalf("%+v", got)
	}
	if len(Search(entries, "example missing")) != 0 {
		t.Fatal("partial query accepted")
	}
}
