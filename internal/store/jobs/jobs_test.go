package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/torrent"
)

func TestCan(t *testing.T) {
	for _, c := range []struct {
		s    State
		a    Action
		want bool
	}{
		{Queued, Pause, true}, {Downloading, Pause, true}, {Paused, Pause, false}, {Downloaded, Pause, false},
		{Paused, Resume, true}, {Failed, Resume, true}, {Downloading, Resume, false}, {Downloaded, Resume, false},
		{Downloaded, Remove, true}, {Failed, Remove, true},
	} {
		if got := Can(c.s, c.a); got != c.want {
			t.Errorf("Can(%s, %s) = %v, want %v", c.s, c.a, got, c.want)
		}
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
	a, err := s.Add("Game A", "magnet:?xt=urn:btih:aa", `D:\Games`, now)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Add("Game B", "magnet:?xt=urn:btih:bb", `D:\Games`, now)
	if a.ID == b.ID || len(a.ID) != 15 {
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
