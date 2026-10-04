// Package settings stores Seaglass's preferences in %APPDATA%\Seaglass\settings.json.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	"github.com/ApolloF/Seaglass/internal/store/sources"
	"github.com/ApolloF/Seaglass/internal/torrent"
)

// Settings are the user's preferences. New fields get their default when
// an older settings file doesn't have them.
type Settings struct {
	// Library
	Folders          []string `json:"folders"`          // extra folders whose subfolders are games
	AutoFolders      bool     `json:"autoFolders"`      // also look in common game folders on every drive
	DetectExternal   bool     `json:"detectExternal"`   // recognise games on Steam API emulators and from repacks
	ReviewUncertain  bool     `json:"reviewUncertain"`  // keep low-confidence matches in New on this PC for a check
	ShowNotInstalled bool     `json:"showNotInstalled"` // list games that aren't installed (anymore)
	ShowOwned        bool     `json:"showOwned"`        // list games connected store accounts own but aren't installed
	OwnedGOG         bool     `json:"ownedGOG"`         // read GOG Galaxy's library for owned games
	HiddenSources    []string `json:"hiddenSources"`    // libraries whose games aren't shown (steam, epic, …, external, folder)

	// Appearance
	Theme            string `json:"theme"`            // system, dark, light (desktop mode)
	BigPictureLayout string `json:"bigPictureLayout"` // deck, console, orbit

	// Big picture and controller
	OpenBigPictureOnController bool   `json:"openBigPictureOnController"`
	StartInBigPicture          bool   `json:"startInBigPicture"`
	Sounds                     bool   `json:"sounds"`
	Haptics                    bool   `json:"haptics"`
	Lightbar                   bool   `json:"lightbar"`
	PSButton                   bool   `json:"psButton"`
	Glyphs                     string `json:"glyphs"` // auto, playstation, xbox

	// While playing
	CloseWhilePlaying bool   `json:"closeWhilePlaying"` // close the interface while a game runs (frees its memory)
	PadWhilePlaying   string `json:"padWhilePlaying"`   // listen (PS button opens the overlay), off (release the controller)
	NoticeExternal    bool   `json:"noticeExternal"`    // follow games started outside Seaglass (playtime, controller)

	// Saves, through Syncer
	SyncSavesBefore  bool `json:"syncSavesBefore"`  // sync a game's saves before it starts
	BackupSavesAfter bool `json:"backupSavesAfter"` // back its saves up after it exits
	SyncWait         int  `json:"syncWait"`         // seconds to wait for a sync before playing
	StartSyncer      bool `json:"startSyncer"`      // start Syncer (without its window) when it isn't running
	SyncProfile      bool `json:"syncProfile"`      // Syncer syncs playtime, achievements and settings between PCs
	SameSettings     bool `json:"sameSettings"`     // take the settings saved last on another PC (see Portable)
	AskWhoPlays      bool `json:"askWhoPlays"`      // with several Syncer accounts, ask who's playing before a game starts

	// Achievements
	Achievements           bool `json:"achievements"`           // show achievements (read from stores and emulator files)
	ShowHiddenAchievements bool `json:"showHiddenAchievements"` // show hidden achievements before they're unlocked (spoilers)

	// Updates
	AutoUpdate bool `json:"autoUpdate"` // check GitHub for new versions and install them on the next start or while idle in the tray

	// Experimental. Not portable: what this PC downloads, and how, is its own.
	ExperimentalStore bool          `json:"experimentalStore"` // the store: catalogs from feeds the user adds, downloads and installs
	Store             StoreSettings `json:"store"`

	// Welcomed: the first-start welcome was seen (or skipped).
	Welcomed bool `json:"welcomed"`
}

// StoreSettings are the experimental store's.
type StoreSettings struct {
	QBittorrent            string          `json:"qbittorrent"`            // qbittorrent.exe; "" uses the installed one
	Downloads              string          `json:"downloads"`              // where downloads go; "" is Downloads\Seaglass in the user's folder
	Games                  string          `json:"games"`                  // where games are installed; "" is Games in the user's folder
	KeepDownloads          bool            `json:"keepDownloads"`          // keep a download (and seed it) after its game is installed
	PauseWhilePlaying      bool            `json:"pauseWhilePlaying"`      // downloads wait while a game runs
	DisablePayloadScanning bool            `json:"disablePayloadScanning"` // per PC; integrity checks still run
	PrivateSources         bool            `json:"privateSources"`         // browse repack sources (discovery runs only with this on)
	Sources                []string        `json:"sources"`                // the sources discovery indexes on this PC (DiscoverySources)
	SourceSetup            string          `json:"sourceSetup"`            // SetupPending, SetupAsk or SetupDone
	IndexingPaused         bool            `json:"indexingPaused"`         // the person paused discovery's background indexing on this PC
	BlockDetections        bool            `json:"blockDetections"`        // a download Defender or VirusTotal flags isn't installed unless the person insists
	Language               string          `json:"language"`               // the language games are recommended in; "" for any
	Network                torrent.Network `json:"network"`
	Feeds                  []FeedSource    `json:"feeds"` // catalogs, in the order they were added
}

