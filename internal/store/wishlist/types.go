// Package wishlist keeps the games the person saved in the Store and what
// happened to them since: a first source release, or a confirmed newer
// version. It follows games, not individual releases. Saved in
// %APPDATA%\Seaglass\wishlist.json; it stays on this PC.
//
// The types in this file are the shared contract with the app services and
// the interface (frontend/src/lib/types.ts mirrors them). Change them only
// through the coordinator of docs/store-discovery-plan.md.
package wishlist

import "time"

// Schema versions wishlist.json.
const Schema = 1

// Activity kinds.
const (
	// KindAvailable: a source release appeared for a game that had none
	// when it was saved.
	KindAvailable = "available"
	// KindNewer: a release with a confirmed newer game version than any
	// known when it was saved (catalog.CompareReleases). A changed article
	// or a newer publication date alone never counts.
	KindNewer = "newer"
)

// Entry is one saved game, as stored.
type Entry struct {
	Key        string    `json:"key"` // the discovery game key
	Title      string    `json:"title"`
	SteamAppID int       `json:"steamAppId,omitempty"`
	AddedAt    time.Time `json:"addedAt"`
	// Baseline are the release IDs known when the game was saved, or
	// processed since; only releases outside it can become activity.
	Baseline []string `json:"baseline"`
	// Version is the newest comparable version seen so far ("" none).
	Version  string     `json:"version,omitempty"`
	Activity []Activity `json:"activity"`
	// Origin: OriginManual (saved in Seaglass) or OriginSteam (imported
	// from the person's Steam wishlist). Importing again never removes an
	// entry, whatever its origin.
	Origin string `json:"origin,omitempty"`
}

// Entry origins.
const (
	OriginManual = ""
	OriginSteam  = "steam"
)

// Activity is something that happened to a saved game.
type Activity struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	ReleaseID  string    `json:"releaseId"`
	Source     string    `json:"source"`
	SourceName string    `json:"sourceName"`
	Version    string    `json:"version,omitempty"`
	At         time.Time `json:"at"`
	Read       bool      `json:"read"`
}

// File is wishlist.json.
type File struct {
	Schema  int     `json:"schema"`
	Entries []Entry `json:"entries"`
}

// Observation is one release of a saved game, as discovery sees it.
type Observation struct {
	ReleaseID  string
	Source     string
	SourceName string
	Version    string
	// Backfill: found on an older listing page. Backfilled releases join
	// the baseline silently: indexing history must not look like news.
	Backfill    bool
	PublishedAt time.Time
}

// ActivityView is an Activity for the interface (times in unix seconds).
type ActivityView struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	ReleaseID  string `json:"releaseId"`
	Source     string `json:"source"`
	SourceName string `json:"sourceName"`
	Version    string `json:"version,omitempty"`
	At         int64  `json:"at"`
	Read       bool   `json:"read"`
}
