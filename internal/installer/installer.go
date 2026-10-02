// Package installer installs a checked download without the installer's
// questions: it tells Inno Setup, NSIS and Windows Installer packages
// apart, builds their silent command lines (folder, language, log),
// unpacks archives, copies games that need no install, and finds how to
// uninstall what it installed.
package installer

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Kind is the sort of installer.
type Kind string

const (
	Inno     Kind = "inno"
	NSIS     Kind = "nsis"
	MSI      Kind = "msi"
	Archive  Kind = "archive"
	Portable Kind = "portable" // the game itself, ready to run: copied into place
	Other    Kind = "other"    // an installer Seaglass can't run silently: it shows its own questions
)

// archives are what Windows' own tar unpacks.
var archives = map[string]bool{".zip": true, ".7z": true, ".rar": true, ".tar": true, ".gz": true, ".tgz": true, ".xz": true}

// Detect works out how to install a download. main is the file the
// safety checks picked ("" for none); hint is the feed's installerType.
// It returns the kind and the file to run or unpack ("" for Portable).
func Detect(root, main, hint string) (Kind, string) {
	if k := Kind(hint); k == Portable {
		return Portable, ""
	}
	if main == "" {
		if a := findArchive(root); a != "" {
			return Archive, a
		}
		return Portable, ""
	}
	switch strings.ToLower(filepath.Ext(main)) {
	case ".msi":
		return MSI, main
	case ".exe":
	default:
		return Other, main
	}
	if k := Kind(hint); k == Inno || k == NSIS || k == MSI {
		return k, main
	}
	if k := sniff(main); k != "" {
		return k, main
	}
	name := strings.ToLower(filepath.Base(main))
	if !strings.Contains(name, "setup") && !strings.Contains(name, "install") {
		return Portable, "" // a game exe, not an installer
	}
	return Other, main
}

// sniff recognises Inno Setup and NSIS by the marks their setup programs
// carry near the start.
func sniff(exe string) Kind {
	f, err := os.Open(exe)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 16<<20))
	switch {
	case bytes.Contains(b, []byte("Inno Setup Setup Data")) || bytes.Contains(b, []byte("Inno Setup Messages")):
		return Inno
	case bytes.Contains(b, []byte("NullsoftInst")) || bytes.Contains(b, []byte("Nullsoft Install System")):
		return NSIS
	}
	// Inno names itself in the version information as UTF-16 text.
	if bytes.Contains(b, utf16("Inno Setup")) {
		return Inno
	}
	return ""
}

func utf16(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r), 0)
	}
	return b
}

func findArchive(root string) string {
	es, err := os.ReadDir(root)
	if err != nil {
		if archives[strings.ToLower(filepath.Ext(root))] {
			return root
		}
		return ""
	}
	var best string
	var size int64
	for _, e := range es {
		if e.IsDir() || !archives[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.Size() > size {
			best, size = filepath.Join(root, e.Name()), fi.Size()
		}
	}
	return best
}
