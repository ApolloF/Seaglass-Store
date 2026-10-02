package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
	"golang.org/x/sys/windows/registry"
)

// Uninstaller finds how to remove a game installed into dir: Windows'
// installed apps list first (what the installer registered), then the
// uninstaller Inno Setup or NSIS leaves in the folder. "" means the
// folder is all there is (archives, portable games).
func Uninstaller(dir string, kind Kind) string {
	for _, e := range uninstallEntries() {
		if c := quietCommand(e, dir); c != "" {
			return c
		}
	}
	switch kind {
	case Inno:
		if p := filepath.Join(dir, "unins000.exe"); platform.IsFile(p) {
			return quote(p) + " /VERYSILENT /SUPPRESSMSGBOXES /NORESTART"
		}
	case NSIS:
		for _, n := range []string{"uninstall.exe", "Uninstall.exe", "uninst.exe", "Uninst.exe"} {
			if p := filepath.Join(dir, n); platform.IsFile(p) {
				return quote(p) + " /S _?=" + dir
			}
		}
	}
	return ""
}

type uninstallEntry struct {
	Dir, Uninstall, Quiet string
}

func uninstallEntries() []uninstallEntry {
	var out []uninstallEntry
	for _, src := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	} {
		k, err := registry.OpenKey(src.root, src.path, registry.ENUMERATE_SUB_KEYS|registry.READ)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, n := range names {
			sk, err := registry.OpenKey(src.root, src.path+`\`+n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			dir, _, _ := sk.GetStringValue("InstallLocation")
			un, _, _ := sk.GetStringValue("UninstallString")
			quiet, _, _ := sk.GetStringValue("QuietUninstallString")
			sk.Close()
			if un != "" || quiet != "" {
				out = append(out, uninstallEntry{Dir: dir, Uninstall: un, Quiet: quiet})
			}
		}
	}
	return out
}

var reInnoUninstaller = regexp.MustCompile(`^unins\d{3}\.exe$`)

var reMsiProduct = regexp.MustCompile(`(?i)msiexec(?:\.exe)?"?\s+/[ix]\s*(\{[0-9A-F-]{36}\})`)

// quietCommand is the silent uninstall command of an entry for a game in
// dir; "" when the entry is for something else.
func quietCommand(e uninstallEntry, dir string) string {
	exe := commandExe(e.Uninstall)
	mine := (e.Dir != "" && platform.Within(dir, strings.Trim(e.Dir, `"`))) || (exe != "" && platform.Within(dir, exe))
	if !mine {
		return ""
	}
	if e.Quiet != "" {
		return e.Quiet
	}
	if m := reMsiProduct.FindStringSubmatch(e.Uninstall); m != nil {
		return quote(filepath.Join(platform.WindowsDir, "System32", "msiexec.exe")) + " /x " + m[1] + " /qn /norestart"
	}
	name := strings.ToLower(filepath.Base(exe))
	switch {
	case reInnoUninstaller.MatchString(name):
		return quote(exe) + " /VERYSILENT /SUPPRESSMSGBOXES /NORESTART"
	case strings.Contains(name, "uninst"):
		return quote(exe) + " /S _?=" + filepath.Dir(exe)
	}
	return e.Uninstall // not known to run silently: it shows its own questions
}

// commandExe is the program a command line runs.
func commandExe(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, `"`) {
		if end := strings.Index(cmd[1:], `"`); end >= 0 {
			return cmd[1 : end+1]
		}
		return ""
	}
	if i := strings.Index(strings.ToLower(cmd), ".exe"); i >= 0 {
		return cmd[:i+4]
	}
	return cmd
}

// Uninstall runs an uninstall command, or, when there's none, deletes the
// game's folder (it only ever held what Seaglass put there).
func Uninstall(ctx context.Context, command, dir string) error {
	if command == "" {
		return removeGameDir(dir)
	}
	exe := commandExe(command)
	args := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(command), quote(exe)), exe))
	code, err := Run(ctx, Command{exe, args}, filepath.Dir(exe))
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the uninstaller stopped with code %d", code)
	}
	// Inno and NSIS (run in place) leave settings files and empty folders.
	_ = removeGameDir(dir)
	return nil
}

func removeGameDir(dir string) error {
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) || filepath.Dir(dir) == dir || filepath.Dir(filepath.Dir(dir)) == filepath.Dir(dir) {
		return errors.New("won't delete a drive or a folder at its top")
	}
	for _, protected := range []string{platform.WindowsDir, platform.ProgramFiles, platform.ProgramFilesX86, platform.Profile, platform.Documents, platform.Desktop, platform.Downloads} {
		if protected != "" && platform.Key(protected) == platform.Key(dir) {
			return errors.New("won't delete " + dir)
		}
	}
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
