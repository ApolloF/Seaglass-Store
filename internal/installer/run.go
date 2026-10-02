package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"golang.org/x/sys/windows"
)

// Request is one install.
type Request struct {
	Ask      bool // show the installer for language/component selection
	Kind     Kind
	File     string // the installer or archive
	Root     string // the download (copied as it is for Portable)
	Dir      string // the game's folder
	Language string // Inno Setup's internal name; "" for the installer's default
	Log      string // where the installer writes its log (Inno, MSI)
}

// Command is a program and its command line.
type Command struct {
	Exe  string
	Args string // as the program reads it, quoted where needed
}

// command is how to run an installer without its questions.
func command(r Request) (Command, error) {
	switch r.Kind {
	case Inno:
		args := []string{"/NORESTART", "/SP-", "/DIR=" + quote(r.Dir)}
		if !r.Ask {
			args = append([]string{"/VERYSILENT", "/SUPPRESSMSGBOXES"}, args...)
		}
		if r.Language != "" && !r.Ask {
			args = append(args, "/LANG="+r.Language)
		}
		if r.Log != "" {
			args = append(args, "/LOG="+quote(r.Log))
		}
		return Command{r.File, strings.Join(args, " ")}, nil
	case NSIS:
		// /D= must come last and unquoted, spaces and all: NSIS reads the rest of the line.
		args := "/D=" + r.Dir
		if !r.Ask {
			args = "/S " + args
		}
		return Command{r.File, args}, nil
	case MSI:
		mode := "/qn"
		if r.Ask {
			mode = "/qf"
		}
		args := []string{"/i", quote(r.File), mode, "/norestart", "INSTALLDIR=" + quote(r.Dir), "TARGETDIR=" + quote(r.Dir), "INSTALLLOCATION=" + quote(r.Dir)}
		if r.Log != "" {
			args = append(args, "/l*v", quote(r.Log))
		}
		return Command{filepath.Join(platform.WindowsDir, "System32", "msiexec.exe"), strings.Join(args, " ")}, nil
	case Archive:
		return Command{filepath.Join(platform.WindowsDir, "System32", "tar.exe"), "-xf " + quote(r.File) + " -C " + quote(r.Dir)}, nil
	case Other:
		return Command{r.File, ""}, nil
	}
	return Command{}, fmt.Errorf("no command for %q", r.Kind)
}

func quote(s string) string { return `"` + s + `"` }

// copyGame copies a download that needs no install (a folder, or a
// single program) into the game's folder.
func copyGame(src, dir string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return copyFile(src, filepath.Join(dir, filepath.Base(src)))
	}
	// Files already there are replaced: an update goes over the old version.
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		return copyFile(p, dst)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Progress hears how far an install got: bytes in the game's folder, and
// how long nothing changed there.
type Progress func(bytes int64, idle time.Duration)

// Install installs a download. An installer that needs administrator
// rights asks Windows for them.
func Install(ctx context.Context, r Request, progress Progress) error {
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return err
	}
	if r.Kind == Portable {
		if err := copyGame(r.Root, r.Dir); err != nil {
			return fmt.Errorf("copying the game: %w", err)
		}
		return nil
	}
	c, err := command(r)
	if err != nil {
		return err
	}
	stop := watch(r.Dir, progress)
	defer stop()
	code, err := Run(ctx, c, filepath.Dir(r.File))
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the installer stopped with code %d%s", code, explain(r.Kind, code))
	}
	return nil
}

func explain(k Kind, code uint32) string {
	switch {
	case k == Inno && code == 2, k == MSI && code == 1602:
		return " (it was cancelled)"
	case k == MSI && code == 1603:
		return " (Windows Installer failed; its log says why)"
	case k == MSI && code == 3010:
		return " (it needs a restart)"
	}
	return ""
}

// Run runs a command and waits for it, as administrator when the program
// asks for that. It returns the exit code.
func Run(ctx context.Context, c Command, dir string) (uint32, error) {
	cmd := exec.CommandContext(ctx, c.Exe)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: quote(c.Exe) + " " + c.Args}
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exit):
		return uint32(exit.ExitCode()), nil
	case errors.Is(err, windows.ERROR_ELEVATION_REQUIRED):
		return platform.RunElevated(ctx, c.Exe, c.Args, dir)
	}
	return 0, err
}

// watch reports the game folder's size every few seconds until stopped.
func watch(dir string, progress Progress) func() {
	if progress == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		var last int64 = -1
		since := time.Now()
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			n := DirSize(dir)
			if n != last {
				last, since = n, time.Now()
			}
			progress(n, time.Since(since))
		}
	}()
	return func() { close(done) }
}

// DirSize adds up the sizes of the files in dir.
func DirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}
