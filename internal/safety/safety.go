// Package safety checks a finished download before anything in it runs:
// that it's what the feed promised, what's inside, Microsoft Defender's
// scan and, with the person's own API key, what VirusTotal knows about it.
// It can't prove a download is safe; it catches what these checks know.
package safety

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Level is how much a finding matters.
type Level string

const (
	OK    Level = "ok"    // a check passed
	Info  Level = "info"  // worth knowing, not a problem
	Warn  Level = "warn"  // the person decides
	Block Level = "block" // not installed unless the person insists
)

func (l Level) rank() int { return slices.Index([]Level{OK, Info, Warn, Block}, l) }

// Verdict is what a report comes to.
type Verdict string

const (
	Clean   Verdict = "clean"
	Caution Verdict = "warn"
	Blocked Verdict = "block"
)

// Finding is one check's result.
type Finding struct {
	Check string `json:"check"` // integrity, signature, files, defender, virustotal
	Level Level  `json:"level"`
	Text  string `json:"text"`
}

// Report is everything the checks found.
type Report struct {
	PayloadSkipped bool      `json:"payloadSkipped,omitempty"`
	Verdict        Verdict   `json:"verdict"`
	Findings       []Finding `json:"findings"`
	Main           string    `json:"main,omitempty"` // the installer (or game exe), relative to the download
	SHA256         string    `json:"sha256,omitempty"`
	Checked        int64     `json:"checked"`              // unix seconds
	Overridden     bool      `json:"overridden,omitempty"` // the person chose to install it anyway
}

// verdict is the worst finding's level.
func verdict(fs []Finding) Verdict {
	worst := OK
	for _, f := range fs {
		if f.Level.rank() > worst.rank() {
			worst = f.Level
		}
	}
	switch worst {
	case Block:
		return Blocked
	case Warn:
		return Caution
	}
	return Clean
}

// MainFile picks the file to install from in a download: an .msi or a
// setup program at the top, else the largest .exe near the top. "" when
// there's none (an archive, or only data).
func MainFile(root string) string {
	if fi, err := os.Stat(root); err == nil && !fi.IsDir() {
		return root
	}
	type cand struct {
		path  string
		score int
		size  int64
	}
	var cs []cand
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		depth := strings.Count(rel, string(filepath.Separator))
		if d.IsDir() {
			if depth >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		ext := filepath.Ext(name)
		if ext != ".exe" && ext != ".msi" {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		score := 0
		switch {
		case ext == ".msi":
			score = 3
		case name == "setup.exe" || name == "install.exe" || strings.HasPrefix(name, "setup"):
			score = 4
		case strings.Contains(name, "setup") || strings.Contains(name, "install"):
			score = 2
		}
		if strings.Contains(name, "redist") || strings.Contains(name, "vcredist") || strings.Contains(name, "directx") || strings.HasPrefix(name, "unins") {
			score = -5 // runtimes and uninstallers aren't the game
		}
		score -= depth * 2
		cs = append(cs, cand{p, score, fi.Size()})
		return nil
	})
	if len(cs) == 0 {
		return ""
	}
	slices.SortFunc(cs, func(a, b cand) int { return cmp.Or(b.score-a.score, cmp.Compare(b.size, a.size)) })
	return cs[0].path
}

// FileSHA256 hashes a file.
func FileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
