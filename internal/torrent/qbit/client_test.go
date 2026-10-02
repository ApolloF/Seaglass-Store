package qbit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ApolloF/Seaglass/internal/torrent"
)

// fake is a qBittorrent Web UI that checks sessions and CSRF headers and
// records what it was asked.
type fake struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	logins int
	sid    string   // the session that's valid; "" expires it
	calls  []string // paths called with a valid session
	form   map[string]map[string]string
	noStop bool // 4.x: no torrents/stop
	v52    bool // 5.2: login answers 204 or 401 instead of "Ok." or "Fails."
}

func newFake(t *testing.T) *fake {
	f := &fake{t: t, form: map[string]map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Referer") != f.srv.URL || r.Header.Get("Origin") != f.srv.URL {
		http.Error(w, "CSRF", http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
	if path == "auth/login" {
		f.logins++
		if r.FormValue("username") == "sg" && r.FormValue("password") == "pw" {
			f.sid = "s" + string(rune('0'+f.logins))
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: f.sid, Path: "/"})
			if f.v52 {
				w.WriteHeader(http.StatusNoContent)
			} else {
				w.Write([]byte("Ok."))
			}
		} else if f.v52 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		} else {
			w.Write([]byte("Fails."))
		}
		return
	}
	if ck, err := r.Cookie("SID"); err != nil || f.sid == "" || ck.Value != f.sid {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	f.calls = append(f.calls, path)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		_ = r.ParseMultipartForm(1 << 20)
	} else {
		_ = r.ParseForm()
	}
	vals := map[string]string{}
	for k, v := range r.Form {
		vals[k] = v[0]
	}
	f.form[path] = vals
	switch path {
	case "torrents/stop", "torrents/start":
		if f.noStop {
			http.NotFound(w, r)
		}
	case "torrents/info":
		w.Write([]byte(`[
			{"hash":"aa","name":"Game A","tags":"seaglass, sg-1","state":"stalledDL","size":1000,"amount_left":250,"progress":0.75,"dlspeed":500,"eta":8640000,"num_seeds":3,"num_leechs":1,"save_path":"D:\\Games"},
			{"hash":"bb","name":"Game B","tags":"sg-2, seaglass","state":"stoppedUP","size":10,"amount_left":0,"progress":1,"eta":0}
		]`))
	case "torrents/files":
		w.Write([]byte(`[{"index":0,"name":"setup.exe","size":5,"progress":1,"priority":1},{"index":1,"name":"lang/fr.bin","size":9,"progress":0,"priority":0}]`))
	case "app/networkInterfaceList":
		w.Write([]byte(`[{"name":"Ethernet","value":"ethernet_32769"},{"name":"Loopback Pseudo-Interface 1","value":"loopback_0"},{"name":"wt0","value":"iftype53_32768"}]`))
	case "torrents/add":
		switch {
		case f.v52 && vals["urls"] == "magnet:bad":
			w.Write([]byte(`{"added_torrent_ids":[],"failure_count":1,"pending_count":0,"success_count":0}`))
		case f.v52:
			w.Write([]byte(`{"added_torrent_ids":[],"failure_count":0,"pending_count":1,"success_count":0}`))
		default:
			w.Write([]byte("Ok."))
		}
	}
}

func TestClientLogsInAgainWhenTheSessionExpires(t *testing.T) {
	f := newFake(t)
	c := NewClient(f.srv.URL, "sg", "pw")
	ctx := context.Background()
	if err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.sid = "" // qBittorrent restarted, or the session timed out
	f.mu.Unlock()
	if _, err := c.List(ctx); err != nil {
		t.Fatalf("list after the session expired: %v", err)
	}
	if f.logins != 2 {
		t.Errorf("logins = %d, want 2", f.logins)
	}
	if err := NewClient(f.srv.URL, "sg", "wrong").Login(ctx); err != ErrAuth {
		t.Errorf("wrong password: %v, want ErrAuth", err)
	}
}

