package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/launch"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/safety"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
)

// A finished download of a game that needs no installer goes through
// the checks (this PC's Defender included), is copied into its folder,
// lands in the library's folders, and uninstalls again.
func TestPipelinePortableGame(t *testing.T) {
	c := testStoreCore(t)
	c.Launch = launch.NewManager(func(launch.Session) {})
	if _, err := c.updateSettings(func(v *settings.Settings) { v.Store.KeepDownloads = true }); err != nil {
		t.Fatal(err)
	}
	dl := t.TempDir()
	game := filepath.Join(dl, "Tiny Game")
	write := func(p string, b []byte) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(game, "TinyGame.exe"), append([]byte("MZ"), make([]byte, 128)...))
	write(filepath.Join(game, "data", "level.pak"), []byte("level"))
	gamesRoot := filepath.Join(t.TempDir(), "Games")
	installDir := filepath.Join(gamesRoot, "Tiny Game")

	j, err := c.store.jobs.Add(jobs.Job{Title: "Tiny Game", Source: "magnet:?xt=urn:btih:aa", SavePath: dl, InstallDir: installDir, AutoInstall: true}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.jobs.Update(j.ID, func(j *jobs.Job) bool { j.State, j.Name = jobs.Downloaded, "Tiny Game"; return true }); err != nil {
		t.Fatal(err)
	}

	p := c.store.pipe
	p.claim(j.ID)
	p.scan(j.ID)
	got, _ := c.store.jobs.Get(j.ID)
	if got.State != jobs.Downloaded || got.Safety == nil || got.Safety.Main != "TinyGame.exe" {
		t.Fatalf("after the checks: %+v", got)
	}
	if got.Safety.Verdict == safety.Blocked {
		t.Fatalf("a harmless game was blocked: %+v", got.Safety.Findings)
	}
	if got.Safety.Verdict == safety.Clean {
		// Clean and asked to install: advance starts it.
		p.advance(c.store.jobs.All())
		waitFor(t, func() bool { j, _ := c.store.jobs.Get(j.ID); return j.State == jobs.Installed || j.State == jobs.Failed })
	} else {
		t.Logf("checks warned (%v), installing by hand", got.Safety.Findings)
		if err := NewStoreService(c).DownloadAction(j.ID, jobs.Install, false); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool { j, _ := c.store.jobs.Get(j.ID); return j.State == jobs.Installed || j.State == jobs.Failed })
	}
	got, _ = c.store.jobs.Get(j.ID)
	if got.State != jobs.Installed || got.Installer != "portable" || got.InstallDir != installDir {
		t.Fatalf("after installing: %+v", got)
	}
	if !platform.IsFile(filepath.Join(installDir, "data", "level.pak")) {
		t.Error("the game wasn't copied into its folder")
	}
	folders := c.Settings.Get().Folders
	if len(folders) != 1 || !strings.EqualFold(folders[0], gamesRoot) {
		t.Errorf("game folders: %v, want %s", folders, gamesRoot)
	}

	if err := p.uninstall(j.ID); err != nil {
		t.Fatal(err)
	}
	if platform.IsDir(installDir) {
		t.Error("the game's folder is still there")
	}
	if _, ok := c.store.jobs.Get(j.ID); ok {
		t.Error("a download whose files are gone should be forgotten after uninstalling")
	}
}

func TestAllowDownload(t *testing.T) {
	c := testStoreCore(t)
	j, _ := c.store.jobs.Add(jobs.Job{Title: "Risky Game v1"}, time.Now())
	c.store.jobs.Update(j.ID, func(j *jobs.Job) bool {
		j.State, j.Safety = jobs.Blocked, &safety.Report{Verdict: safety.Blocked}
		return true
	})
	svc := NewStoreService(c)
	if err := svc.AllowDownload(j.ID, "risky game"); err == nil {
		t.Error("a wrong confirmation was taken")
	}
	if err := svc.DownloadAction(j.ID, jobs.Install, false); err != nil {
		t.Fatal(err) // not allowed in this state: nothing happens
	}
	if got, _ := c.store.jobs.Get(j.ID); got.State != jobs.Blocked {
		t.Fatalf("a blocked download started installing: %+v", got)
	}
	if err := svc.AllowDownload(j.ID, " risky game V1 "); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.store.jobs.Get(j.ID); got.State != jobs.Downloaded || !got.Safety.Overridden {
		t.Errorf("allowed: %+v", got)
	}
}

func TestSandboxConfig(t *testing.T) {
	b, err := sandboxConfig(`D:\Downloads\Tom & Jerry <Deluxe>`)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `<HostFolder>D:\Downloads\Tom &amp; Jerry &lt;Deluxe&gt;</HostFolder>`) || !strings.Contains(s, "<ReadOnly>true</ReadOnly>") || !strings.Contains(s, "<Networking>Disable</Networking>") {
		t.Errorf("config:\n%s", s)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
