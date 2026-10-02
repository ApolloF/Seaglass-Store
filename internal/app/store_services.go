package app

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/torrent"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// StoreService is the experimental store. Every method fails with
// ErrStoreDisabled while the store is turned off.
type StoreService struct{ c *Core }

// NewStoreService binds the store to core.
func NewStoreService(c *Core) *StoreService { return &StoreService{c} }

func (s *StoreService) on() error {
	if !s.c.Settings.Get().ExperimentalStore {
		return ErrStoreDisabled
	}
	return nil
}

// Engine returns the download engine's status.
func (s *StoreService) Engine() EngineStatus { return s.c.store.getStatus() }

// StartEngine starts the download engine (to list network interfaces,
// say) when it isn't running; it stops again when idle.
func (s *StoreService) StartEngine() (EngineStatus, error) {
	if err := s.on(); err != nil {
		return s.Engine(), err
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 45*time.Second)
	defer cancel()
	if _, err := s.c.store.engine(ctx); err != nil {
		s.c.store.setStatus(func(st *EngineStatus) { st.Running, st.Error = false, err.Error() })
		return s.Engine(), err
	}
	s.c.store.setStatus(func(st *EngineStatus) { st.Running, st.Error = true, "" })
	return s.Engine(), nil
}

// Interfaces lists the network interfaces downloads can be bound to.
func (s *StoreService) Interfaces() ([]torrent.Interface, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 45*time.Second)
	defer cancel()
	eng, err := s.c.store.engine(ctx)
	if err != nil {
		return nil, err
	}
	return eng.Interfaces(ctx)
}

// Addresses lists an interface's addresses.
func (s *StoreService) Addresses(iface string) ([]string, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 45*time.Second)
	defer cancel()
	eng, err := s.c.store.engine(ctx)
	if err != nil {
		return nil, err
	}
	return eng.Addresses(ctx, iface)
}

// HasProxyPassword reports whether a proxy password is saved.
func (s *StoreService) HasProxyPassword() bool { return platform.LoadSecret(proxySecret) != "" }

// SetProxyPassword stores the proxy's password encrypted for this Windows
// account ("" removes it) and applies it.
func (s *StoreService) SetProxyPassword(password string) error {
	if err := s.on(); err != nil {
		return err
	}
	if err := platform.SaveSecret(proxySecret, password); err != nil {
		return err
	}
	if s.c.store.engineRunning() {
		go s.c.store.applyNetwork()
	}
	return nil
}

// ChooseQBittorrent asks where qbittorrent.exe is.
func (s *StoreService) ChooseQBittorrent() (settings.Settings, error) {
	if err := s.on(); err != nil {
		return s.c.Settings.Get(), err
	}
	p, err := application.Get().Dialog.OpenFile().
		SetTitle("Choose qbittorrent.exe").
		CanChooseFiles(true).CanChooseDirectories(false).
		AddFilter("qBittorrent", "qbittorrent.exe").
		PromptForSingleSelection()
	if err != nil || p == "" {
		return s.c.Settings.Get(), err
	}
	v := s.c.Settings.Get()
	v.Store.QBittorrent = p
	return NewSettingsService(s.c).Save(v)
}

// GetQBittorrent opens qBittorrent's download page.
func (s *StoreService) GetQBittorrent() error {
	return platform.OpenWebPage("https://www.qbittorrent.org/download")
}

// DownloadsFolder is where downloads go.
func (s *StoreService) DownloadsFolder() string { return downloadsDir(s.c.Settings.Get().Store) }

// ChooseDownloadsFolder asks where downloads go.
func (s *StoreService) ChooseDownloadsFolder() (settings.Settings, error) {
	if err := s.on(); err != nil {
		return s.c.Settings.Get(), err
	}
	p, err := application.Get().Dialog.OpenFile().
		SetTitle("Choose where downloads go").
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
		PromptForSingleSelection()
	if err != nil || p == "" {
		return s.c.Settings.Get(), err
	}
	v := s.c.Settings.Get()
	v.Store.Downloads = p
	return NewSettingsService(s.c).Save(v)
}

// Downloads returns the downloads, oldest first.
func (s *StoreService) Downloads() []jobs.Job { return s.c.store.jobs.All() }

// AddDownload queues a magnet link or a link to a .torrent file. title
// may be empty: the link's own name is used.
func (s *StoreService) AddDownload(source, title string) (jobs.Job, error) {
	if err := s.on(); err != nil {
		return jobs.Job{}, err
	}
	src, title, err := downloadSource(source, title)
	if err != nil {
		return jobs.Job{}, err
	}
	dir := downloadsDir(s.c.Settings.Get().Store)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return jobs.Job{}, err
	}
	j, err := s.c.store.jobs.Add(title, src, dir, time.Now())
	if err == nil {
		s.c.store.wake()
	}
	return j, err
}

// DownloadAction pauses, resumes (or retries) or removes a download.
// Removing deletes its downloaded files too when deleteFiles is set.
func (s *StoreService) DownloadAction(id string, action jobs.Action, deleteFiles bool) error {
	if err := s.on(); err != nil {
		return err
	}
	j, ok := s.c.store.jobs.Get(id)
	if !ok {
		return jobs.ErrNotFound
	}
	if !jobs.Can(j.State, action) {
		return nil // already done (a double click)
	}
	switch action {
	case jobs.Pause:
		_, err := s.c.store.jobs.Update(id, func(j *jobs.Job) bool { j.State = jobs.Paused; return true })
		s.c.store.wake()
		return err
	case jobs.Resume:
		_, err := s.c.store.jobs.Update(id, func(j *jobs.Job) bool {
			j.State, j.Error = jobs.Queued, ""
			return true
		})
		s.c.store.wake()
		return err
	}
	// The engine may have it before its hash reached the download.
	if j.Hash != "" || s.c.store.engineRunning() {
		if err := s.c.store.removeTorrent(j, deleteFiles); err != nil {
			return err
		}
	}
	if err := s.c.store.jobs.Delete(id); err != nil {
		return err
	}
	s.c.emit(EventStoreJobs, s.c.store.jobs.All())
	return nil
}

// ShowDownload opens a download's folder in Explorer.
func (s *StoreService) ShowDownload(id string) error {
	j, ok := s.c.store.jobs.Get(id)
	if !ok {
		return jobs.ErrNotFound
	}
	dir := j.SavePath
	if d := filepath.Join(j.SavePath, j.Name); j.Name != "" && platform.IsDir(d) {
		dir = d
	}
	return platform.ShowInExplorer(dir)
}
