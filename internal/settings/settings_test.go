package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A new install sees the welcome; someone updating from before it
// existed doesn't.
func TestWelcomed(t *testing.T) {
	dir := t.TempDir()
	if Open(filepath.Join(dir, "none.json")).Get().Welcomed {
		t.Error("a first start is welcomed already")
	}
	old := filepath.Join(dir, "old.json")
	if err := os.WriteFile(old, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Open(old).Get().Welcomed {
		t.Error("settings from before the welcome should count as welcomed")
	}
	fresh := filepath.Join(dir, "fresh.json")
	if err := os.WriteFile(fresh, []byte(`{"theme":"dark","welcomed":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if Open(fresh).Get().Welcomed {
		t.Error("an unfinished welcome should show again")
	}
}

// Only libraries the interface knows can be hidden, each once.
func TestHiddenSources(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "settings.json"))
	if got := s.Get().HiddenSources; got == nil || len(got) != 0 {
		t.Fatalf("new settings hide %v", got)
	}
	v := s.Get()
	v.HiddenSources = []string{"epic", "nope", "epic", "external"}
	got, err := s.Set(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.HiddenSources) != 2 || got.HiddenSources[0] != "epic" || got.HiddenSources[1] != "external" {
		t.Errorf("hidden sources = %v, want [epic external]", got.HiddenSources)
	}
}

// The experimental store is off until this PC turns it on; another PC's
// settings never turn it on.
func TestExperimentalStore(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "settings.json"))
	if s.Get().ExperimentalStore {
		t.Fatal("the experimental store is on by default")
	}
	v, err := s.Get().WithPortable([]byte(`{"theme":"dark","experimentalStore":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if v.ExperimentalStore {
		t.Error("another PC's settings turned the experimental store on")
	}
	v.ExperimentalStore = true
	if got, err := s.Set(v); err != nil || !got.ExperimentalStore {
		t.Errorf("turning it on: %v, %v", got.ExperimentalStore, err)
	}
}

// Settings from before the store get its defaults; paths that aren't
// absolute are dropped and network values kept in range.
func TestStoreSettings(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.json")
	if err := os.WriteFile(old, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Open(old)
	if got := s.Get().Store; !got.PauseWhilePlaying || !got.Network.DHT || got.Network.MaxActive != 2 {
		t.Errorf("store defaults missing: %+v", got)
	}
	v := s.Get()
	v.Store.QBittorrent, v.Store.Downloads, v.Store.Network.Port = `qbittorrent.exe`, `D:\`, -4
	got, err := s.Set(v)
	if err != nil {
		t.Fatal(err)
	}
	v.Store.Feeds = []FeedSource{{URL: "https://a.example/feed.json", Enabled: true}, {URL: "http://b.example/feed.json"}, {URL: " https://a.example/feed.json "}}
	got, err = s.Set(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Store.Feeds) != 1 || got.Store.Feeds[0].URL != "https://a.example/feed.json" {
		t.Errorf("feeds: plain HTTP and duplicates should go: %+v", got.Store.Feeds)
	}
	if got.Store.QBittorrent != "" || got.Store.Downloads != "" || got.Store.Network.Port != 0 {
		t.Errorf("store settings kept bad values: %+v", got.Store)
	}
	v.Store.QBittorrent, v.Store.Downloads = `C:\Apps\qBittorrent\qbittorrent.exe`, `D:\Downloads\Games\`
	if got, _ := s.Set(v); got.Store.QBittorrent != `C:\Apps\qBittorrent\qbittorrent.exe` || got.Store.Downloads != `D:\Downloads\Games` {
		t.Errorf("good paths: %q, %q", got.Store.QBittorrent, got.Store.Downloads)
	}
}

// Achievements are on for everyone, updaters included; hidden ones stay hidden.
func TestAchievementDefaults(t *testing.T) {
	old := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(old, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if v := Open(old).Get(); !v.Achievements || v.ShowHiddenAchievements {
		t.Errorf("achievements %v, hidden shown %v", v.Achievements, v.ShowHiddenAchievements)
	}
}

func TestAskWhoPlaysOff(t *testing.T) {
	if Defaults().AskWhoPlays {
		t.Error("asking who's playing is on by default")
	}
}

func TestPortable(t *testing.T) {
	a := Defaults()
	a.Theme, a.Glyphs, a.HiddenSources, a.Folders, a.StartInBigPicture = "light", "xbox", []string{"epic"}, []string{`D:\Games`}, true
	a.AskWhoPlays = true // a PC people take turns on
	b, err := json.Marshal(a.Portable())
	if err != nil {
		t.Fatal(err)
	}
	got, err := Defaults().WithPortable(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "light" || got.Glyphs != "xbox" || len(got.HiddenSources) != 1 {
		t.Fatalf("not taken: %+v", got)
	}
	if len(got.Folders) != 0 || got.StartInBigPicture || got.AskWhoPlays {
		t.Fatalf("the PC's own settings changed: %+v", got)
	}
	// Bad values from another PC are normalised.
	got, _ = Defaults().WithPortable([]byte(`{"theme":"neon","syncWait":7}`))
	if got.Theme != "system" || got.SyncWait != 60 {
		t.Fatalf("not normalised: %+v", got)
	}
}

// Settings from before v1.9 called external copies unofficial.
func TestLegacyUnofficial(t *testing.T) {
	old := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(old, []byte(`{"detectUnofficial":false,"hiddenSources":["unofficial","steam"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	v := Open(old).Get()
	if v.DetectExternal {
		t.Error("detectUnofficial false was lost")
	}
	if len(v.HiddenSources) != 2 || v.HiddenSources[0] != "external" || v.HiddenSources[1] != "steam" {
		t.Errorf("hidden sources = %v, want [external steam]", v.HiddenSources)
	}
	got, err := Defaults().WithPortable([]byte(`{"detectUnofficial":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.DetectExternal {
		t.Error("an older PC's detectUnofficial false was lost")
	}
	if got, _ := Defaults().WithPortable([]byte(`{"detectUnofficial":false,"detectExternal":true}`)); !got.DetectExternal {
		t.Error("detectExternal should win over detectUnofficial")
	}
}

// A new Store user gets both sources when they turn the Store on; nothing
// is chosen for them while it stays off.
func TestStoreOnChoosesSources(t *testing.T) {
	s := Open(filepath.Join(t.TempDir(), "settings.json"))
	v := s.Get()
	if v.Store.SourceSetup != SetupPending || len(v.Store.Sources) != 0 || v.Store.PrivateSources {
		t.Fatalf("new settings chose sources already: %+v", v.Store)
	}
	v.Theme = "dark"
	if got, _ := s.Set(v); got.Store.PrivateSources || len(got.Store.Sources) != 0 {
		t.Fatalf("sources were chosen with the Store off: %+v", got.Store)
	}
	v.ExperimentalStore = true
	got, err := s.Set(v)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Store.PrivateSources || got.Store.SourceSetup != SetupDone || len(got.Store.Sources) != 2 {
		t.Fatalf("turning the Store on: %+v", got.Store)
	}
	if !got.Store.DiscoveryOn("fitgirl") || !got.Store.DiscoveryOn("dodi") || got.Store.DiscoveryOn("1337x") {
		t.Error("DiscoveryOn disagrees with the chosen sources")
	}
	// Turning it off and on again keeps what the person chose meanwhile.
	got.Store.Sources = []string{"dodi"}
	got.ExperimentalStore = false
	got, _ = s.Set(got)
	got.ExperimentalStore = true
	if got, _ = s.Set(got); len(got.Store.Sources) != 1 || got.Store.Sources[0] != "dodi" {
		t.Errorf("the choice was replaced: %v", got.Store.Sources)
	}
}

// Someone who used the Store before discovery existed chooses its sources
// once; their feeds and a disabled source preference stay as they were.
func TestExistingStoreUserIsAsked(t *testing.T) {
	dir := t.TempDir()
	for _, private := range []bool{false, true} {
		p := filepath.Join(dir, "settings.json")
		raw := `{"welcomed":true,"experimentalStore":true,"store":{"privateSources":` + map[bool]string{false: "false", true: "true"}[private] +
			`,"feeds":[{"url":"https://a.example/feed.json","enabled":true,"trust":1}]}}`
		if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		got := Open(p).Get().Store
		if got.SourceSetup != SetupAsk || len(got.Sources) != 0 || got.PrivateSources != private {
			t.Errorf("privateSources %v: %+v", private, got)
		}
		if len(got.Feeds) != 1 || got.Feeds[0].Trust != 1 {
			t.Errorf("feeds changed: %+v", got.Feeds)
		}
		if got.DiscoveryOn("fitgirl") {
			t.Error("discovery runs before the person chose")
		}
	}
	// A settings file from before the Store was ever on is a new Store user.
	p := filepath.Join(dir, "off.json")
	if err := os.WriteFile(p, []byte(`{"welcomed":true,"experimentalStore":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Open(p).Get().Store.SourceSetup; got != SetupPending {
		t.Errorf("store off: setup %q", got)
	}
	// Saved settings are not asked again, and unknown values are dropped.
	s := Open(p)
	v := s.Get()
	v.Store.SourceSetup, v.Store.Sources = SetupDone, []string{"dodi", "nope", "fitgirl", "dodi"}
	if got, _ := s.Set(v); got.Store.SourceSetup != SetupDone || len(got.Store.Sources) != 2 || got.Store.Sources[0] != "fitgirl" {
		t.Errorf("normalized sources: %+v", got.Store)
	}
	if got := Open(p).Get().Store.SourceSetup; got != SetupDone {
		t.Errorf("reopened: %q", got)
	}
}

// A settings file cut short (a power cut while saving) is set aside and
// the copy from the last good start is used, not the defaults.
func TestDamagedFileFallsBackToBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s := Open(path)
	v := s.Get()
	v.Theme, v.Folders = "light", []string{`D:\Games`}
	if _, err := s.Set(v); err != nil {
		t.Fatal(err)
	}
	Open(path) // a good start keeps a backup
	if err := os.WriteFile(path, []byte(`{"theme":"li`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := Open(path).Get()
	if got.Theme != "light" || len(got.Folders) != 1 {
		t.Errorf("after damage: theme %q, folders %v", got.Theme, got.Folders)
	}
	if broken, _ := filepath.Glob(path + ".broken-*"); len(broken) != 1 {
		t.Errorf("the damaged file wasn't set aside: %v", broken)
	}
}
