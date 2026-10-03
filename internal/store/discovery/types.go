// Package discovery keeps a local, persistent index of the releases the
// repack sources publish, groups them into games, and answers the Store's
// browse, search and home queries from it. Indexing reads public release
// metadata only: it never starts torrent peers, downloads a game or runs an
// installer.
//
// The types in this file are the shared contract between the discovery
// backend, the metadata providers, the app services and the interface
// (frontend/src/lib/types.ts mirrors them). Change them only through the
// coordinator of docs/store-discovery-plan.md.
package discovery

import (
	"time"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// Sources discovery can index, in the order the interface lists them:
// the providers sources.Providers declares.
var Sources = sources.DiscoveryIDs()

// Provider and request states the interface shows. A failure never hides
// what is already known: cached results come back with StateStale.
const (
	StateOK          = "ok"          // fresh
	StateLoading     = "loading"     // asked for; arrives through an event
	StateStale       = "stale"       // cached, older than its refresh time or the last refresh failed
	StateUnavailable = "unavailable" // nothing is known and nothing could be fetched
	StateError       = "error"       // the request failed; Error says why
	StateSkipped     = "skipped"     // not asked (source turned off, query too short)
)

// Source crawl states (SourceStatus.State).
const (
	CrawlIdle     = "idle"     // up to date; the next pass waits for its time
	CrawlRecent   = "recent"   // fetching the newest listings
	CrawlBackfill = "backfill" // fetching older listing pages
	CrawlPaused   = "paused"   // a game is running
	CrawlBackoff  = "backoff"  // the source failed or asked to slow down; RetryAt says when
	CrawlDisabled = "disabled" // turned off on this PC
)

// Release availability (Release.Availability).
const (
	AvailInstallable = "installable" // a validated torrent identity exists
	AvailUnresolved  = "unresolved"  // torrent mirrors are listed but not resolved yet
	AvailManual      = "manual"      // only a browser can get the torrent (CAPTCHA, unsupported host)
	AvailUpdateOnly  = "update-only" // a patch, not a standalone install
	AvailPreview     = "preview"     // announced by the source; nothing to download yet
	AvailSummary     = "summary"     // only the listing summary is known; details load on demand
	AvailGone        = "unavailable" // the article disappeared from its source
)

// Release origins: a discovered source article or an offer from a feed the
// person added (the existing catalog).
const (
	OriginSource = "source"
	OriginFeed   = "feed"
)

// SourceFeeds is the source filter value for the games from added feeds
// and reviewed offers.
const SourceFeeds = "feeds"

// ---------------------------------------------------------------------------
// Persisted records (%LOCALAPPDATA%\Seaglass\store\discovery). Disposable:
// deleting them only means indexing again.

// IndexSchema versions the persisted index; a different version is
// re-indexed from scratch rather than migrated.
const IndexSchema = 1

// Record is one source article as discovery knows it. Keyed by
// sources.Entry.ID, which is derived from the source and the canonical
// article URL.
type Record struct {
	Entry sources.Entry `json:"entry"` // the latest parsed claims
	// FirstSeen is when the article was first indexed. Backfill marks
	// articles found on older listing pages so they never count as new
	// wishlist activity.
	FirstSeen time.Time `json:"firstSeen"`
	Backfill  bool      `json:"backfill,omitempty"`
	LastSeen  time.Time `json:"lastSeen"` // last listing, search or detail fetch that returned it
	// Changed is when the parsed claims (title, version, size, transports)
	// last changed after the first fetch; zero when they never did.
	Changed  time.Time `json:"changed,omitzero"`
	Detailed time.Time `json:"detailed,omitzero"` // the full article was parsed (not a summary)
	Origin   string    `json:"origin"`            // rss, listing, search, detail
	// Gone: the article returned 404/410 or vanished from its listing.
	// The record stays so wishlists and installed games keep their history.
	Gone       bool   `json:"gone,omitempty"`
	FetchError string `json:"fetchError,omitempty"` // the last detail fetch's failure
}

// CrawlState is one source's indexing progress, persisted between starts.
type CrawlState struct {
	Source       string    `json:"source"`
	RecentAt     time.Time `json:"recentAt,omitzero"` // last successful recent refresh
	NextPage     int       `json:"nextPage"`          // next older listing page to fetch (2 after the first)
	BackfillDone bool      `json:"backfillDone"`      // a confirmed end was reached
	PassAt       time.Time `json:"passAt,omitzero"`   // last background pass
	Failures     int       `json:"failures"`          // consecutive failures, for backoff
	RetryAt      time.Time `json:"retryAt,omitzero"`  // no request before this (Retry-After, backoff)
	Error        string    `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Roaming user data (%APPDATA%\Seaglass\store-identity.json).

// IdentityCorrection is the person's choice of Steam game (or "not on
// Steam") for a group of source releases, kept across refreshes. It is
// keyed by the group's title key so it applies to releases published later.
type IdentityCorrection struct {
	TitleKey   string    `json:"titleKey"`   // scan.Normalize of the release title
	SteamAppID int       `json:"steamAppId"` // 0: not on Steam, keep it apart
	Name       string    `json:"name"`       // Steam's name for it
	At         time.Time `json:"at"`
}

// ---------------------------------------------------------------------------
// Interface models. Times are unix seconds; 0 means unknown.

// Status is what discovery is doing, for the Store and Settings.
type Status struct {
	// Enabled: the Store and source browsing are on and at least one
	// source is chosen.
	Enabled bool `json:"enabled"`
	// SetupNeeded: an existing Store user must make the one-time source
	// choice before anything is indexed.
	SetupNeeded bool           `json:"setupNeeded"`
	Sources     []SourceStatus `json:"sources"`
	Games       int            `json:"games"`    // source-backed games in the index
	Releases    int            `json:"releases"` // source articles in the index
	Refreshing  bool           `json:"refreshing"`
	// Paused: the person paused indexing on this PC
	// (StoreSettings.IndexingPaused); nothing is requested until resumed.
	Paused bool `json:"paused"`
	// Playing: a game is running, so indexing waits for it to end.
	Playing bool `json:"playing"`
	// Stale: the newest listings are older than six hours or their last
	// refresh failed; what is shown is the cached index.
	Stale bool `json:"stale"`
}

// SourceStatus is one source's indexing state.
type SourceStatus struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	State        string `json:"state"` // Crawl* constants
	Releases     int    `json:"releases"`
	RecentAt     int64  `json:"recentAt"`
	BackfillPage int    `json:"backfillPage"` // next older page; 0 before the first pass
	BackfillDone bool   `json:"backfillDone"`
	RetryAt      int64  `json:"retryAt"`
	Error        string `json:"error,omitempty"`
	// What the provider supports (sources.Provider), so setup, filters and
	// status read it instead of naming sources.
	Host      string   `json:"host"`
	Search    bool     `json:"search"`    // its own site search fills search results
	Paged     bool     `json:"paged"`     // older listing pages exist; false: one finite catalog page
	Torrents  bool     `json:"torrents"`  // releases may be installable; false: they open in the browser
	DefaultOn bool     `json:"defaultOn"` // chosen for a new Store user
	Notes     []string `json:"notes"`     // verified limitations, plain sentences
}

// GameSummary is one game on a shelf, in Browse or in search results.
type GameSummary struct {
	Key        string `json:"key"` // "steam:<appid>" or "title:<normalized title>", as catalog.Entry.Key
	Title      string `json:"title"`
	SteamAppID int    `json:"steamAppId,omitempty"`
	// SourceBacked: at least one source release or feed offer exists.
	// Steam-only search results have none and cannot be installed.
	SourceBacked bool     `json:"sourceBacked"`
	Sources      []string `json:"sources"` // fitgirl, dodi, feeds
	Releases     int      `json:"releases"`
	Version      string   `json:"version,omitempty"` // the newest release's claim, as published
	// PublishedAt is the newest source publication; UpdatedAt the newest
	// change to a source release. Neither is a game release or build date.
	PublishedAt int64 `json:"publishedAt"`
	UpdatedAt   int64 `json:"updatedAt"`
	// ReleaseDate is the game's own release date as Steam's metadata
	// gives it ("12 Mar, 2024"); "" unknown. Never a source date.
	ReleaseDate string   `json:"releaseDate,omitempty"`
	SizeBytes   int64    `json:"sizeBytes"` // the newest release's download claim; 0 unknown
	Languages   []string `json:"languages"` // claimed by any release; empty: unknown
	Genres      []string `json:"genres"`    // from Steam once its metadata is known; empty: unknown
	Installable bool     `json:"installable"`
	// BrowserOnly: every source release opens in a browser (the source
	// offers no torrents). Announced: the sources only announce the game.
	BrowserOnly bool `json:"browserOnly"`
	Announced   bool `json:"announced"`
	// PopularRank is the place on Steam's most-played chart (1 = first); 0
	// when it isn't on the chart or the chart is unavailable. Never derived
	// from anything else.
	PopularRank int `json:"popularRank"`
	// Steam's overall review summary, once fetched for this game.
	ReviewPercent int    `json:"reviewPercent"` // positive share 0..100
	ReviewTotal   int    `json:"reviewTotal"`   // 0: unknown or no reviews
	ReviewLabel   string `json:"reviewLabel,omitempty"`
	// CompletionMain is HowLongToBeat's Main Story time in minutes from
	// the cache; 0 unknown. Cards never fetch it.
	CompletionMain int        `json:"completionMain"`
	Installed      *Installed `json:"installed,omitempty"`
	Wishlisted     bool       `json:"wishlisted"`
	// Activity: unread wishlist activity (a first release or a confirmed
	// newer version).
	Activity bool `json:"activity"`
}

// Installed is a game the Store installed on this PC.
type Installed struct {
	Download string `json:"download"`
	Version  string `json:"version"`
	Update   bool   `json:"update"` // a release is a confirmed newer version
}

// BrowseQuery picks and orders games.
type BrowseQuery struct {
	Text     string   `json:"text"`
	Sources  []string `json:"sources"`  // fitgirl, dodi, feeds; empty: all
	Language string   `json:"language"` // "" any
	Genre    string   `json:"genre"`    // "" any
	// Availability: "" any, "installable", "unresolved" (no validated
	// transport yet).
	Availability string `json:"availability"`
	Installed    string `json:"installed"` // "" any, "installed", "not-installed"
	// Sort: title, published (newest first), popular (chart order, unranked
	// last), reviews (best first, unknown last).
	Sort   string `json:"sort"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"` // 60 by default, at most 200
}

// Browse sorts.
const (
	SortTitle     = "title"
	SortPublished = "published"
	SortPopular   = "popular"
	SortReviews   = "reviews"
)

// BrowsePage is one page of source-backed games.
type BrowsePage struct {
	Games []GameSummary `json:"games"`
	Total int           `json:"total"` // matching games on every page together
	// Unknown counts games left out only because the filtered field
	// (language or genre) is unknown for them, so the interface can say so
	// instead of inventing a value.
	Unknown   int      `json:"unknown"`
	Languages []string `json:"languages"` // filter choices, most common first
	Genres    []string `json:"genres"`
}

// ProviderProgress is one remote search: a source site or Steam.
type ProviderProgress struct {
	ID    string `json:"id"` // fitgirl, dodi, steam
	Name  string `json:"name"`
	State string `json:"state"` // State* constants
	Found int    `json:"found"` // results it returned
	Error string `json:"error,omitempty"`
	// Cached: answered from the one-hour search cache.
	Cached bool `json:"cached"`
}

// SearchResult is the Store's unified search: local matches at once, then
// the source sites and Steam to fill gaps.
type SearchResult struct {
	Query BrowseQuery `json:"query"`
	Page  BrowsePage  `json:"page"`
	// Other are Steam games with no known source release ("No known
	// source release"); they can be wishlisted but not installed.
	Other    []GameSummary      `json:"other"`
	Remote   []ProviderProgress `json:"remote"`
	Complete bool               `json:"complete"` // every remote search finished or was skipped
}

// Home is the Store's front page.
type Home struct {
	New     []GameSummary `json:"new"`     // newest source publication first
	Popular []GameSummary `json:"popular"` // Steam chart order among source-backed games
	// PopularState: StateOK, StateStale (cached chart) or StateUnavailable
	// (no chart: the shelf shows its empty state, never another ranking).
	PopularState string        `json:"popularState"`
	Updated      []GameSummary `json:"updated"`  // source releases that changed, newest change first
	Wishlist     []GameSummary `json:"wishlist"` // wishlisted games with unread activity
	// Featured are a few source-backed games for the top of the page:
	// new and popular first, never installed ones.
	Featured []GameSummary `json:"featured"`
	// Recommended are source-backed games that share genres with games
	// played recently or wishlisted, never installed ones; with no useful
	// history they are popular games. RecommendedBasis says which.
	Recommended      []Recommendation `json:"recommended"`
	RecommendedBasis string           `json:"recommendedBasis"` // Basis* constants
	Status           Status           `json:"status"`
}

// Recommendation bases (Home.RecommendedBasis).
const (
	BasisNone     = ""         // nothing to recommend
	BasisPlayed   = "played"   // genres of recently played library games
	BasisWishlist = "wishlist" // genres of wishlisted games
	BasisBoth     = "played+wishlist"
	BasisPopular  = "popular" // no useful history: Steam's chart among source-backed games
)

// Recommendation is one recommended game and why.
type Recommendation struct {
	Game GameSummary `json:"game"`
	// Because names the played or wishlisted games it shares genres with,
	// most relevant first, at most three; empty for BasisPopular.
	Because []string `json:"because"`
	Genres  []string `json:"genres"` // the shared genres
}

// Signal is a game whose genres steer recommendations.
type Signal struct {
	Title  string
	Genres []string
	Kind   string // BasisPlayed or BasisWishlist
	// Weight: more recent play or a newer wishlist entry weighs more.
	Weight float64
}

// Release is one release choice on a game's page.
type Release struct {
	ID         string `json:"id"`     // sources.Entry.ID, or "feed:<offer index>"
	Origin     string `json:"origin"` // OriginSource or OriginFeed
	Source     string `json:"source"` // fitgirl, dodi, or the feed's URL
	SourceName string `json:"sourceName"`
	Title      string `json:"title"`
	RawTitle   string `json:"rawTitle"`
	Version    string `json:"version,omitempty"`
	PageURL    string `json:"pageUrl,omitempty"`
	// PublishedAt / UpdatedAt are the source's publication and change
	// times, not the game's release or build date.
	PublishedAt        int64    `json:"publishedAt"`
	UpdatedAt          int64    `json:"updatedAt"`
	SizeBytes          int64    `json:"sizeBytes"`
	SizeClaim          string   `json:"sizeClaim,omitempty"`
	SizeIsMinimum      bool     `json:"sizeIsMinimum,omitempty"`
	InstalledSizeBytes int64    `json:"installedSizeBytes,omitempty"`
	Languages          []string `json:"languages"` // recognized names; empty: unknown
	LanguageClaim      string   `json:"languageClaim,omitempty"`
	Kind               string   `json:"kind"`         // release, update or preview
	Availability       string   `json:"availability"` // Avail* constants
	// Transports is the number of validated torrent identities.
	Transports int `json:"transports"`
	// BrowserOnly: the source offers no torrents; its files open in a
	// browser and can't be downloaded or installed by Seaglass.
	BrowserOnly bool `json:"browserOnly"`
	// Unresolved explains why mirrors are not usable yet (captcha-required,
	// rate-limited, unsupported host …), one line each.
	Unresolved []string `json:"unresolved"`
	Warnings   []string `json:"warnings"`
	// Newer: a confirmed newer game version than the installed one.
	Newer bool `json:"newer"`
	// FeedKey and FeedOffer locate a feed offer for DownloadOffer.
	FeedKey   string `json:"feedKey,omitempty"`
	FeedOffer int    `json:"feedOffer"`
}

// Identity says how a game's Steam match was made.
type Identity struct {
	SteamAppID int    `json:"steamAppId"`
	Name       string `json:"name,omitempty"`
	// How: "game-database" (Seaglass's identification), "correction" (the
	// person chose it), "feed" (the feed gave it) or "" (no Steam match).
	How       string `json:"how"`
	Corrected bool   `json:"corrected"`
}

// GameDetails is a game's page.
type GameDetails struct {
	Summary  GameSummary `json:"summary"`
	Releases []Release   `json:"releases"` // newest publication first
	// Recommended is the index in Releases to offer first; -1 none.
	Recommended int      `json:"recommended"`
	Why         []string `json:"why"`
	Identity    Identity `json:"identity"`
	// Loading: release details are being fetched; EventStoreGames brings
	// the update.
	Loading bool `json:"loading"`
}

// PreparedRelease is a release made ready for the install confirmation.
type PreparedRelease struct {
	GameKey string  `json:"gameKey"`
	Release Release `json:"release"`
	// Ready: a validated transport exists; Offers is filled and
	// DownloadRelease can queue one of them.
	Ready bool `json:"ready"`
	// Offers are the release as the install confirmation shows it, one per
	// validated transport.
	Offers []PreparedOffer `json:"offers"`
	// State: "ready", "unresolved" (resolution failed or needs a browser),
	// "update-only", "unavailable", "preview" (announced, nothing to
	// download yet), "browser" (a source without torrents).
	State    string   `json:"state"`
	Reason   string   `json:"reason,omitempty"`
	Warnings []string `json:"warnings"`
	// Installed is the copy the Store installed, which an update replaces
	// in its folder; nil when there is none.
	Installed *InstalledCopy `json:"installed,omitempty"`
}

// InstalledCopy is where the Store installed a game, and which version.
type InstalledCopy struct {
	Version string `json:"version"`
	Dir     string `json:"dir"`
}

// PreparedOffer is one validated way to download a prepared release.
type PreparedOffer struct {
	Transport          int      `json:"transport"` // index for DownloadRelease
	Title              string   `json:"title"`
	Version            string   `json:"version,omitempty"`
	SizeBytes          int64    `json:"sizeBytes"`
	InstalledSizeBytes int64    `json:"installedSizeBytes,omitempty"`
	Languages          []string `json:"languages"`
	TorrentName        string   `json:"torrentName,omitempty"`
	InfoHash           string   `json:"infoHash"`
	SourceName         string   `json:"sourceName"`
}

// Change says which games changed, so open pages reload.
type Change struct {
	Keys []string `json:"keys"` // these games changed
	All  bool     `json:"all"`  // the whole index changed (refresh, setting)
}

// SearchProgress reports remote searches while SearchGames runs.
type SearchProgress struct {
	Text   string             `json:"text"`
	Remote []ProviderProgress `json:"remote"`
}
