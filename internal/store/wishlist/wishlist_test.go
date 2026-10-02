package wishlist

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func obs(id, version string, backfill bool, ago time.Duration) Observation {
	return Observation{ReleaseID: id, Source: "fitgirl", SourceName: "FitGirl", Version: version, Backfill: backfill, PublishedAt: t0.Add(-ago)}
}

func TestSavedGamesSurviveARestart(t *testing.T) {
	p := filepath.Join(t.TempDir(), "wishlist.json")
	s := Open(p)
	if err := s.Add("steam:3100100", "Hollow Tide II", 3100100, nil, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("title:embercrown", "Ember Crown", 0, []Observation{obs("a", "v1.0", false, 0)}, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	s = Open(p)
	got := s.List()
	if len(got) != 2 || got[0].Key != "title:embercrown" || got[1].SteamAppID != 3100100 || got[0].Version != "v1.0" {
		t.Fatalf("after reopening: %+v", got)
	}
	if err := s.Remove("title:embercrown"); err != nil {
		t.Fatal(err)
	}
	if got := Open(p).List(); len(got) != 1 {
		t.Errorf("removed game came back: %+v", got)
	}
}

func TestGamesSavedAtTheSameInstantListNewestFirst(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	for _, key := range []string{"title:a", "title:b", "title:c"} {
		if err := s.Add(key, key, 0, nil, t0); err != nil {
			t.Fatal(err)
		}
	}
	got := s.List()
	if len(got) != 3 || got[0].Key != "title:c" || got[1].Key != "title:b" || got[2].Key != "title:a" {
		t.Fatalf("order: %+v", got)
	}
}

func TestSavingSetsABaselineAndOnlyLaterReleasesBecomeActivity(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	known := []Observation{obs("a", "v1.0", false, time.Hour), obs("b", "v1.1", false, 0)}
	_ = s.Add("k", "Game", 0, known, t0)
	if changed, _ := s.Observe("k", known, t0); changed {
		t.Error("the releases known when saving became activity")
	}
	// The same version from another source, and an article change, are not news.
	if changed, _ := s.Observe("k", append(known, obs("c", "v1.1", false, 0), obs("d", "", false, 0)), t0); !changed {
		t.Error("new releases weren't added to the baseline")
	}
	if e, _ := s.Get("k"); len(e.Activity) != 0 {
		t.Errorf("not a newer version, but: %+v", e.Activity)
	}
	_, _ = s.Observe("k", []Observation{obs("e", "v1.2", false, 0)}, t0)
	e, _ := s.Get("k")
	if len(e.Activity) != 1 || e.Activity[0].Kind != KindNewer || e.Activity[0].Version != "v1.2" || e.Version != "v1.2" {
		t.Fatalf("a confirmed newer version: %+v", e)
	}
	// Incomparable claims are never "newer".
	_, _ = s.Observe("k", []Observation{obs("f", "v1.3/v1.4", false, 0), obs("g", "Build 9999", false, 0)}, t0)
	if e, _ := s.Get("k"); len(e.Activity) != 1 {
		t.Errorf("incomparable versions made activity: %+v", e.Activity)
	}
}

func TestFirstReleaseOfASteamOnlyGameIsAnnounced(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	_ = s.Add("steam:1", "Lantern Season", 1, nil, t0)
	_, _ = s.Observe("steam:1", []Observation{obs("b", "v1.1", false, 0), obs("a", "v1.0", false, time.Hour)}, t0)
	e, _ := s.Get("steam:1")
	if len(e.Activity) != 2 || e.Activity[0].Kind != KindAvailable || e.Activity[0].ReleaseID != "a" || e.Activity[1].Kind != KindNewer {
		t.Fatalf("first availability: %+v", e.Activity)
	}
}

func TestBackfilledHistoryIsNeverNews(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	_ = s.Add("k", "Old Game", 0, nil, t0)
	var old []Observation
	for i, v := range []string{"v1.0", "v1.5", "v2.0"} {
		old = append(old, obs(string(rune('a'+i)), v, true, time.Duration(100-i)*24*time.Hour))
	}
	if changed, _ := s.Observe("k", old, t0); !changed {
		t.Error("backfilled releases didn't join the baseline")
	}
	e, _ := s.Get("k")
	if len(e.Activity) != 0 || e.Version != "v2.0" {
		t.Fatalf("backfill made activity: %+v", e)
	}
	_, _ = s.Observe("k", []Observation{obs("z", "v2.1", false, 0)}, t0)
	if e, _ := s.Get("k"); len(e.Activity) != 1 || e.Activity[0].Kind != KindNewer {
		t.Errorf("a real update after backfill: %+v", e.Activity)
	}
}

func TestActivityStaysUnreadUntilAcknowledged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "w.json")
	s := Open(p)
	_ = s.Add("a", "A", 0, nil, t0)
	_ = s.Add("b", "B", 0, nil, t0)
	_, _ = s.Observe("a", []Observation{obs("1", "v1", false, 0)}, t0)
	_, _ = s.Observe("b", []Observation{obs("2", "v1", false, 0)}, t0)
	s = Open(p)
	e, _ := s.Get("a")
	if _, unread := View(e); unread != 1 {
		t.Fatalf("unread after a restart: %d", unread)
	}
	_ = s.Acknowledge("a")
	ea, _ := s.Get("a")
	eb, _ := s.Get("b")
	if _, ua := View(ea); ua != 0 {
		t.Error("acknowledged activity is unread")
	}
	if _, ub := View(eb); ub != 1 {
		t.Error("acknowledging one game read another's")
	}
	_ = s.Acknowledge("")
	if eb, _ = Open(p).Get("b"); eb.Activity[0].Read != true {
		t.Error("mark all read didn't persist")
	}
}

func TestRekeyFollowsACorrectedGame(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "w.json"))
	_ = s.Add("title:embercrown", "Ember Crown", 0, []Observation{obs("a", "v1", false, 0)}, t0)
	_ = s.Rekey("title:embercrown", "steam:9", "Ember Crown", 9)
	if _, ok := s.Get("title:embercrown"); ok {
		t.Error("the old key is still saved")
	}
	if e, ok := s.Get("steam:9"); !ok || e.SteamAppID != 9 || len(e.Baseline) != 1 {
		t.Errorf("rekeyed: %+v", e)
	}
}

func TestUnreadableFileIsAnEmptyWishlist(t *testing.T) {
	p := filepath.Join(t.TempDir(), "w.json")
	if err := os.WriteFile(p, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Open(p).List(); len(got) != 0 {
		t.Errorf("%+v", got)
	}
}
