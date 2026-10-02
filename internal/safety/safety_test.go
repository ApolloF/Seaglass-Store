package safety

import (
	"archive/zip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// exe is enough of a Windows program for the checks.
var exe = append([]byte("MZ"), make([]byte, 64)...)

func TestMainFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Redist", "vcredist_x64.exe"), exe)
	write(t, filepath.Join(dir, "unins000.exe"), exe)
	write(t, filepath.Join(dir, "Game Setup.exe"), exe)
	write(t, filepath.Join(dir, "setup.exe"), exe)
	write(t, filepath.Join(dir, "a", "b", "c", "setup.exe"), exe)
	if got := MainFile(dir); filepath.Base(got) != "setup.exe" || filepath.Dir(got) != dir {
		t.Errorf("main file = %s", got)
	}
	empty := t.TempDir()
	write(t, filepath.Join(empty, "readme.txt"), nil)
	if got := MainFile(empty); got != "" {
		t.Errorf("no programs, main file = %s", got)
	}
	one := filepath.Join(t.TempDir(), "game.msi")
	write(t, one, []byte("x"))
	if MainFile(one) != one {
		t.Error("a download that's one file is its own main file")
	}
}

func check(t *testing.T, root string, o Options) Report {
	o.Root = root
	if o.defender == nil {
		o.defender = func(context.Context, string) ([]string, error) { return nil, nil }
	}
	if o.signer == nil {
		o.signer = func(string) (string, error) { return "", errors.New("not signed") }
	}
	return Check(context.Background(), o)
}

func find(r Report, check string) Finding {
	for _, f := range r.Findings {
		if f.Check == check {
			return f
		}
	}
	return Finding{}
}

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "setup.exe"), exe)
	sha, _ := FileSHA256(filepath.Join(dir, "setup.exe"))

	r := check(t, dir, Options{SHA256: strings.ToUpper(sha)})
	if r.Verdict != Clean || r.Main != "setup.exe" || r.SHA256 != sha || find(r, "integrity").Level != OK {
		t.Errorf("clean download: %+v", r)
	}
	if r := check(t, dir, Options{SHA256: strings.Repeat("0", 64)}); r.Verdict != Blocked || find(r, "integrity").Level != Block {
		t.Errorf("checksum mismatch: %+v", r)
	}

	threat := func(context.Context, string) ([]string, error) { return []string{"Trojan:Win32/Test"}, nil }
	if r := check(t, dir, Options{BlockDetections: true, defender: threat}); r.Verdict != Blocked || !strings.Contains(find(r, "defender").Text, "Trojan:Win32/Test") {
		t.Errorf("Defender detection, blocking: %+v", r)
	}
	if r := check(t, dir, Options{defender: threat}); r.Verdict != Caution {
		t.Errorf("Defender detection, warning: %+v", r)
	}
	gone := func(context.Context, string) ([]string, error) { return nil, ErrNoDefender }
	if r := check(t, dir, Options{defender: gone}); r.Verdict != Caution {
		t.Errorf("no Defender should be a warning: %+v", r)
	}
	signed := func(string) (string, error) { return "Example Games Ltd", nil }
	if f := find(check(t, dir, Options{signer: signed}), "signature"); f.Level != OK || !strings.Contains(f.Text, "Example Games") {
		t.Errorf("signature: %+v", f)
	}
}

func TestCheckFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "setup.exe"), []byte("not a program"))
	write(t, filepath.Join(dir, "readme.pdf.exe"), exe)
	write(t, filepath.Join(dir, "fix.vbs"), nil)
	r := check(t, dir, Options{})
	var texts []string
	for _, f := range r.Findings {
		if f.Check == "files" {
			texts = append(texts, f.Text)
		}
	}
	all := strings.Join(texts, "\n")
	for _, want := range []string{"isn't a Windows program", "named like a pdf file", "fix.vbs (VBScript)"} {
		if !strings.Contains(all, want) {
			t.Errorf("files findings lack %q:\n%s", want, all)
		}
	}
	if r.Verdict != Blocked {
		t.Errorf("an installer that isn't a program should block: %v", r.Verdict)
	}
}

func TestZipBomb(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "data.zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("zeros.bin")
	zero := make([]byte, 1<<20)
	for range 1100 { // 1.1 GB of zeros, compressed to about a megabyte
		w.Write(zero)
	}
	zw.Close()
	f.Close()
	if r := check(t, dir, Options{}); r.Verdict != Blocked {
		t.Errorf("zip bomb: %+v", r.Findings)
	}
}

func TestVirusTotal(t *testing.T) {
	answers := map[string]string{
		"aaa": `{"data":{"attributes":{"last_analysis_stats":{"malicious":0,"suspicious":0,"undetected":60,"harmless":10}}}}`,
		"bbb": `{"data":{"attributes":{"last_analysis_stats":{"malicious":1,"suspicious":0,"undetected":69}}}}`,
		"ccc": `{"data":{"attributes":{"last_analysis_stats":{"malicious":12,"undetected":58}}}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-apikey") != "k" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		a, ok := answers[strings.TrimPrefix(r.URL.Path, "/api/v3/files/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(a))
	}))
	defer srv.Close()
	vt := VirusTotal{Key: "k", Base: srv.URL}
	ctx := context.Background()
	for sha, want := range map[string]vtResult{
		"aaa": {Known: true, Engines: 70},
		"bbb": {Known: true, Malicious: 1, Engines: 70},
		"ccc": {Known: true, Malicious: 12, Engines: 70},
		"ddd": {},
	} {
		if got, err := vt.lookup(ctx, sha); err != nil || got != want {
			t.Errorf("%s: %+v, %v", sha, got, err)
		}
	}
	if _, err := (VirusTotal{Key: "bad", Base: srv.URL}).lookup(ctx, "aaa"); !errors.Is(err, errVTKey) {
		t.Errorf("bad key: %v", err)
	}
}

func TestParseThreats(t *testing.T) {
	out := "Scan starting...\r\nScan finished.\r\nScanning C:\\x found 1 threats.\r\n\r\n<===========================LIST OF DETECTED THREATS==========================>\r\n----------------------------- Threat information ------------------------------\r\nThreat                  : Virus:DOS/EICAR_Test_File\r\nResources               : 1 total\r\n    file                : C:\\x\\eicar.com\r\n"
	if got := parseThreats(out); len(got) != 1 || got[0] != "Virus:DOS/EICAR_Test_File" {
		t.Errorf("threats = %v", got)
	}
}

// WL_REAL_DEFENDER=1 scans a folder holding the EICAR test string with
// this PC's Defender (real-time protection may quarantine it first).
func TestRealDefender(t *testing.T) {
	if os.Getenv("WL_REAL_DEFENDER") == "" {
		t.Skip("set WL_REAL_DEFENDER=1 to scan with this PC's Defender")
	}
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean")
	write(t, filepath.Join(clean, "readme.txt"), []byte("hello"))
	threats, err := defenderScan(context.Background(), clean)
	if err != nil || len(threats) != 0 {
		t.Fatalf("clean folder: %v, %v", threats, err)
	}
	t.Log("clean folder: nothing found")
}
