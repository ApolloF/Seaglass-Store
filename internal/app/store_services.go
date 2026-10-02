package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/safety"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
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

// HoldDownloads pauses every download until resumed (the tray does the same).
func (s *StoreService) HoldDownloads(on bool) { s.c.store.setHold(on) }

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
	j, err := s.c.store.jobs.Add(jobs.Job{Title: title, Source: src, SavePath: dir, Language: "English"}, time.Now())
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
	if !j.Can(action) {
		return nil // already done (a double click)
	}
	switch action {
	case jobs.Pause:
		_, err := s.c.store.jobs.Update(id, func(j *jobs.Job) bool { j.State = jobs.Paused; return true })
		s.c.store.wake()
		return err
	case jobs.Resume:
		if j.Safety != nil { // it failed installing: install again
			return s.startInstall(j)
		}
		_, err := s.c.store.jobs.Update(id, func(j *jobs.Job) bool {
			j.State, j.Error = jobs.Queued, ""
			return true
		})
		s.c.store.wake()
		return err
	case jobs.Install:
		return s.startInstall(j)
	case jobs.Recheck:
		_, err := s.c.store.jobs.Update(id, func(j *jobs.Job) bool {
			j.State, j.Safety = jobs.Downloaded, nil
			return true
		})
		s.c.store.wake()
		return err
	case jobs.Uninstall:
		return s.c.store.pipe.uninstall(id)
	case jobs.Allow:
		return errors.New("installing a blocked download needs AllowDownload")
	}
	// The engine may have it before its hash reached the download.
	if j.Hash != "" || (s.c.store.engineRunning() && j.State != jobs.Installed) {
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

func (s *StoreService) startInstall(j jobs.Job) error {
	if j.Safety == nil {
		return errors.New("the download hasn't been checked yet")
	}
	if j.Safety.Verdict == safety.Blocked && !j.Safety.Overridden {
		return errors.New("the safety checks blocked this download")
	}
	if !s.c.store.pipe.claim(j.ID) {
		return nil // already installing
	}
	go s.c.store.pipe.install(j.ID)
	return nil
}

// AllowDownload lets a download the safety checks blocked be installed
// after all. confirm must be the download's title, typed by the person.
func (s *StoreService) AllowDownload(id, confirm string) error {
	if err := s.on(); err != nil {
		return err
	}
	j, ok := s.c.store.jobs.Get(id)
	if !ok {
		return jobs.ErrNotFound
	}
	if !j.Can(jobs.Allow) || j.Safety == nil {
		return errors.New("that download isn't blocked")
	}
	if !strings.EqualFold(strings.TrimSpace(confirm), strings.TrimSpace(j.Title)) {
		return errors.New("type the download's name exactly to install it anyway")
	}
	_, err := s.c.store.jobs.Update(id, func(j *jobs.Job) bool {
		j.Safety.Overridden, j.State = true, jobs.Downloaded
		return true
	})
	if err == nil {
		logx.Printf("store: %s installed despite the safety checks, as the person chose", j.Title)
		s.c.emit(EventStoreJobs, s.c.store.jobs.All())
	}
	return err
}

// HasVirusTotalKey reports whether a VirusTotal API key is saved.
func (s *StoreService) HasVirusTotalKey() bool { return platform.LoadSecret(virusTotalSecret) != "" }

// SetVirusTotalKey stores the person's VirusTotal API key encrypted for
// this Windows account ("" removes it). Only file hashes are looked up.
func (s *StoreService) SetVirusTotalKey(key string) error {
	if err := s.on(); err != nil {
		return err
	}
	return platform.SaveSecret(virusTotalSecret, strings.TrimSpace(key))
}

// SandboxAvailable reports whether Windows Sandbox is turned on.
func (s *StoreService) SandboxAvailable() bool { return sandboxExe() != "" }

// OpenInSandbox opens Windows Sandbox with a download on its desktop,
// read-only and without network, to try it there first.
func (s *StoreService) OpenInSandbox(id string) error {
	if err := s.on(); err != nil {
		return err
	}
	j, ok := s.c.store.jobs.Get(id)
	if !ok || j.Name == "" {
		return jobs.ErrNotFound
	}
	if sandboxExe() == "" {
		return errors.New("Windows Sandbox isn't turned on (Windows features, Windows Sandbox)")
	}
	folder := root(j)
	if fi, err := os.Stat(folder); err != nil {
		return err
	} else if !fi.IsDir() {
		folder = filepath.Dir(folder)
	}
	b, err := sandboxConfig(folder)
	if err != nil {
		return err
	}
	p := filepath.Join(platform.CacheDir("store", "sandbox"), j.ID+".wsb")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return err
	}
	return platform.OpenFile(p)
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

// Feeds lists the catalog feeds and how their last fetch went.
func (s *StoreService) Feeds() []FeedInfo { return s.c.catalog.feeds() }

// AddFeed fetches a feed and adds it when Seaglass can read it.
func (s *StoreService) AddFeed(url string) (settings.Settings, error) {
	if err := s.on(); err != nil {
		return s.c.Settings.Get(), err
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, time.Minute)
	defer cancel()
	return s.c.catalog.addFeed(ctx, url)
}

// RemoveFeed removes a feed; its games leave the catalog.
func (s *StoreService) RemoveFeed(url string) (settings.Settings, error) {
	if err := s.on(); err != nil {
		return s.c.Settings.Get(), err
	}
	return s.c.catalog.removeFeed(url)
}

// SetFeedEnabled shows a feed's games in the catalog, or not.
func (s *StoreService) SetFeedEnabled(url string, on bool) (settings.Settings, error) {
	if err := s.on(); err != nil {
		return s.c.Settings.Get(), err
	}
	return s.c.catalog.setFeedEnabled(url, on)
}

// RefreshFeeds fetches every enabled feed now.
func (s *StoreService) RefreshFeeds() ([]FeedInfo, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 2*time.Minute)
	defer cancel()
	s.c.catalog.refresh(ctx, true)
	return s.c.catalog.feeds(), nil
}