// The repack sources Store discovery can index, in display order: the
// providers the source registry declares.
var DiscoverySources = sources.DiscoveryIDs()

// Source setup states (StoreSettings.SourceSetup).
const (
	// SetupPending: nobody chose yet. Turning the Store on chooses every
	// source (a new Store user).
	SetupPending = ""
	// SetupAsk: the Store was on before discovery existed; the person
	// chooses the sources once, and nothing is indexed until they do.
	SetupAsk = "ask"
	// SetupDone: the sources were chosen.
	SetupDone = "done"
)

// FeedSource is a catalog feed the person added.
type FeedSource struct {
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	Trust   int    `json:"trust"` // -2 (less) … 2 (more): weighs in when versions are recommended
}

// Sources are the libraries that can be hidden, as the interface groups
// games: by store, then external copies, standalone installs and folders.
var Sources = map[string]bool{
	"steam": true, "epic": true, "gog": true, "ea": true, "ubisoft": true, "battlenet": true, "xbox": true,
	"external": true, "standalone": true, "folder": true,
}

// Defaults are the settings on first start.
func Defaults() Settings {
	return Settings{
		Folders: []string{}, HiddenSources: []string{}, AutoFolders: true, DetectExternal: true, ReviewUncertain: true,
		Theme: "system", BigPictureLayout: "deck",
		OpenBigPictureOnController: true, Haptics: true, Lightbar: true, PSButton: true, Glyphs: "auto",
		CloseWhilePlaying: true, PadWhilePlaying: "listen", NoticeExternal: true,
		SyncSavesBefore: true, BackupSavesAfter: true, SyncWait: 60, StartSyncer: true,
		SyncProfile: true, SameSettings: true,
		AutoUpdate: true, Achievements: true,
		Store: StoreSettings{PauseWhilePlaying: true, BlockDetections: true, Language: "English", Network: torrent.DefaultNetwork(), Feeds: []FeedSource{}, Sources: []string{}},
	}
}

// Portable are the settings that go with a person to their other PCs.
// What depends on the PC stays: its game folders and stores, whether it
// starts in big picture (a TV, not a desk), whether it asks who's playing
// (a PC several people share), and the welcome.
type Portable struct {
	DetectExternal         bool     `json:"detectExternal"`
	ReviewUncertain        bool     `json:"reviewUncertain"`
	ShowNotInstalled       bool     `json:"showNotInstalled"`
	ShowOwned              bool     `json:"showOwned"`
	HiddenSources          []string `json:"hiddenSources"`
	Theme                  string   `json:"theme"`
	BigPictureLayout       string   `json:"bigPictureLayout"`
	Sounds                 bool     `json:"sounds"`
	Haptics                bool     `json:"haptics"`
	Lightbar               bool     `json:"lightbar"`
	PSButton               bool     `json:"psButton"`
	Glyphs                 string   `json:"glyphs"`
	CloseWhilePlaying      bool     `json:"closeWhilePlaying"`
	PadWhilePlaying        string   `json:"padWhilePlaying"`
	NoticeExternal         bool     `json:"noticeExternal"`
	SyncSavesBefore        bool     `json:"syncSavesBefore"`
	BackupSavesAfter       bool     `json:"backupSavesAfter"`
	SyncWait               int      `json:"syncWait"`
	Achievements           bool     `json:"achievements"`
	ShowHiddenAchievements bool     `json:"showHiddenAchievements"`
	AutoUpdate             bool     `json:"autoUpdate"`
}

// Portable returns the settings that go with a person.
func (v Settings) Portable() Portable {
	return Portable{
		DetectExternal: v.DetectExternal, ReviewUncertain: v.ReviewUncertain, ShowNotInstalled: v.ShowNotInstalled,
		ShowOwned: v.ShowOwned, HiddenSources: append([]string{}, v.HiddenSources...), Theme: v.Theme,
		BigPictureLayout: v.BigPictureLayout, Sounds: v.Sounds, Haptics: v.Haptics, Lightbar: v.Lightbar,
		PSButton: v.PSButton, Glyphs: v.Glyphs, CloseWhilePlaying: v.CloseWhilePlaying, PadWhilePlaying: v.PadWhilePlaying,
		NoticeExternal: v.NoticeExternal, SyncSavesBefore: v.SyncSavesBefore, BackupSavesAfter: v.BackupSavesAfter,
		SyncWait: v.SyncWait, Achievements: v.Achievements,
		ShowHiddenAchievements: v.ShowHiddenAchievements, AutoUpdate: v.AutoUpdate,
	}
}

