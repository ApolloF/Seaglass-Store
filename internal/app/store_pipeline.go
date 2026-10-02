package app

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/installer"
	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/safety"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
)

// virusTotalSecret holds the person's VirusTotal API key.
const virusTotalSecret = "store-virustotal"

// installStall is how long an installer may show no progress before the
// download says it seems stuck.
const installStall = 10 * time.Minute

// pipeline takes finished downloads through the safety checks and, when
// asked, the install: one check and one install at a time.
type pipeline struct {
	st     *storeState
	mu     sync.Mutex
	busy   map[string]bool // downloads being checked or installed
	auto   map[string]bool // downloads whose automatic install was started once
	scanMu sync.Mutex
	instMu sync.Mutex
}

func newPipeline(st *storeState) *pipeline {
	return &pipeline{st: st, busy: map[string]bool{}, auto: map[string]bool{}}
}

// claim marks a download busy; false when it already is.
func (p *pipeline) claim(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.busy[id] {
		return false
	}
	p.busy[id] = true
	return true
}

// working reports whether a download is being checked or installed.
func (p *pipeline) working(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.busy[id]
}

func (p *pipeline) release(id string) {
	p.mu.Lock()
	delete(p.busy, id)
	p.mu.Unlock()
}

// advance starts what finished downloads need next.
func (p *pipeline) advance(all []jobs.Job) {
	for _, j := range all {
		switch {
		case j.State == jobs.Downloaded && j.Safety == nil && j.Name != "":
			if p.claim(j.ID) {
				go p.scan(j.ID)
			}
		case j.State == jobs.Downloaded && j.Safety != nil && j.AutoInstall && j.Safety.Verdict == safety.Clean:
			p.mu.Lock()
			first := !p.auto[j.ID]
			p.auto[j.ID] = true
			p.mu.Unlock()
			if first && p.claim(j.ID) {
				go p.install(j.ID)
			}
		}
	}
}

// root is a download's files: its folder, or its one file.
func root(j jobs.Job) string { return filepath.Join(j.SavePath, j.Name) }

func (p *pipeline) update(id string, fn func(*jobs.Job)) {
	_, _ = p.st.jobs.Update(id, func(j *jobs.Job) bool { fn(j); return true })
	p.st.c.emit(EventStoreJobs, p.st.jobs.All())
}

func (p *pipeline) scan(id string) {
	defer p.release(id)
	p.scanMu.Lock()
	defer p.scanMu.Unlock()
	j, ok := p.st.jobs.Get(id)
	if !ok || j.State != jobs.Downloaded {
		return
	}
	p.update(id, func(j *jobs.Job) { j.State, j.Error = jobs.Scanning, "" })
	cfg := p.st.c.Settings.Get().Store
	o := safety.Options{Root: root(j), SHA256: j.SHA256, BlockDetections: cfg.BlockDetections, DisablePayloadScanning: cfg.DisablePayloadScanning}
	if key := platform.LoadSecret(virusTotalSecret); key != "" {
		o.VirusTotal = &safety.VirusTotal{Key: key}
	}
	ctx, cancel := context.WithTimeout(p.st.c.ctx, time.Hour)
	defer cancel()
	r := safety.Check(ctx, o)
	if p.st.c.ctx.Err() != nil {
		p.update(id, func(j *jobs.Job) { j.State = jobs.Downloaded }) // checked again at the next start
		return
	}
	logx.Printf("store: %s checked: %s", j.Title, r.Verdict)
	for _, f := range r.Findings {
		if f.Level == safety.Warn || f.Level == safety.Block {
			logx.Printf("store:   %s %s: %s", f.Level, f.Check, f.Text)
		}
	}
	p.update(id, func(j *jobs.Job) {
		j.Safety = &r
		j.State = jobs.Downloaded
		if r.Verdict == safety.Blocked {
			j.State = jobs.Blocked
		}
	})
	p.st.wake()
}

