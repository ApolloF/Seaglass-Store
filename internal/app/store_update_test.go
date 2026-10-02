package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/safety"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
)

// A newer version of a game the store installed shows as an update, goes
// into the same folder over the old files, and replaces the old download.
func TestUpdateOverInstalled(t *testing.T) {
	c := testStoreCore(t)
	c.Launch = launch.NewManager(func(launch.Session) {})
	if _, err := c.updateSettings(func(v *settings.Settings) { v.Store.KeepDownloads = true }); err != nil {
		t.Fatal(err)
	}
	gameDir := filepath.Join(t.TempDir(), "Games", "Tiny Game")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "TinyGame.exe"), []byte("MZ old"), 0o644); err != nil {
		t.Fatal(err)
	}
	old, _ := c.store.jobs.Add(jobs.Job{Title: "Tiny Game v1", GameKey: "title:tinygame", Version: "v1", InstallDir: gameDir}, time.Now())
	c.store.jobs.Update(old.ID, func(j *jobs.Job) bool {
		j.State, j.Safety = jobs.Installed, &safety.Report{Verdict: safety.Clean}
		return true
	})
	m := "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c"
	c.catalog.entries = catalog.Build([]catalog.Source{{URL: "https://a/f.json", Name: "A", Feed: feed.Feed{Items: []feed.Item{
		{Title: "Tiny Game", Version: "v2", Magnet: m, InstallerType: "portable"},
		{Title: "Tiny Game", Version: "v1", Magnet: m},
	}}}}, nil)

	ups := c.catalog.updates()
	if len(ups) != 1 || ups[0].Installed == nil || ups[0].Installed.Version != "v1" || ups[0].Recommended.Offer != 0 {
		t.Fatalf("updates: %+v", ups)
	}
	svc := NewStoreService(c)
	j, err := svc.DownloadOffer(ups[0].Key, 0, InstallOptions{Update: true, Dir: `C:\ignored`})
	if err != nil {
		t.Fatal(err)
	}
	if j.InstallDir != gameDir || j.Replaces != old.ID || !j.AutoInstall {
		t.Fatalf("update download: %+v", j)
	}

	dl := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dl, "Tiny Game v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dl, "Tiny Game v2", "TinyGame.exe"), []byte("MZ new"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.store.jobs.Update(j.ID, func(j *jobs.Job) bool {
		j.SavePath, j.Name, j.State, j.Safety = dl, "Tiny Game v2", jobs.Downloaded, &safety.Report{Verdict: safety.Clean, Main: "TinyGame.exe"}
		return true
	})
	c.store.pipe.claim(j.ID)
	c.store.pipe.install(j.ID)

	got, _ := c.store.jobs.Get(j.ID)
	if got.State != jobs.Installed {
		t.Fatalf("after updating: %+v", got)
	}
	if b, _ := os.ReadFile(filepath.Join(gameDir, "TinyGame.exe")); string(b) != "MZ new" {
		t.Errorf("game file after the update: %q", b)
	}
	if _, ok := c.store.jobs.Get(old.ID); ok {
		t.Error("the old version's download is still listed")
	}
	if ups := c.catalog.updates(); len(ups) != 0 {
		t.Errorf("still an update after updating: %+v", ups)
	}
}
