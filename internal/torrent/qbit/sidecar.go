package qbit

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/torrent"
)

// Find returns where qBittorrent is installed, or "".
func Find() string {
	for _, dir := range []string{platform.ProgramFiles, platform.ProgramFilesX86, filepath.Join(platform.Local, "Programs")} {
		if dir == "" {
			continue
		}
		if p := filepath.Join(dir, "qBittorrent", "qbittorrent.exe"); platform.IsFile(p) {
			return p
		}
	}
	return ""
}

// Sidecar runs qBittorrent hidden, with a profile of its own (the
// person's own qBittorrent, settings and torrents are left alone), its
// Web UI on a random localhost port and a password made up at each start.
type Sidecar struct {
	Exe     string // qbittorrent.exe
	Profile string // its profile folder; qBittorrent keeps everything under Profile\qBittorrent

	mu      sync.Mutex
	cmd     *exec.Cmd
	client  *Client
	exited  chan struct{}
	lastErr error
	failAt  time.Time
	fails   int
}

// Running reports whether qBittorrent runs.
func (s *Sidecar) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client != nil && !closed(s.exited)
}

// Ensure returns a client for a running qBittorrent, starting it when it
// isn't running. After a failed start it waits a while (longer after each
// failure) before trying again, returning the failure meanwhile.
func (s *Sidecar) Ensure(ctx context.Context, n torrent.Network) (*Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil && !closed(s.exited) {
		return s.client, nil
	}
	s.client = nil
	if s.lastErr != nil && time.Since(s.failAt) < backoff(s.fails) {
		return nil, s.lastErr
	}
	c, err := s.start(ctx, n)
	if err != nil {
		s.lastErr, s.failAt, s.fails = err, time.Now(), s.fails+1
		return nil, err
	}
	s.client, s.lastErr, s.fails = c, nil, 0
	return c, nil
}

func backoff(fails int) time.Duration {
	return min(time.Duration(1<<min(fails, 6))*time.Second, time.Minute)
}

func closed(ch chan struct{}) bool {
	if ch == nil {
		return true
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (s *Sidecar) start(ctx context.Context, n torrent.Network) (*Client, error) {
	if s.Exe == "" || !platform.IsFile(s.Exe) {
		return nil, errors.New("qBittorrent isn't installed")
	}
	s.endOrphan()
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	pass, err := secret()
	if err != nil {
		return nil, err
	}
	if err := s.configure(port, "seaglass", pass); err != nil {
		return nil, fmt.Errorf("qBittorrent's settings: %w", err)
	}
	cmd := exec.Command(s.Exe, "--profile="+s.Profile, "--no-splash")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	s.cmd, s.exited = cmd, exited
	s.savePID(cmd.Process.Pid)

	c := NewClient("http://127.0.0.1:"+strconv.Itoa(port), "seaglass", pass)
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := c.Login(ctx)
		if err == nil {
			break
		}
		if closed(exited) {
			return nil, errors.New("qBittorrent quit while starting (is another copy using this profile?)")
		}
		if time.Now().After(deadline) || ctx.Err() != nil || errors.Is(err, ErrAuth) {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("qBittorrent didn't answer: %w", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(300 * time.Millisecond):
		}
	}
	if err := c.Apply(ctx, n); err != nil {
		return nil, fmt.Errorf("qBittorrent's network settings: %w", err)
	}
	return c, nil
}

// Stop asks qBittorrent to quit (it saves its torrents' state), and ends
// it when it doesn't within a few seconds.
func (s *Sidecar) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || closed(s.exited) {
		return
	}
	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = s.client.Shutdown(ctx)
		cancel()
	}
	select {
	case <-s.exited:
	case <-time.After(10 * time.Second):
		_ = s.cmd.Process.Kill()
	}
	s.client = nil
	_ = os.Remove(s.pidFile())
}

// configure writes what Seaglass needs into qBittorrent's settings file
// before it starts: the Web UI on localhost only with this start's
// password, no window, no tray icon, no update prompts. The legal notice
// was shown to the person (they turned the store on).
func (s *Sidecar) configure(port int, user, pass string) error {
	ini := filepath.Join(s.Profile, "qBittorrent", "config", "qBittorrent.ini")
	if err := os.MkdirAll(filepath.Dir(ini), 0o700); err != nil {
		return err
	}
	b, err := os.ReadFile(ini)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	hash, err := passwordHash(pass)
	if err != nil {
		return err
	}
	text := string(b)
	for _, kv := range [][3]string{
		{"LegalNotice", "Accepted", "true"},
		{"GUI", "StartUpWindowState", "Hidden"},
		{"Preferences", `General\SystemTrayEnabled`, "false"},
		{"Preferences", `General\ExitConfirm`, "false"},
		{"Preferences", `Advanced\updateCheck`, "false"},
		{"Preferences", `WebUI\Enabled`, "true"},
		{"Preferences", `WebUI\Address`, "127.0.0.1"},
		{"Preferences", `WebUI\Port`, strconv.Itoa(port)},
		{"Preferences", `WebUI\UseUPnP`, "false"},
		{"Preferences", `WebUI\LocalHostAuth`, "true"},
		{"Preferences", `WebUI\CSRFProtection`, "true"},
		{"Preferences", `WebUI\HostHeaderValidation`, "true"},
		{"Preferences", `WebUI\Username`, user},
		{"Preferences", `WebUI\Password_PBKDF2`, `"@ByteArray(` + hash + `)"`},
	} {
		text = setINI(text, kv[0], kv[1], kv[2])
	}
	tmp := ini + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ini)
}

// passwordHash is a password the way qBittorrent stores it:
// base64(salt):base64(PBKDF2-HMAC-SHA512, 100000 rounds, 64 bytes).
func passwordHash(pass string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha512.New, pass, salt, 100000, 64)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(salt) + ":" + base64.StdEncoding.EncodeToString(key), nil
}

func secret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (s *Sidecar) pidFile() string { return filepath.Join(s.Profile, "seaglass.pid") }

// savePID notes which process is the sidecar, so one left behind by a
// crash can be ended at the next start (it would hold the profile).
func (s *Sidecar) savePID(pid int) {
	_, started, err := platform.ProcessImage(uint32(pid))
	if err != nil {
		return
	}
	_ = os.WriteFile(s.pidFile(), []byte(fmt.Sprintf("%d %d", pid, started)), 0o600)
}

func (s *Sidecar) endOrphan() {
	b, err := os.ReadFile(s.pidFile())
	if err != nil {
		return
	}
	_ = os.Remove(s.pidFile())
	f := strings.Fields(string(b))
	if len(f) != 2 {
		return
	}
	pid, err1 := strconv.ParseUint(f[0], 10, 32)
	started, err2 := strconv.ParseUint(f[1], 10, 64)
	if err1 != nil || err2 != nil {
		return
	}
	// Only the same exe started at the same moment: a process id Windows
	// handed out again is someone else's.
	img, st, err := platform.ProcessImage(uint32(pid))
	if err != nil || st != started || !strings.EqualFold(filepath.Clean(img), filepath.Clean(s.Exe)) {
		return
	}
	_ = platform.EndProcess(uint32(pid), started)
	time.Sleep(500 * time.Millisecond) // let Windows release the profile's lock file
}
