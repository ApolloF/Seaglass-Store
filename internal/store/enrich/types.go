// Package enrich fetches what the Store shows about a game besides its
// releases: Steam's most-played chart, Steam review summaries and review
// text, the Metacritic score and link Steam gives, and HowLongToBeat
// completion times. Every answer is cached on disk; when a provider fails
// or changes its format, the cached answer comes back marked stale, and
// with nothing cached the answer says it is unavailable. Nothing is ever
// estimated or invented.
//
// The types in this file are the shared contract with the app services and
// the interface (frontend/src/lib/types.ts mirrors them). Change them only
// through the coordinator of docs/store-discovery-plan.md.
package enrich

import "time"

// States, as in the discovery package.
const (
	StateOK          = "ok"
	StateLoading     = "loading"
	StateStale       = "stale"
	StateUnavailable = "unavailable"
	StateError       = "error"
)

// Cache lifetimes agreed in the plan.
const (
	PopularityTTL    = time.Hour
	ReviewSummaryTTL = 6 * time.Hour
	ReviewPageTTL    = time.Hour
	CriticTTL        = 7 * 24 * time.Hour
	CompletionTTL    = 7 * 24 * time.Hour
)

// Popularity is Steam's most-played chart. Only games on it have a rank.
type Popularity struct {
	Ranks     map[int]int `json:"ranks"`     // Steam AppID → rank, 1 = most played
	FetchedAt int64       `json:"fetchedAt"` // unix seconds; 0 never
	State     string      `json:"state"`
	Error     string      `json:"error,omitempty"`
}

// Score is one Steam review summary.
type Score struct {
	Label   string `json:"label,omitempty"` // Steam's own words: "Very Positive"
	Percent int    `json:"percent"`         // positive share 0..100
	Total   int    `json:"total"`           // 0: no reviews (Label says so) or unknown
}

// ReviewSummary is a game's Steam review scores.
type ReviewSummary struct {
	AppID     int    `json:"appId"`
	Overall   Score  `json:"overall"`
	Recent    Score  `json:"recent"` // the last 30 days; Total 0 when Steam gives none
	URL       string `json:"url"`    // the game's reviews on Steam
	FetchedAt int64  `json:"fetchedAt"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

// Review is one Steam user review, shown as plain text with its link.
type Review struct {
	ID          string `json:"id"`     // Steam's recommendation id
	Author      string `json:"author"` // persona name when given, else the SteamID
	Recommended bool   `json:"recommended"`
	Text        string `json:"text"` // plain text; never rendered as HTML
	Language    string `json:"language,omitempty"`
	Helpful     int    `json:"helpful"`
	Funny       int    `json:"funny"`
	// Playtime in minutes; 0 when Steam doesn't say.
	PlaytimeAtReview int    `json:"playtimeAtReview"`
	PlaytimeForever  int    `json:"playtimeForever"`
	Posted           int64  `json:"posted"`
	URL              string `json:"url"` // the review on Steam
}

// Review filters (ReviewQuery.Filter).
const (
	ReviewsHelpful = "helpful" // Steam's "all" ordering: most helpful
	ReviewsRecent  = "recent"
)

// ReviewQuery asks for one page of reviews.
type ReviewQuery struct {
	AppID    int    `json:"appId"`
	Cursor   string `json:"cursor"`   // "" or "*" for the first page; then ReviewPage.Cursor
	Filter   string `json:"filter"`   // ReviewsHelpful or ReviewsRecent
	Language string `json:"language"` // Steam language name ("english"); "" all
}

// ReviewPage is one page of reviews.
type ReviewPage struct {
	AppID   int      `json:"appId"`
	Reviews []Review `json:"reviews"`
	Cursor  string   `json:"cursor"` // for the next page
	More    bool     `json:"more"`
	State   string   `json:"state"`
	Error   string   `json:"error,omitempty"`
}

// Critic is the Metacritic score and link Steam's game metadata gives.
// Seaglass doesn't fetch critic articles.
type Critic struct {
	AppID     int    `json:"appId"`
	Score     int    `json:"score"` // 0: Steam gives none
	URL       string `json:"url,omitempty"`
	FetchedAt int64  `json:"fetchedAt"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

// Completion is a game's HowLongToBeat times, in minutes; 0 is unknown.
type Completion struct {
	HLTBID        int    `json:"hltbId"` // 0: no confident match
	Title         string `json:"title,omitempty"`
	Main          int    `json:"main"`
	MainExtras    int    `json:"mainExtras"`
	Completionist int    `json:"completionist"`
	URL           string `json:"url"` // the game's page, or a search on HowLongToBeat
	// Corrected: the person chose this match.
	Corrected bool   `json:"corrected"`
	FetchedAt int64  `json:"fetchedAt"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
}

// CompletionQuery identifies the game to time.
type CompletionQuery struct {
	Title  string `json:"title"`
	Year   int    `json:"year"`   // release year when known; helps tell remakes apart
	HLTBID int    `json:"hltbId"` // a match the person chose; 0 to search
}

// Candidate is one HowLongToBeat search result the person can choose.
type Candidate struct {
	HLTBID        int    `json:"hltbId"`
	Title         string `json:"title"`
	Year          int    `json:"year"`
	Type          string `json:"type"` // HowLongToBeat's kind of entry: game, dlc, mod, …; "" when not given
	Main          int    `json:"main"`
	MainExtras    int    `json:"mainExtras"`
	Completionist int    `json:"completionist"`
	URL           string `json:"url"`
}

// Enrichment is everything known about one game, for its page and cards.
type Enrichment struct {
	Key        string        `json:"key"` // the game's discovery key
	SteamAppID int           `json:"steamAppId"`
	Reviews    ReviewSummary `json:"reviews"`
	Critic     Critic        `json:"critic"`
	Completion Completion    `json:"completion"`
	// PopularRank is the game's place on the chart; 0 off the chart or unknown.
	PopularRank int `json:"popularRank"`
}
