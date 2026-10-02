package safety

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Limits for what a download may hold before it's called out.
const (
	maxFiles = 200_000
	maxRatio = 200      // uncompressed to compressed, inside a zip
	maxUnzip = 1 << 41 // 2 TB unpacked: no game is that large
)

// risky are file types that run code without looking like a game.
var risky = map[string]string{
	".scr": "screen saver", ".pif": "program shortcut", ".com": "DOS program", ".vbs": "VBScript", ".vbe": "VBScript",
	".js": "JScript", ".jse": "JScript", ".wsf": "Windows script", ".hta": "HTML application", ".ps1": "PowerShell script",
	".lnk": "shortcut", ".reg": "registry file", ".msc": "management console file",
}

// documents are what a disguised program pretends to be ("readme.pdf.exe").
var documents = []string{".pdf", ".txt", ".doc", ".docx", ".jpg", ".png", ".mp4", ".nfo", ".html"}

// checkFiles looks at what's in the download and how the main file looks.
func checkFiles(root, main string) []Finding {
	var out []Finding
	n := 0
	var notes []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		n++
		name := strings.ToLower(d.Name())
		ext := filepath.Ext(name)
		rel, _ := filepath.Rel(root, p)
		if kind, ok := risky[ext]; ok && len(notes) < 10 {
			notes = append(notes, fmt.Sprintf("%s (%s)", rel, kind))
		}
		if ext == ".exe" || ext == ".bat" || ext == ".cmd" {
			inner := filepath.Ext(strings.TrimSuffix(name, ext))
			for _, doc := range documents {
				if inner == doc {
					out = append(out, Finding{"files", Warn, fmt.Sprintf("%s is a program named like a %s file.", rel, strings.TrimPrefix(doc, "."))})
				}
			}
		}
		if ext == ".zip" {
			out = append(out, checkZip(p, rel)...)
		}
		return nil
	})
	if n > maxFiles {
		out = append(out, Finding{"files", Warn, fmt.Sprintf("The download holds %d files: unusually many.", n)})
	}
	if len(notes) > 0 {
		out = append(out, Finding{"files", Warn, "Files that run code but aren't programs: " + strings.Join(notes, ", ") + "."})
	}
	if main != "" && strings.EqualFold(filepath.Ext(main), ".exe") {
		if b, err := readStart(main, 2); err != nil || !bytes.Equal(b, []byte("MZ")) {
			out = append(out, Finding{"files", Block, "The installer isn't a Windows program."})
		}
	}
	if len(out) == 0 {
		out = append(out, Finding{"files", OK, fmt.Sprintf("%d files, nothing unusual among them.", n)})
	}
	return out
}

// checkZip calls out archives that unpack to far more than they hold.
func checkZip(p, rel string) []Finding {
	r, err := zip.OpenReader(p)
	if err != nil {
		return nil
	}
	defer r.Close()
	var packed, unpacked uint64
	for _, f := range r.File {
		packed += f.CompressedSize64
		unpacked += f.UncompressedSize64
	}
	if unpacked > maxUnzip || (packed > 0 && unpacked/packed > maxRatio && unpacked > 1<<30) {
		return []Finding{{"files", Block, fmt.Sprintf("%s unpacks to %d GB from %d MB: built to fill the disk.", rel, unpacked>>30, packed>>20)}}
	}
	return nil
}

func readStart(p string, n int) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := make([]byte, n)
	_, err = f.Read(b)
	return b, err
}