// Catalog returns a page of the catalog.
func (s *StoreService) Catalog(q catalog.Query) (catalog.Page, error) {
	if err := s.on(); err != nil {
		return catalog.Page{Entries: []catalog.Entry{}}, err
	}
	return s.c.catalog.search(q), nil
}

// CatalogLanguages lists the catalog's languages, most offered first.
func (s *StoreService) CatalogLanguages() []string { return s.c.catalog.languages() }

// CatalogEntry returns one game in the catalog.
func (s *StoreService) CatalogEntry(key string) (catalog.Entry, error) {
	if err := s.on(); err != nil {
		return catalog.Entry{}, err
	}
	e, ok := s.c.catalog.entry(key)
	if !ok {
		return e, errors.New("that game isn't in the catalog anymore")
	}
	return e, nil
}

// InstallOptions are chosen before a game downloads.
type InstallOptions struct {
	Dir      string `json:"dir"`      // the game's own folder; "" uses one in the games folder
	Language string `json:"language"` // one of the offer's languages; "" for the installer's default
	Install  bool   `json:"install"`  // install once downloaded and checked; false only downloads
	// Update installs over the version the store installed before, in its
	// folder (Dir is ignored).
	Update bool `json:"update"`
}

// Updates lists the games the store installed that have a newer version.
func (s *StoreService) Updates() []catalog.Entry {
	if s.on() != nil {
		return []catalog.Entry{}
	}
	return s.c.catalog.updates()
}

// InstallFolder suggests a folder for a game.
func (s *StoreService) InstallFolder(title string) string {
	return filepath.Join(gamesDir(s.c.Settings.Get().Store), folderName(title))
}

// ChooseInstallFolder asks for a game's folder (it isn't saved).
func (s *StoreService) ChooseInstallFolder(current string) (string, error) {
	d := application.Get().Dialog.OpenFile().
		SetTitle("Choose where the game goes").
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true)
	if dir := filepath.Dir(current); platform.IsDir(dir) {
		d.SetDirectory(dir)
	}
	p, err := d.PromptForSingleSelection()
	if err != nil || p == "" {
		return current, err
	}
	return p, nil
}

// ChooseGamesFolder asks where games are installed.
func (s *StoreService) ChooseGamesFolder() (settings.Settings, error) {
	if err := s.on(); err != nil {
		return s.c.Settings.Get(), err
	}
	p, err := application.Get().Dialog.OpenFile().
		SetTitle("Choose where games are installed").
		CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).
		PromptForSingleSelection()
	if err != nil || p == "" {
		return s.c.Settings.Get(), err
	}
	v := s.c.Settings.Get()
	v.Store.Games = p
	return NewSettingsService(s.c).Save(v)
}

// GamesFolder is where games are installed.
func (s *StoreService) GamesFolder() string { return gamesDir(s.c.Settings.Get().Store) }

// Art returns the art and descriptions known for these catalog games and
// looks up the rest, in this order (EventStoreArt brings them).
func (s *StoreService) Art(keys []string) []StoreArt {
	if s.on() != nil || len(keys) > 500 {
		return []StoreArt{}
	}
	return s.c.art.get(keys)
}

