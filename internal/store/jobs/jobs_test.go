package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/safety"
	"github.com/ApolloF/Seaglass/internal/torrent"
)

func TestCan(t *testing.T) {
	checked := &safety.Report{Verdict: safety.Clean}
	for _, c := range []struct {
		j    Job
		a    Action
		want bool
	}{
		{Job{State: Queued}, Pause, true}, {Job{State: Downloading}, Pause, true}, {Job{State: Paused}, Pause, false}, {Job{State: Downloaded}, Pause, false},
		{Job{State: Paused}, Resume, true}, {Job{State: Failed}, Resume, true}, {Job{State: Downloading}, Resume, false}, {Job{State: Downloaded}, Resume, false},
		{Job{State: Downloaded}, Remove, true}, {Job{State: Failed}, Remove, true}, {Job{State: Installing}, Remove, false}, {Job{State: Scanning}, Remove, false},
		{Job{State: Downloaded, Safety: checked}, Install, true}, {Job{State: Downloaded}, Install, false}, {Job{State: Blocked, Safety: checked}, Install, false},
		{Job{State: Blocked}, Allow, true}, {Job{State: Downloaded}, Allow, false},
		{Job{State: Installed}, Uninstall, true}, {Job{State: Downloaded}, Uninstall, false},
	} {
		if got := c.j.Can(c.a); got != c.want {
			t.Errorf("%s job, %s = %v, want %v", c.j.State, c.a, got, c.want)
		}
	}
}

// Once downloaded, the engine's view (seeding, stopped, even failing
// after the files were cleaned up) doesn't move the job back.
func TestSyncAfterDownload(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, s := range []State{Scanning, Downloaded, Blocked, Installing, Installed} {
		j := Job{State: s, Safety: &safety.Report{}}
		j.Sync(&torrent.Torrent{Hash: "aa", State: torrent.Failed}, now)
		j.Sync(&torrent.Torrent{Hash: "aa", State: torrent.Complete}, now)
		if j.State != s {
			t.Errorf("%s became %s", s, j.State)
		}
	}
	f := Job{State: Failed, Safety: &safety.Report{}, Error: "installer failed"}
	f.Sync(&torrent.Torrent{Hash: "aa", State: torrent.Seeding}, now)
	if f.State != Failed || f.Error != "installer failed" || !f.Seeding {
		t.Errorf("a failed install: %+v", f)
	}
}

func TestSync(t *testing.T) {
	now := time.Unix(1000, 0)
	j := Job{State: Queued}
	if !j.Sync(&torrent.Torrent{Hash: "aa", State: torrent.Metadata}, now) || j.State != Downloading || j.Hash != "aa" {
		t.Errorf("picked up by the engine: %+v", j)
	}
	if j.Sync(&torrent.Torrent{Hash: "aa", State: torrent.Downloading, Done: 5, Size: 10, DownSpeed: 3}, now) {
		t.Error("progress alone shouldn't need a save")
	}
	if j.Done != 5 || j.DownSpeed != 3 {
		t.Errorf("progress not taken: %+v", j)
	}

	p := Job{State: Paused}
	p.Sync(&torrent.Torrent{Hash: "bb", State: torrent.Downloading}, now)
	if p.State != Paused {
		t.Error("the engine still running a paused download mustn't unpause it")
	}

	if !j.Sync(&torrent.Torrent{Hash: "aa", State: torrent.Seeding, Done: 10, Size: 10}, now) || j.State != Downloaded || !j.Seeding || j.Finished != 1000 {
		t.Errorf("complete: %+v", j)
	}
	if !j.Active() {
		t.Error("a seeding download needs the engine")
	}
	j.Sync(&torrent.Torrent{Hash: "aa", State: torrent.Complete, Done: 10, Size: 10}, now)
	if j.Active() || j.Seeding {
		t.Errorf("a finished, stopped download doesn't need the engine: %+v", j)
	}

	f := Job{State: Downloading}
	if !f.Sync(&torrent.Torrent{State: torrent.Failed}, now) || f.State != Failed || f.Error == "" {
		t.Errorf("failed: %+v", f)
	}

	lost := Job{State: Downloading, DownSpeed: 9}
	if !lost.Sync(nil, now) || lost.State != Queued || lost.DownSpeed != 0 {
		t.Errorf("gone from the engine: %+v", lost)
	}
}

func TestStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.json")
	s := Open(path)
	now := time.Unix(1000, 0)
	a, err := s.Add(Job{Title: "Game A", Source: "magnet:?xt=urn:btih:aa", SavePath: `D:\Games`, State: Downloaded, Hash: "x"}, now)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Add(Job{Title: "Game B", Source: "magnet:?xt=urn:btih:bb", SavePath: `D:\Games`}, now)
	if a.ID == b.ID || len(a.ID) != 15 || a.State != Queued || a.Hash != "" {
		t.Errorf("ids %q and %q", a.ID, b.ID)
	}
	if _, err := s.Update(a.ID, func(j *Job) bool { j.State = Paused; return true }); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(b.ID, func(*Job) bool { return true }); err != ErrNotFound {
		t.Errorf("update of a deleted download: %v", err)
	}

	again := Open(path)
	got := again.All()
	if len(got) != 1 || got[0].ID != a.ID || got[0].State != Paused {
		t.Fatalf("reopened: %+v", got)
	}

	// A damaged file falls back to the copy from the last good open.
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Open(path).All(); len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("after damage: %+v", got)
	}
}

// Seaglass closing (or crashing) while a download was being checked or
// installed leaves it where it can go on: checked again, or retried.
func TestOpenPicksUpInterruptedWork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "downloads.json")
	s := Open(path)
	now := time.Unix(1000, 0)
	scan, _ := s.Add(Job{Title: "Scanned", Source: "magnet:?xt=urn:btih:aa", SavePath: `D:\Games`}, now)
	inst, _ := s.Add(Job{Title: "Installing", Source: "magnet:?xt=urn:btih:bb", SavePath: `D:\Games`}, now)
	report := &safety.Report{Verdict: safety.Clean}
	_, _ = s.Update(scan.ID, func(j *Job) bool { j.State, j.Safety = Scanning, report; return true })
	_, _ = s.Update(inst.ID, func(j *Job) bool { j.State, j.Safety, j.InstallStarted = Installing, report, true; return true })

	again := Open(path)
	if j, _ := again.Get(scan.ID); j.State != Downloaded || j.Safety != nil {
		t.Errorf("interrupted check: %s, safety %v; want downloaded, to be checked again", j.State, j.Safety)
	}
	j, _ := again.Get(inst.ID)
	if j.State != Failed || j.Error == "" || !j.Can(Resume) || !j.Can(Remove) {
		t.Errorf("interrupted install: %+v; want failed, retryable and removable", j)
	}
}