func TestClientLogsInToQBittorrent52(t *testing.T) {
	f := newFake(t)
	f.v52 = true
	ctx := context.Background()
	c := NewClient(f.srv.URL, "sg", "pw")
	if err := c.Login(ctx); err != nil {
		t.Fatalf("login with an empty 204: %v", err)
	}
	if _, err := c.List(ctx); err != nil {
		t.Fatalf("list after logging in: %v", err)
	}
	if err := NewClient(f.srv.URL, "sg", "wrong").Login(ctx); err != ErrAuth {
		t.Errorf("wrong password (401): %v, want ErrAuth", err)
	}
	// Adds answer with counts: a pending .torrent URL is taken, a failure isn't.
	if err := c.Add(ctx, "http://127.0.0.1/a.torrent", torrent.AddOptions{}); err != nil {
		t.Errorf("a pending add was refused: %v", err)
	}
	if err := c.Add(ctx, "magnet:bad", torrent.AddOptions{}); err == nil {
		t.Error("a failed add was taken")
	}
}

func TestClientList(t *testing.T) {
	f := newFake(t)
	c := NewClient(f.srv.URL, "sg", "pw")
	got, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.form["torrents/info"]["tag"] != Tag {
		t.Errorf("listed with tag %q; Seaglass should only see its own torrents", f.form["torrents/info"]["tag"])
	}
	want := []torrent.Torrent{
		{Hash: "aa", Name: "Game A", Tag: "sg-1", State: torrent.Downloading, Size: 1000, Done: 750, Progress: 0.75, DownSpeed: 500, Seeds: 3, Peers: 1, SavePath: `D:\Games`},
		{Hash: "bb", Name: "Game B", Tag: "sg-2", State: torrent.Complete, Size: 10, Done: 10, Progress: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d torrents, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("torrent %d\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestClientAdd(t *testing.T) {
	f := newFake(t)
	c := NewClient(f.srv.URL, "sg", "pw")
	ctx := context.Background()
	if err := c.Add(ctx, "magnet:?xt=urn:btih:abc", torrent.AddOptions{SavePath: `D:\Downloads`, Tag: "sg-7", Paused: true}); err != nil {
		t.Fatal(err)
	}
	form := f.form["torrents/add"]
	for k, want := range map[string]string{"urls": "magnet:?xt=urn:btih:abc", "savepath": `D:\Downloads`, "tags": "seaglass,sg-7", "stopped": "true"} {
		if form[k] != want {
			t.Errorf("%s = %q, want %q", k, form[k], want)
		}
	}
	if err := c.Add(ctx, "magnet:a\nmagnet:b", torrent.AddOptions{}); err == nil {
		t.Error("two sources in one add were accepted")
	}
	if err := c.Add(ctx, "magnet:a", torrent.AddOptions{Tag: "a,b"}); err == nil {
		t.Error("a tag with a comma was accepted")
	}
}

func TestClientPauseFallsBackToQBittorrent4(t *testing.T) {
	f := newFake(t)
	f.noStop = true
	c := NewClient(f.srv.URL, "sg", "pw")
	if err := c.Pause(context.Background(), "aa", "bb"); err != nil {
		t.Fatal(err)
	}
	if f.form["torrents/pause"]["hashes"] != "aa|bb" {
		t.Errorf("pause hashes = %q, calls %v", f.form["torrents/pause"]["hashes"], f.calls)
	}
}

func TestClientFilesAndInterfaces(t *testing.T) {
	f := newFake(t)
	c := NewClient(f.srv.URL, "sg", "pw")
	ctx := context.Background()
	files, err := c.Files(ctx, "aa")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[1] != (torrent.File{Index: 1, Name: "lang/fr.bin", Size: 9, Skip: true}) {
		t.Errorf("files = %+v", files)
	}
	if err := c.SetSkipped(ctx, "aa", []int{1, 3}, true); err != nil {
		t.Fatal(err)
	}
	if p := f.form["torrents/filePrio"]; p["id"] != "1|3" || p["priority"] != "0" {
		t.Errorf("filePrio = %v", p)
	}
	ifs, err := c.Interfaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ifs) != 2 || ifs[1] != (torrent.Interface{ID: "iftype53_32768", Name: "wt0"}) {
		t.Errorf("interfaces = %+v (loopback should be left out)", ifs)
	}
}

func TestClientApply(t *testing.T) {
	f := newFake(t)
	c := NewClient(f.srv.URL, "sg", "pw")
	n := torrent.DefaultNetwork()
	n.MaxActive = 500 // normalized before it's sent
	if err := c.Apply(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(f.form["app/setPreferences"]["json"]), &p); err != nil {
		t.Fatal(err)
	}
	if p["max_active_downloads"] != float64(2) || p["web_ui_address"] != "127.0.0.1" {
		t.Errorf("preferences sent: max_active_downloads %v, web_ui_address %v", p["max_active_downloads"], p["web_ui_address"])
	}
}