// DownloadOffer queues one of a game's offers (by its place in the
// entry's offers).
func (s *StoreService) DownloadOffer(key string, offer int, opts InstallOptions) (jobs.Job, error) {
	e, err := s.CatalogEntry(key)
	if err != nil {
		return jobs.Job{}, err
	}
	return s.queueOffer(e, offer, opts)
}

// queueOffer checks an offer of an entry (from the catalog, or a prepared
// source release) and queues it: languages, the update guard, the folder
// and free space.
func (s *StoreService) queueOffer(e catalog.Entry, offer int, opts InstallOptions) (jobs.Job, error) {
	if offer < 0 || offer >= len(e.Offers) {
		return jobs.Job{}, errors.New("that version isn't offered anymore")
	}
	o := e.Offers[offer]
	if opts.Language == "" {
		opts.Language = "English"
	}
	if len(o.Languages) > 0 && !slices.ContainsFunc(o.Languages, func(l string) bool { return strings.EqualFold(l, opts.Language) }) {
		return jobs.Job{}, fmt.Errorf("this version doesn't offer %s", opts.Language)
	}
	replaces := ""
	switch {
	case opts.Update:
		if e.Installed == nil || e.Installed.Dir == "" {
			return jobs.Job{}, errors.New("the store didn't install this game, so there's nothing to update")
		}
		if order, comparable := catalog.CompareReleases(o.Version, e.Installed.Version); !comparable || order <= 0 {
			return jobs.Job{}, errors.New("this release is not a confirmed newer game version; choose a separate installation")
		}
		opts.Dir, opts.Install, replaces = e.Installed.Dir, true, e.Installed.Download
	case opts.Dir == "":
		opts.Dir = s.InstallFolder(e.Title)
	}
	if replaces == "" {
		if err := checkInstallDir(opts.Dir); err != nil {
			return jobs.Job{}, err
		}
	}
	dir := downloadsDir(s.c.Settings.Get().Store)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return jobs.Job{}, err
	}
	if err := enoughSpace(dir, o.SizeBytes); err != nil {
		return jobs.Job{}, err
	}
	if opts.Install && o.InstalledSizeBytes > 0 {
		if err := enoughSpace(existingParent(opts.Dir), o.InstalledSizeBytes); err != nil {
			return jobs.Job{}, fmt.Errorf("for the installed game: %w", err)
		}
	}
	title := e.Title
	if o.Version != "" {
		title += " " + o.Version
	}
	j, err := s.c.store.jobs.Add(jobs.Job{Title: title, Source: o.Source(), SavePath: dir, GameKey: e.Key, Version: o.Version, FeedName: o.FeedName,
		InstallDir: filepath.Clean(opts.Dir), Language: opts.Language, Languages: slices.Clone(o.Languages), AutoInstall: opts.Install, SHA256: o.SHA256, Installer: o.InstallerType, Replaces: replaces}, time.Now())
	if err == nil {
		s.c.store.wake()
	}
	return j, err
}

// gamesDir is where games are installed.
func gamesDir(s settings.StoreSettings) string {
	if s.Games != "" {
		return s.Games
	}
	return filepath.Join(platform.Profile, "Games")
}

// folderName makes a title safe as a folder name.
func folderName(title string) string {
	name := strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return -1
		}
		return r
	}, title)
	name = strings.TrimRight(strings.TrimSpace(name), ". ")
	if name == "" {
		name = "Game"
	}
	return name
}

// checkInstallDir accepts a folder a game can be installed into: absolute,
// not a drive or Windows itself, and empty when it exists.
func checkInstallDir(dir string) error {
	dir = filepath.Clean(dir)
	switch {
	case !filepath.IsAbs(dir) || filepath.Dir(dir) == dir:
		return errors.New("choose a folder for the game, not a whole drive")
	case platform.WindowsDir != "" && platform.Within(platform.WindowsDir, dir):
		return errors.New("games can't go into the Windows folder")
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s isn't empty: choose a new folder for the game", dir)
	}
	return nil
}

// existingParent is dir, or its closest parent that exists.
func existingParent(dir string) string {
	for !platform.IsDir(dir) && filepath.Dir(dir) != dir {
		dir = filepath.Dir(dir)
	}
	return dir
}

func enoughSpace(dir string, need int64) error {
	if free, err := platform.FreeSpace(dir); err == nil && need > 0 && free < uint64(need)+256<<20 {
		return fmt.Errorf("not enough free space: %s is needed, %s is free", bytesText(need), bytesText(int64(free)))
	}
	return nil
}
