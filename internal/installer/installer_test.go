package installer

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ApolloF/Seaglass/internal/platform"
)

func write(t *testing.T, p string, b []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	mz := []byte("MZ\x90\x00 padding ")
	inno := write(t, filepath.Join(dir, "inno", "setup.exe"), append(mz, []byte("....Inno Setup Setup Data (6.2.0)...")...))
	innoVer := write(t, filepath.Join(dir, "innover", "setup.exe"), append(mz, utf16("Inno Setup")...))
	nsis := write(t, filepath.Join(dir, "nsis", "setup.exe"), append(mz, []byte("\xef\xbe\xad\xdeNullsoftInst")...))
	other := write(t, filepath.Join(dir, "other", "install.exe"), mz)
	game := write(t, filepath.Join(dir, "game", "Game.exe"), mz)
	msi := write(t, filepath.Join(dir, "msi", "game.msi"), []byte("x"))
	arch := filepath.Join(dir, "arch")
	write(t, filepath.Join(arch, "small.zip"), []byte("z"))
	big := write(t, filepath.Join(arch, "game.7z"), []byte("zzzz"))

	for _, c := range []struct {
		root, main, hint string
		kind             Kind
		file             string
	}{
		{filepath.Dir(inno), inno, "", Inno, inno},
		{filepath.Dir(innoVer), innoVer, "", Inno, innoVer},
		{filepath.Dir(nsis), nsis, "", NSIS, nsis},
		{filepath.Dir(other), other, "", Other, other},
		{filepath.Dir(other), other, "inno", Inno, other},
		{filepath.Dir(game), game, "", Portable, ""},
		{filepath.Dir(msi), msi, "", MSI, msi},
		{arch, "", "", Archive, big},
		{filepath.Dir(inno), inno, "portable", Portable, ""},
	} {
		if k, f := Detect(c.root, c.main, c.hint); k != c.kind || f != c.file {
			t.Errorf("%s (hint %q): %s %s, want %s %s", c.main, c.hint, k, f, c.kind, c.file)
		}
	}
}

func TestCommand(t *testing.T) {
	dir := `D:\Games\Ember Crown`
	for _, c := range []struct {
		r    Request
		exe  string
		args string
	}{
		{Request{Kind: Inno, File: `C:\dl\setup.exe`, Dir: dir, Language: "german", Log: `C:\dl\install.log`}, `C:\dl\setup.exe`,
			`/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /DIR="D:\Games\Ember Crown" /LANG=german /LOG="C:\dl\install.log"`},
		{Request{Kind: Inno, File: `C:\dl\setup.exe`, Dir: dir}, `C:\dl\setup.exe`, `/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /DIR="D:\Games\Ember Crown"`},
		{Request{Kind: NSIS, File: `C:\dl\setup.exe`, Dir: dir}, `C:\dl\setup.exe`, `/S /D=D:\Games\Ember Crown`},
		{Request{Kind: MSI, File: `C:\dl\game.msi`, Dir: dir}, filepath.Join(platform.WindowsDir, "System32", "msiexec.exe"),
			`/i "C:\dl\game.msi" /qn /norestart INSTALLDIR="D:\Games\Ember Crown" TARGETDIR="D:\Games\Ember Crown" INSTALLLOCATION="D:\Games\Ember Crown"`},
		{Request{Kind: Archive, File: `C:\dl\game.7z`, Dir: dir}, filepath.Join(platform.WindowsDir, "System32", "tar.exe"), `-xf "C:\dl\game.7z" -C "D:\Games\Ember Crown"`},
	} {
		got, err := command(c.r)
		if err != nil || got.Exe != c.exe || got.Args != c.args {
			t.Errorf("%s:\n got %q %q (%v)\nwant %q %q", c.r.Kind, got.Exe, got.Args, err, c.exe, c.args)
		}
	}
}

