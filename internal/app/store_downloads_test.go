package app

import (
	"testing"

	"github.com/ApolloF/Seaglass/internal/store/jobs"
)

// The loop sends the downloads only after reaching qBittorrent; while it
// can't start, what the person does must still reach the interface.
func TestDownloadChangesReachTheInterfaceWithoutTheEngine(t *testing.T) {
	c := testStoreCore(t)
	v := c.Settings.Get()
	v.Store.Downloads = t.TempDir()
	if _, err := c.Settings.Set(v); err != nil {
		t.Fatal(err)
	}
	var sent [][]jobs.Job
	c.store.sendJobs = func(all []jobs.Job) { sent = append(sent, all) }
	s := NewStoreService(c)

	j, err := s.AddDownload("magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", "Night Harbor")
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || len(sent[0]) != 1 || sent[0][0].ID != j.ID || sent[0][0].State != jobs.Queued {
		t.Fatalf("a queued download wasn't sent at once: %+v", sent)
	}

	if err := s.DownloadAction(j.ID, jobs.Pause, false); err != nil {
		t.Fatal(err)
	}
	if last := sent[len(sent)-1]; len(last) != 1 || last[0].State != jobs.Paused {
		t.Fatalf("pausing wasn't sent: %+v", last)
	}

	if err := s.DownloadAction(j.ID, jobs.Resume, false); err != nil {
		t.Fatal(err)
	}
	if last := sent[len(sent)-1]; len(last) != 1 || last[0].State != jobs.Queued {
		t.Fatalf("resuming wasn't sent: %+v", last)
	}
	select {
	case <-c.store.kick:
	default:
		t.Error("the loop wasn't woken to start the engine")
	}
}
