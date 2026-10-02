package app

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/torrent"
)

// fakeEngine records what it was told.
type fakeEngine struct {
	torrent.Engine
	added           []torrent.AddOptions
	paused, resumed []string
}

func (f *fakeEngine) Add(_ context.Context, _ string, o torrent.AddOptions) error {
	f.added = append(f.added, o)
	return nil
}
func (f *fakeEngine) Pause(_ context.Context, h ...string) error {
	f.paused = append(f.paused, h...)
	return nil
}
func (f *fakeEngine) Resume(_ context.Context, h ...string) error {
	f.resumed = append(f.resumed, h...)
	return nil
}

func TestSteer(t *testing.T) {
	st := &storeState{jobs: jobs.Open(filepath.Join(t.TempDir(), "downloads.json"))}
	ctx := context.Background()
	job := func(s jobs.State) jobs.Job { return jobs.Job{ID: "sg-1", State: s, SavePath: t.TempDir()} }
	tor := func(s torrent.State) *torrent.Torrent { return &torrent.Torrent{Hash: "aa", State: s} }

	for _, c := range []struct {
		name            string
		job             jobs.Job
		t               *torrent.Torrent
		playing         bool
		add, pause, res bool
	}{
		{"queued, not in the engine yet", job(jobs.Queued), nil, false, true, false, false},
		{"paused before the engine had it", job(jobs.Paused), nil, false, false, false, false},
		{"paused by the person", job(jobs.Paused), tor(torrent.Downloading), false, false, true, false},
		{"resumed by the person", job(jobs.Downloading), tor(torrent.Paused), false, false, false, true},
		{"a game started", job(jobs.Downloading), tor(torrent.Downloading), true, false, true, false},
		{"a game still runs", job(jobs.Downloading), tor(torrent.Paused), true, false, false, false},
		{"seeding keeps going", job(jobs.Downloaded), tor(torrent.Seeding), false, false, false, false},
		{"seeding waits for a game", job(jobs.Downloaded), tor(torrent.Seeding), true, false, true, false},
		{"done seeding stays stopped", job(jobs.Downloaded), tor(torrent.Complete), false, false, false, false},
	} {
		f := &fakeEngine{}
		st.steer(ctx, f, c.job, c.t, c.playing, false)
		if (len(f.added) > 0) != c.add || slices.Contains(f.paused, "aa") != c.pause || slices.Contains(f.resumed, "aa") != c.res {
			t.Errorf("%s: added %v, paused %v, resumed %v", c.name, f.added, f.paused, f.resumed)
		}
		if c.add && (f.added[0].Tag != "sg-1" || f.added[0].Paused != c.playing) {
			t.Errorf("%s: added with %+v", c.name, f.added[0])
		}
	}
}

func TestSteerStopsWhenTheDiskIsFull(t *testing.T) {
	st := &storeState{jobs: jobs.Open(filepath.Join(t.TempDir(), "downloads.json"))}
	j, err := st.jobs.Add("Huge", "magnet:?xt=urn:btih:aa", t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeEngine{}
	st.steer(context.Background(), f, j, &torrent.Torrent{Hash: "aa", State: torrent.Downloading, Size: 1 << 62}, false, true)
	got, _ := st.jobs.Get(j.ID)
	if got.State != jobs.Failed || got.Error == "" || !slices.Contains(f.paused, "aa") {
		t.Errorf("a download bigger than the disk: %+v, paused %v", got, f.paused)
	}
}

func TestDownloadSource(t *testing.T) {
	for _, c := range []struct{ src, title, wantTitle string }{
		{"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=Big+Buck+Bunny", "", "Big Buck Bunny"},
		{" magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567 ", "", "Download"},
		{"magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=x", "Mine", "Mine"},
		{"https://archive.org/download/item/item_archive.torrent", "", "item_archive"},
	} {
		_, title, err := downloadSource(c.src, c.title)
		if err != nil || title != c.wantTitle {
			t.Errorf("%q: title %q, err %v; want %q", c.src, title, err, c.wantTitle)
		}
	}
	for _, bad := range []string{"", "hello", "magnet:?dn=no-hash", "file:///C:/x.torrent", "ftp://host/x.torrent", "https:///x.torrent"} {
		if _, _, err := downloadSource(bad, ""); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestBytesText(t *testing.T) {
	if bytesText(5<<30+1<<29) != "5.5 GB" || bytesText(1) != "1 MB" {
		t.Errorf("%s, %s", bytesText(5<<30+1<<29), bytesText(1))
	}
}