// install runs a checked download's installer and adds the game to the
// library.
func (p *pipeline) install(id string) {
	defer p.release(id)
	p.instMu.Lock()
	defer p.instMu.Unlock()
	if !p.st.c.waitIdle(p.st.c.ctx) {
		return
	}
	j, ok := p.st.jobs.Get(id)
	if !ok || j.Safety == nil || (j.State != jobs.Downloaded && j.State != jobs.Failed) {
		return
	}
	if j.Safety.Verdict == safety.Blocked && !j.Safety.Overridden {
		return
	}
	if j.InstallDir == "" {
		j.InstallDir = filepath.Join(gamesDir(p.st.c.Settings.Get().Store), folderName(j.Title))
	}
	if j.Replaces == "" { // an update goes over the version before it
		if err := checkInstallDir(j.InstallDir); err != nil {
			p.fail(id, err)
			return
		}
	}
	rt := root(j)
	main := ""
	if j.Safety.Main != "" {
		main = filepath.Join(rt, j.Safety.Main)
		if fi, err := os.Stat(rt); err == nil && !fi.IsDir() {
			main = rt
		}
	}
	kind, file := installer.Detect(rt, main, j.Installer)
	req := installer.Request{Kind: kind, File: file, Root: rt, Dir: j.InstallDir,
		Log: filepath.Join(platform.CacheDir("store", "logs"), j.ID+".log"), Ask: j.AskInstaller}
	if kind == installer.Inno && !j.AskInstaller {
		req.Language = j.SetupLanguage
		if req.Language == "" {
			if tool, err := exec.LookPath("innoextract.exe"); err == nil {
				languages, err := installer.InnoLanguages(p.st.c.ctx, tool, file)
				if err != nil {
					logx.Printf("store: installer language inspection: %v", err)
				} else if _, ok := languages["english"]; ok {
					req.Language = "english"
				}
			}
		}
	}
	p.update(id, func(j *jobs.Job) {
		j.State, j.Error, j.Installer, j.InstallDone, j.Stalled = jobs.Installing, "", string(kind), 0, false
		j.InstallDir = req.Dir
	})
	logx.Printf("store: installing %s (%s) into %s", j.Title, kind, req.Dir)
	err := installer.Install(p.st.c.ctx, req, func(n int64, idle time.Duration) {
		stalled := idle > installStall
		_, _ = p.st.jobs.Update(id, func(j *jobs.Job) bool {
			changed := j.Stalled != stalled
			j.InstallDone, j.Stalled = n, stalled
			return changed
		})
		p.st.c.emit(EventStoreJobs, p.st.jobs.All())
	})
	if err != nil {
		if errors.Is(err, platform.ErrCancelled) {
			err = errors.New("the installer needs administrator rights, and Windows' prompt was declined")
		}
		p.fail(id, fmt.Errorf("installing failed: %w", err))
		return
	}
	dir := req.Dir
	if kind == installer.Other && installer.DirSize(dir) == 0 {
		_ = os.Remove(dir) // the installer put the game where it chose
		dir = ""
	}
	un := ""
	if dir != "" {
		un = installer.Uninstaller(dir, kind)
	}
	p.update(id, func(j *jobs.Job) {
		j.State, j.InstalledAt, j.Uninstaller, j.InstallDir, j.Stalled = jobs.Installed, time.Now().Unix(), un, dir, false
	})
	logx.Printf("store: %s installed", j.Title)
	if old, ok := p.st.jobs.Get(j.Replaces); ok && old.State == jobs.Installed {
		// The newer version took its place: forget the older download (and its files).
		if old.Hash != "" {
			_ = p.st.removeTorrent(old, true)
		}
		_ = p.st.jobs.Delete(old.ID)
	}
	p.addToLibrary(dir)
	if j.Hash != "" && !p.st.c.Settings.Get().Store.KeepDownloads {
		if err := p.st.removeTorrent(j, true); err != nil {
			logx.Printf("store: removing the download of %s: %v", j.Title, err)
		} else {
			p.update(id, func(j *jobs.Job) { j.Hash, j.Seeding, j.UpSpeed = "", false, 0 })
		}
	}
}

// addToLibrary makes sure the folder the game went into is one the
// library looks in, and looks.
func (p *pipeline) addToLibrary(dir string) {
	c := p.st.c
	if parent := filepath.Dir(dir); dir != "" && filepath.Dir(parent) != parent {
		cfg := c.Settings.Get()
		inFolders := slices.ContainsFunc(cfg.Folders, func(f string) bool { return platform.Key(f) == platform.Key(parent) })
		if !inFolders {
			if _, err := c.updateSettings(func(v *settings.Settings) { v.Folders = append(v.Folders, parent) }); err != nil {
				logx.Printf("store: adding %s to the game folders: %v", parent, err)
			}
		}
	}
	c.RequestScan()
}

func (p *pipeline) fail(id string, err error) {
	logx.Printf("store: %v", err)
	p.update(id, func(j *jobs.Job) { j.State, j.Error, j.Stalled = jobs.Failed, err.Error(), false })
}

// uninstall removes an installed game. The download stays when it's
// still on disk; otherwise the download is forgotten too.
func (p *pipeline) uninstall(id string) error {
	if !p.claim(id) {
		return errors.New("that game is busy")
	}
	defer p.release(id)
	j, ok := p.st.jobs.Get(id)
	if !ok || j.State != jobs.Installed {
		return errors.New("that game isn't installed by the store")
	}
	if j.InstallDir == "" && j.Uninstaller == "" {
		return errors.New("the installer put the game somewhere Seaglass doesn't know; uninstall it from Windows' Settings, Apps")
	}
	ctx, cancel := context.WithTimeout(p.st.c.ctx, time.Hour)
	defer cancel()
	if err := installer.Uninstall(ctx, j.Uninstaller, j.InstallDir); err != nil {
		if errors.Is(err, platform.ErrCancelled) {
			err = errors.New("the uninstaller needs administrator rights, and Windows' prompt was declined")
		}
		return err
	}
	logx.Printf("store: %s uninstalled", j.Title)
	if j.Hash != "" && (platform.IsDir(root(j)) || platform.IsFile(root(j))) {
		p.update(id, func(j *jobs.Job) { j.State, j.Uninstaller, j.InstalledAt = jobs.Downloaded, "", 0 })
	} else if err := p.st.jobs.Delete(id); err != nil {
		return err
	}
	p.st.c.emit(EventStoreJobs, p.st.jobs.All())
	p.st.c.RequestScan()
	return nil
}

// sandboxExe is Windows Sandbox, when the feature is turned on.
func sandboxExe() string {
	p := filepath.Join(platform.WindowsDir, "System32", "WindowsSandbox.exe")
	if platform.IsFile(p) {
		return p
	}
	return ""
}

// sandboxConfig is a Windows Sandbox configuration that shows a download,
// read-only and without network, on the sandbox's desktop.
func sandboxConfig(folder string) ([]byte, error) {
	var esc strings.Builder
	if err := xml.EscapeText(&esc, []byte(folder)); err != nil {
		return nil, err
	}
	return []byte(`<Configuration>
  <Networking>Disable</Networking>
  <MappedFolders>
    <MappedFolder>
      <HostFolder>` + esc.String() + `</HostFolder>
      <SandboxFolder>C:\Users\WDAGUtilityAccount\Desktop\Download</SandboxFolder>
      <ReadOnly>true</ReadOnly>
    </MappedFolder>
  </MappedFolders>
</Configuration>
`), nil
}