// WithPortable returns v with another PC's portable settings, given as
// JSON. Settings the JSON doesn't have (an older Seaglass saved it) stay
// as they are.
func (v Settings) WithPortable(b []byte) (Settings, error) {
	p := v.Portable()
	if err := json.Unmarshal(b, &p); err != nil {
		return v, err
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(b, &keys) == nil {
		legacyExternal(&p.DetectExternal, keys)
	}
	v.DetectExternal, v.ReviewUncertain, v.ShowNotInstalled = p.DetectExternal, p.ReviewUncertain, p.ShowNotInstalled
	v.ShowOwned, v.HiddenSources, v.Theme = p.ShowOwned, p.HiddenSources, p.Theme
	v.BigPictureLayout, v.Sounds, v.Haptics, v.Lightbar = p.BigPictureLayout, p.Sounds, p.Haptics, p.Lightbar
	v.PSButton, v.Glyphs, v.CloseWhilePlaying, v.PadWhilePlaying = p.PSButton, p.Glyphs, p.CloseWhilePlaying, p.PadWhilePlaying
	v.NoticeExternal, v.SyncSavesBefore, v.BackupSavesAfter = p.NoticeExternal, p.SyncSavesBefore, p.BackupSavesAfter
	v.SyncWait, v.Achievements = p.SyncWait, p.Achievements
	v.ShowHiddenAchievements, v.AutoUpdate = p.ShowHiddenAchievements, p.AutoUpdate
	return normalize(v), nil
}

// Store loads and saves settings. Safe for concurrent use.
type Store struct {
	path string
	mu   sync.Mutex
	cur  Settings
}

// Open reads the settings file (defaults when missing). A damaged file
// (cut short by a power cut) is set aside and the copy from the last
// good start is used, so the folders and choices aren't lost to defaults.
func Open(path string) *Store {
	s := &Store{path: path, cur: Defaults()}
	for _, p := range []string{path, path + ".bak"} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v := Defaults()
		if json.Unmarshal(b, &v) != nil {
			if p == path {
				_ = os.Rename(path, path+".broken-"+time.Now().Format("20060102-150405"))
			}
			continue
		}
		if p == path {
			_ = writeAtomic(path+".bak", b)
		}
		// Settings saved before the welcome existed belong to someone
		// who has used Seaglass already.
		var keys map[string]json.RawMessage
		if json.Unmarshal(b, &keys) == nil {
			if _, ok := keys["welcomed"]; !ok {
				v.Welcomed = true
			}
			legacyExternal(&v.DetectExternal, keys)
			legacySourceSetup(&v, keys)
		}
		s.cur = normalize(v)
		break
	}
	return s
}

// DefaultPath is %APPDATA%\Seaglass\settings.json.
func DefaultPath() string { return filepath.Join(platform.AppDir(), "settings.json") }

// Get returns the current settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cur
	c.Folders = append([]string{}, s.cur.Folders...)
	c.HiddenSources = append([]string{}, s.cur.HiddenSources...)
	c.Store.Feeds = append([]FeedSource{}, s.cur.Store.Feeds...)
	c.Store.Sources = append([]string{}, s.cur.Store.Sources...)
	return c
}

// Set validates, stores and saves new settings, returning what was saved.
func (s *Store) Set(v Settings) (Settings, error) {
	s.mu.Lock()
	old := s.cur
	s.mu.Unlock()
	v = normalize(StoreTurnedOn(old, v))
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return s.Get(), err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeAtomic(s.path, b); err != nil {
		return s.cur, err
	}
	s.cur = v
	return v, nil
}

// legacyExternal reads detectUnofficial, the name before v1.9, when the
// JSON (an older settings file, or another PC's older Seaglass) has no
// detectExternal.
func legacyExternal(detect *bool, keys map[string]json.RawMessage) {
	if _, ok := keys["detectExternal"]; ok {
		return
	}
	if raw, ok := keys["detectUnofficial"]; ok {
		_ = json.Unmarshal(raw, detect)
	}
}

