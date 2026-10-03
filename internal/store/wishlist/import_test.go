package wishlist

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestImportMatchesSavedGamesBySteamAppIDAndRemovesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wishlist.json")
	s := Open(p)
	// Saved here before: one under its Steam key, one under a title key
	// with the same AppID, and one Steam doesn't list.
	_ = s.Add("steam:10", "Ember Crown", 10, nil, t0)
	_ = s.Add("title:hollowtide", "Hollow Tide", 20, nil, t0)
	_ = s.Add("title:quietorbit", "Quiet Orbit", 0, nil, t0)
	games := []Imported{{Key: "steam:10", Title: "Ember Crown", AppID: 10}, {Key: "steam:20", Title: "Hollow Tide", AppID: 20}, {Key: "steam:30", Title: Placeholder(30), AppID: 30}}
	added, existing, err := s.Import(games, t0.Add(time.Hour))
	if err != nil || added != 1 || existing != 2 {
		t.Fatalf("first import: added %d, existing %d, %v", added, existing, err)
	}
	// Importing again changes nothing and keeps the games Steam doesn't list.
	if added, existing, _ := s.Import(games[2:], t0.Add(2*time.Hour)); added != 0 || existing != 1 {
		t.Errorf("second import: added %d, existing %d", added, existing)
	}
	got := Open(p).List()
	if len(got) != 4 {
		t.Fatalf("after importing: %+v", got)
	}
	origins := map[string]string{}
	for _, e := range got {
		origins[e.Key] = e.Origin
	}
	if origins["steam:30"] != OriginSteam || origins["steam:10"] != OriginManual || origins["title:hollowtide"] != OriginManual || origins["title:quietorbit"] != OriginManual {
		t.Errorf("origins: %v", origins)
	}
}

func TestAFileWithoutOriginReadsAsSavedHere(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wishlist.json")
	old := `{"schema":1,"entries":[{"key":"steam:10","title":"Ember Crown","steamAppId":10,"addedAt":"2026-10-01T12:00:00Z","baseline":[],"activity":[]}]}`
	if err := os.WriteFile(p, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if e, ok := Open(p).Get("steam:10"); !ok || e.Origin != OriginManual {
		t.Errorf("an entry from an older file: %+v", e)
	}
}

func TestImportedGamesGetABaselineSoPublishedReleasesAreNotNews(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	known := []Observation{obs("a", "v1.0", false, 48*time.Hour)}
	if _, _, err := s.Import([]Imported{{Key: "steam:10", Title: "Ember Crown", AppID: 10, Known: known}, {Key: "steam:20", Title: "Northwind", AppID: 20}}, t0); err != nil {
		t.Fatal(err)
	}
	if changed, _ := s.Observe("steam:10", known, t0); changed {
		t.Error("a release known at import became activity")
	}
	// An idle search finds an old release of a game that had none: history.
	if _, _ = s.Observe("steam:20", []Observation{obs("old", "v0.9", true, 400*24*time.Hour)}, t0); len(mustGet(t, s, "steam:20").Activity) != 0 {
		t.Error("a release found in the source's history became activity")
	}
	// A newer version published later is news, as for any saved game.
	_, _ = s.Observe("steam:10", append(known, obs("b", "v1.1", false, 0)), t0)
	if e := mustGet(t, s, "steam:10"); len(e.Activity) != 1 || e.Activity[0].Kind != KindNewer {
		t.Errorf("after a newer release: %+v", e.Activity)
	}
}

func TestReimportKeepsUnreadActivity(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	_, _, _ = s.Import([]Imported{{Key: "steam:10", Title: "Ember Crown", AppID: 10}}, t0)
	_, _ = s.Observe("steam:10", []Observation{obs("a", "v1.0", false, 0)}, t0)
	if _, _, err := s.Import([]Imported{{Key: "steam:10", Title: "Ember Crown", AppID: 10}}, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, unread := View(mustGet(t, s, "steam:10")); unread != 1 {
		t.Errorf("unread after importing again: %d", unread)
	}
}

func TestOnlyAPlaceholderTitleIsReplacedByTheName(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	_, _, _ = s.Import([]Imported{{Key: "steam:30", Title: Placeholder(30), AppID: 30}, {Key: "steam:40", Title: "Dune Lark", AppID: 40}}, t0)
	if changed, _ := s.Name("steam:30", "Northwind Saga"); !changed || mustGet(t, s, "steam:30").Title != "Northwind Saga" {
		t.Error("the placeholder wasn't replaced")
	}
	if changed, _ := s.Name("steam:40", "Something Else"); changed || mustGet(t, s, "steam:40").Title != "Dune Lark" {
		t.Error("a known title was replaced")
	}
}

func TestImportStopsAtTheWishlistLimit(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	games := make([]Imported, MaxEntries+5)
	for i := range games {
		games[i] = Imported{Key: Placeholder(i + 1), Title: Placeholder(i + 1), AppID: i + 1}
	}
	added, _, err := s.Import(games, t0)
	if err != nil || added != MaxEntries || len(s.List()) != MaxEntries {
		t.Errorf("added %d of %d, %v", added, len(games), err)
	}
}

func mustGet(t *testing.T, s *Store, key string) Entry {
	t.Helper()
	e, ok := s.Get(key)
	if !ok {
		t.Fatalf("%s isn't saved", key)
	}
	return e
}