func TestInnoLanguage(t *testing.T) {
	for in, want := range map[string]string{"English": "english", "Brazilian Portuguese": "brazilianportuguese", "Portuguese (Brazil)": "brazilianportuguese",
		"Simplified Chinese": "chinesesimplified", "Klingon": ""} {
		if got := InnoLanguage(in); got != want {
			t.Errorf("InnoLanguage(%q) = %q, want %q", in, got, want)
		}
	}
	got := parseLanguages("Listing languages in setup.exe:\n - english: English\n - german: Deutsch\r\nfrench (Français)\n")
	if len(got) != 3 || got["german"] != "Deutsch" || got["french"] != "Français" {
		t.Errorf("parsed %v", got)
	}
}

func TestQuietCommand(t *testing.T) {
	dir := `D:\Games\Ember Crown`
	for _, c := range []struct {
		e    uninstallEntry
		want string
	}{
		{uninstallEntry{Dir: `D:\Games\Other`, Uninstall: `"D:\Games\Other\unins000.exe"`}, ""},
		{uninstallEntry{Dir: dir + `\`, Uninstall: `"D:\Games\Ember Crown\unins000.exe"`}, `"D:\Games\Ember Crown\unins000.exe" /VERYSILENT /SUPPRESSMSGBOXES /NORESTART`},
		{uninstallEntry{Uninstall: `"D:\Games\Ember Crown\unins000.exe"`, Quiet: `"D:\Games\Ember Crown\unins000.exe" /SILENT`}, `"D:\Games\Ember Crown\unins000.exe" /SILENT`},
		{uninstallEntry{Dir: dir, Uninstall: `MsiExec.exe /I{12345678-1234-1234-1234-123456789ABC}`},
			`"` + filepath.Join(platform.WindowsDir, "System32", "msiexec.exe") + `" /x {12345678-1234-1234-1234-123456789ABC} /qn /norestart`},
		{uninstallEntry{Uninstall: `D:\Games\Ember Crown\Uninstall.exe`}, `"D:\Games\Ember Crown\Uninstall.exe" /S _?=D:\Games\Ember Crown`},
	} {
		if got := quietCommand(c.e, dir); got != c.want {
			t.Errorf("%+v:\n got %q\nwant %q", c.e, got, c.want)
		}
	}
}

func TestRemoveGameDirRefusesWhatIsntAGame(t *testing.T) {
	for _, d := range []string{`C:\`, `D:\Games`, platform.Profile, platform.ProgramFiles, "relative"} {
		if err := removeGameDir(d); err == nil {
			t.Errorf("would delete %s", d)
		}
	}
}

func TestInstallPortableAndArchive(t *testing.T) {
	ctx := context.Background()
	src := t.TempDir()
	write(t, filepath.Join(src, "Game.exe"), []byte("MZ"))
	write(t, filepath.Join(src, "data", "level1.pak"), []byte("level"))
	dst := filepath.Join(t.TempDir(), "Game")
	if err := Install(ctx, Request{Kind: Portable, Root: src, Dir: dst}, nil); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "data", "level1.pak")); err != nil || string(b) != "level" {
		t.Errorf("portable copy: %q, %v", b, err)
	}

	z := filepath.Join(t.TempDir(), "game.zip")
	f, err := os.Create(z)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("Game/Game.exe")
	w.Write([]byte("MZ"))
	zw.Close()
	f.Close()
	out := filepath.Join(t.TempDir(), "Unzipped")
	if err := Install(ctx, Request{Kind: Archive, File: z, Dir: out}, nil); err != nil {
		t.Fatal(err)
	}
	if !platform.IsFile(filepath.Join(out, "Game", "Game.exe")) {
		t.Error("the archive wasn't unpacked")
	}
	if n := DirSize(out); n != 2 {
		t.Errorf("unpacked size %d", n)
	}
	if err := Uninstall(ctx, "", out); err != nil || platform.IsDir(out) {
		t.Errorf("uninstall of an unpacked game: %v (still there: %v)", err, platform.IsDir(out))
	}
	if !strings.HasPrefix(out, os.TempDir()) {
		t.Skip()
	}
}
