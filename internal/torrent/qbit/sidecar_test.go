package qbit

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/torrent"
)

// The settings file gets a password qBittorrent can check, and keeps
// what qBittorrent wrote itself.
func TestConfigure(t *testing.T) {
	s := &Sidecar{Profile: t.TempDir()}
	ini := filepath.Join(s.Profile, "qBittorrent", "config", "qBittorrent.ini")
	if err := os.MkdirAll(filepath.Dir(ini), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ini, []byte("[BitTorrent]\r\nSession\\Port=47653\r\n\r\n[Preferences]\r\nWebUI\\Port=8080\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.configure(18089, "seaglass", "pw"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(ini)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{"Session\\Port=47653", "WebUI\\Port=18089", "WebUI\\Address=127.0.0.1", "StartUpWindowState=Hidden", "[LegalNotice]\nAccepted=true", "Advanced\\updateCheck=false"} {
		if !strings.Contains(text, want) {
			t.Errorf("settings file lacks %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "WebUI\\Port=") != 1 {
		t.Errorf("the port is in the settings file more than once:\n%s", text)
	}

	_, after, _ := strings.Cut(text, `WebUI\Password_PBKDF2="@ByteArray(`)
	hash, _, _ := strings.Cut(after, `)"`)
	saltB64, keyB64, ok := strings.Cut(hash, ":")
	salt, err1 := base64.StdEncoding.DecodeString(saltB64)
	key, err2 := base64.StdEncoding.DecodeString(keyB64)
	if !ok || err1 != nil || err2 != nil || len(salt) != 16 {
		t.Fatalf("password hash %q isn't salt:key in base64", hash)
	}
	want, _ := pbkdf2.Key(sha512.New, "pw", salt, 100000, 64)
	if string(key) != string(want) {
		t.Error("the password hash doesn't check out")
	}
}

func TestBackoff(t *testing.T) {
	if backoff(1) != 2*time.Second || backoff(20) != time.Minute {
		t.Errorf("backoff: %v after one failure, %v after many", backoff(1), backoff(20))
	}
}

// WL_REAL_QBIT=1 starts this PC's qBittorrent as a sidecar with a
// throwaway profile, applies network settings and stops it again.
func TestRealSidecar(t *testing.T) {
	if os.Getenv("WL_REAL_QBIT") == "" {
		t.Skip("set WL_REAL_QBIT=1 to start this PC's qBittorrent")
	}
	exe := Find()
	if exe == "" {
		t.Skip("qBittorrent isn't installed")
	}
	s := &Sidecar{Exe: exe, Profile: t.TempDir()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	n := torrent.DefaultNetwork()
	n.Port = 51999
	c, err := s.Ensure(ctx, n)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	v, err := c.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ifs, err := c.Interfaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("qBittorrent %s, interfaces %v", v, ifs)
	if again, err := s.Ensure(ctx, n); err != nil || again != c {
		t.Errorf("a second Ensure started another qBittorrent: %v", err)
	}

	// Big Buck Bunny (Creative Commons), from WebTorrent's examples: added
	// paused, so only its file list is fetched.
	const bbb = "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c&dn=Big+Buck+Bunny&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337&tr=wss%3A%2F%2Ftracker.webtorrent.dev"
	if err := c.Add(ctx, bbb, torrent.AddOptions{SavePath: t.TempDir(), Tag: "sg-test", Paused: true}); err != nil {
		t.Fatal(err)
	}
	var ts []torrent.Torrent
	for range 50 { // qBittorrent adds it a moment later
		if ts, err = c.List(ctx); err != nil || len(ts) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || len(ts) != 1 || ts[0].Tag != "sg-test" || ts[0].Hash != "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c" {
		t.Fatalf("listed %+v, %v", ts, err)
	}
	t.Logf("added: %+v", ts[0])
	if err := c.Remove(ctx, true, ts[0].Hash); err != nil {
		t.Fatal(err)
	}
	if ts, _ := c.List(ctx); len(ts) != 0 {
		t.Errorf("still listed after removal: %+v", ts)
	}
	s.Stop()
	if s.Running() {
		t.Error("still running after Stop")
	}
}