// legacySourceSetup asks a person who used the Store before discovery
// existed to choose its sources once, instead of turning them on for them.
func legacySourceSetup(v *Settings, keys map[string]json.RawMessage) {
	var store map[string]json.RawMessage
	if json.Unmarshal(keys["store"], &store) != nil {
		store = nil
	}
	if _, ok := store["sourceSetup"]; !ok && v.ExperimentalStore {
		v.Store.SourceSetup = SetupAsk
		v.Store.Sources = []string{}
	}
}

// StoreTurnedOn settles source setup when a new Store user turns the Store
// on: they start with the default discovery sources, which is none of the
// built-in ones, and turn sources on themselves in Store settings. Source
// browsing is on only when something was chosen.
func StoreTurnedOn(old, v Settings) Settings {
	if !old.ExperimentalStore && v.ExperimentalStore && v.Store.SourceSetup == SetupPending {
		v.Store.Sources = sources.DefaultIDs()
		v.Store.PrivateSources = len(v.Store.Sources) > 0
		v.Store.SourceSetup = SetupDone
	}
	return v
}

// DiscoveryOn reports whether discovery may index source on this PC.
func (s StoreSettings) DiscoveryOn(source string) bool {
	return s.PrivateSources && s.SourceSetup == SetupDone && slices.Contains(s.Sources, source)
}

func normalize(v Settings) Settings {
	var folders []string
	seen := map[string]bool{}
	for _, f := range v.Folders {
		f = filepath.Clean(strings.TrimSpace(f))
		if !filepath.IsAbs(f) || filepath.Dir(f) == f || seen[platform.Key(f)] {
			continue // drive roots would make every folder a "game"
		}
		seen[platform.Key(f)] = true
		folders = append(folders, f)
	}
	if folders == nil {
		folders = []string{}
	}
	v.Folders = folders
	hidden := []string{}
	for _, id := range v.HiddenSources {
		if id == "unofficial" { // its name before v1.9
			id = "external"
		}
		if Sources[id] && !slices.Contains(hidden, id) {
			hidden = append(hidden, id)
		}
	}
	v.HiddenSources = hidden
	switch v.Theme {
	case "system", "dark", "light":
	default:
		v.Theme = "system"
	}
	switch v.SyncWait {
	case 30, 60, 150, 300:
	default:
		v.SyncWait = 60
	}
	switch v.BigPictureLayout {
	case "deck", "console", "orbit":
	default:
		v.BigPictureLayout = "deck"
	}
	switch v.PadWhilePlaying {
	case "listen", "off":
	default:
		v.PadWhilePlaying = "listen"
	}
	v.Store = normalizeStore(v.Store)
	switch v.Glyphs {
	case "auto", "playstation", "xbox":
	default:
		v.Glyphs = "auto"
	}
	return v
}

func normalizeStore(s StoreSettings) StoreSettings {
	if s.QBittorrent = filepath.Clean(strings.TrimSpace(s.QBittorrent)); !filepath.IsAbs(s.QBittorrent) || !strings.EqualFold(filepath.Ext(s.QBittorrent), ".exe") {
		s.QBittorrent = ""
	}
	if s.Downloads = filepath.Clean(strings.TrimSpace(s.Downloads)); !filepath.IsAbs(s.Downloads) || filepath.Dir(s.Downloads) == s.Downloads {
		s.Downloads = ""
	}
	if s.Games = filepath.Clean(strings.TrimSpace(s.Games)); !filepath.IsAbs(s.Games) || filepath.Dir(s.Games) == s.Games {
		s.Games = ""
	}
	s.Network = s.Network.Normalize()
	if r := []rune(strings.TrimSpace(s.Language)); len(r) > 40 {
		s.Language = string(r[:40])
	} else {
		s.Language = string(r)
	}
	feeds := []FeedSource{}
	seen := map[string]bool{}
	for _, f := range s.Feeds {
		f.URL = strings.TrimSpace(f.URL)
		if feed.CheckURL(f.URL) != nil || seen[f.URL] {
			continue
		}
		seen[f.URL] = true
		f.Trust = min(max(f.Trust, -2), 2)
		feeds = append(feeds, f)
	}
	s.Feeds = feeds
	chosen := []string{}
	for _, id := range DiscoverySources {
		if slices.Contains(s.Sources, id) {
			chosen = append(chosen, id)
		}
	}
	s.Sources = chosen
	switch s.SourceSetup {
	case SetupPending, SetupAsk, SetupDone:
	default:
		s.SourceSetup = SetupPending
	}
	return s
}

// writeAtomic writes through a flushed temporary file, so a power cut
// leaves the old file or the new one, never half of one.
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
