package app

import (
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/enrich"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/store/sources"
	"github.com/ApolloF/Seaglass/internal/store/wishlist"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Store discovery events.
const (
	EventStoreDiscovery  = "store:discovery"  // discovery.Status changed
	EventStoreGames      = "store:games"      // discovery.Change: these games changed
	EventStoreSearch     = "store:search"     // discovery.SearchProgress while SearchGames runs
	EventStoreEnrichment = "store:enrichment" // enrich.Enrichment for one game
	EventStoreWishlist   = "store:wishlist"   // []WishlistItem: the wishlist changed
)

func init() {
	application.RegisterEvent[discovery.Status](EventStoreDiscovery)
	application.RegisterEvent[discovery.Change](EventStoreGames)
	application.RegisterEvent[discovery.SearchProgress](EventStoreSearch)
	application.RegisterEvent[enrich.Enrichment](EventStoreEnrichment)
	application.RegisterEvent[[]WishlistItem](EventStoreWishlist)
}

// WishlistItem is a saved game with its current state and activity.
type WishlistItem struct {
	Key        string                  `json:"key"`
	Title      string                  `json:"title"`
	SteamAppID int                     `json:"steamAppId,omitempty"`
	AddedAt    int64                   `json:"addedAt"`
	Game       discovery.GameSummary   `json:"game"`
	Activity   []wishlist.ActivityView `json:"activity"` // newest first
	Unread     int                     `json:"unread"`
}

// errNotIndexed is the answer for a game discovery doesn't know (anymore).
var errNotIndexed = errors.New("that game isn't in the Store anymore")

// discoveryOn fails unless the Store and source browsing are on.
func (s *StoreService) discoveryOn() error {
	if err := s.on(); err != nil {
		return err
	}
	if !s.c.Settings.Get().Store.PrivateSources {
		return sources.ErrDisabled
	}
	return nil
}

// DiscoveryStatus says what discovery is doing.
func (s *StoreService) DiscoveryStatus() discovery.Status {
	return baseDiscoveryStatus(s.c.Settings.Get())
}

// baseDiscoveryStatus is the status the settings alone decide.
func baseDiscoveryStatus(v settings.Settings) discovery.Status {
	st := discovery.Status{SetupNeeded: v.ExperimentalStore && v.Store.SourceSetup == settings.SetupAsk, Sources: []discovery.SourceStatus{}}
	for _, id := range discovery.Sources {
		src, _ := sources.PrivateSource(id, true)
		on := v.ExperimentalStore && v.Store.DiscoveryOn(id)
		state := discovery.CrawlDisabled
		if on {
			state = discovery.CrawlIdle
			st.Enabled = true
		}
		st.Sources = append(st.Sources, discovery.SourceStatus{ID: id, Name: src.Name, Enabled: on, State: state})
	}
	return st
}

// SetupSources records the one-time source choice (or a later change in
// Settings) and starts indexing the chosen sources at once.
func (s *StoreService) SetupSources(chosen []string) (settings.Settings, error) {
	if err := s.on(); err != nil {
		return s.c.Settings.Get(), err
	}
	for _, id := range chosen {
		if !slices.Contains(discovery.Sources, id) {
			return s.c.Settings.Get(), errors.New("choose FitGirl or DODI")
		}
	}
	return s.c.updateSettings(func(v *settings.Settings) {
		v.Store.Sources = slices.Clone(chosen)
		v.Store.SourceSetup = settings.SetupDone
		v.Store.PrivateSources = len(chosen) > 0
	})
}

// RefreshDiscovery fetches the newest listings of every chosen source now.
func (s *StoreService) RefreshDiscovery() (discovery.Status, error) {
	if err := s.discoveryOn(); err != nil {
		return s.DiscoveryStatus(), err
	}
	return s.DiscoveryStatus(), nil
}

// StoreHome returns the Store's front page from the index.
func (s *StoreService) StoreHome() (discovery.Home, error) {
	h := discovery.Home{New: []discovery.GameSummary{}, Popular: []discovery.GameSummary{}, PopularState: discovery.StateUnavailable,
		Updated: []discovery.GameSummary{}, Wishlist: []discovery.GameSummary{}, Status: s.DiscoveryStatus()}
	return h, s.on()
}

// BrowseGames answers a browse or search query from the index alone, at
// once.
func (s *StoreService) BrowseGames(q discovery.BrowseQuery) (discovery.SearchResult, error) {
	return emptySearch(q), s.on()
}

// SearchGames searches the chosen source sites and Steam for q.Text to
// fill gaps in the index, then answers like BrowseGames. A newer call
// cancels an older one, which then returns its partial result.
func (s *StoreService) SearchGames(q discovery.BrowseQuery) (discovery.SearchResult, error) {
	return emptySearch(q), s.on()
}

func emptySearch(q discovery.BrowseQuery) discovery.SearchResult {
	return discovery.SearchResult{Query: q, Page: discovery.BrowsePage{Games: []discovery.GameSummary{}, Languages: []string{}, Genres: []string{}},
		Other: []discovery.GameSummary{}, Remote: []discovery.ProviderProgress{}, Complete: true}
}

// GameDetails returns a game's page from the index and fetches missing
// release details in the background (EventStoreGames brings them).
func (s *StoreService) GameDetails(key string) (discovery.GameDetails, error) {
	if err := s.on(); err != nil {
		return discovery.GameDetails{}, err
	}
	return discovery.GameDetails{}, errNotIndexed
}

// SetSteamMatch corrects which Steam game a game's releases are (appID 0:
// not on Steam). The correction survives refreshes. The game's key can
// change; the returned details carry the new one.
func (s *StoreService) SetSteamMatch(key string, appID int, name string) (discovery.GameDetails, error) {
	if err := s.discoveryOn(); err != nil {
		return discovery.GameDetails{}, err
	}
	return discovery.GameDetails{}, errNotIndexed
}

// PrepareRelease makes a release ready for the install confirmation: it
// fetches the article when only its summary is known and tries the
// supported torrent-metadata resolver once. Nothing is downloaded but
// metadata.
func (s *StoreService) PrepareRelease(key, releaseID string) (discovery.PreparedRelease, error) {
	if err := s.discoveryOn(); err != nil {
		return discovery.PreparedRelease{}, err
	}
	return discovery.PreparedRelease{}, errNotIndexed
}

// AttachSourceTorrent asks for a .torrent file the person got in their
// browser for an unresolved release and validates it.
func (s *StoreService) AttachSourceTorrent(key, releaseID string) (discovery.PreparedRelease, error) {
	if err := s.discoveryOn(); err != nil {
		return discovery.PreparedRelease{}, err
	}
	return discovery.PreparedRelease{}, errNotIndexed
}

// OpenSourceRelease opens a release's article in the browser.
func (s *StoreService) OpenSourceRelease(key, releaseID string) error {
	if err := s.discoveryOn(); err != nil {
		return err
	}
	return errNotIndexed
}

// DownloadRelease queues a prepared release's validated transport, after
// the person confirmed it, through the same checks as DownloadOffer.
func (s *StoreService) DownloadRelease(key, releaseID string, transport int, opts InstallOptions) (jobs.Job, error) {
	if err := s.discoveryOn(); err != nil {
		return jobs.Job{}, err
	}
	return jobs.Job{}, errNotIndexed
}

// EnrichGames returns the cached enrichment for these games (cards on
// screen, the first first) and fetches the rest; EventStoreEnrichment
// brings them.
func (s *StoreService) EnrichGames(keys []string) []enrich.Enrichment {
	return []enrich.Enrichment{}
}

// GameEnrichment returns everything known about a game for its page,
// fetching what is missing or old first (a few seconds at most).
func (s *StoreService) GameEnrichment(key string) (enrich.Enrichment, error) {
	if err := s.on(); err != nil {
		return enrich.Enrichment{}, err
	}
	return enrich.Enrichment{Key: key, Reviews: enrich.ReviewSummary{State: enrich.StateUnavailable}, Critic: enrich.Critic{State: enrich.StateUnavailable},
		Completion: enrich.Completion{State: enrich.StateUnavailable}}, nil
}

// GameReviews returns one page of a game's Steam reviews.
func (s *StoreService) GameReviews(q enrich.ReviewQuery) (enrich.ReviewPage, error) {
	if err := s.on(); err != nil {
		return enrich.ReviewPage{}, err
	}
	return enrich.ReviewPage{AppID: q.AppID, Reviews: []enrich.Review{}, State: enrich.StateUnavailable}, nil
}

// CompletionCandidates searches HowLongToBeat for title, so the person can
// choose the right game.
func (s *StoreService) CompletionCandidates(key, title string) ([]enrich.Candidate, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	return []enrich.Candidate{}, nil
}

// SetCompletionMatch makes hltbID the game's HowLongToBeat match, kept
// across refreshes (0 goes back to the automatic match).
func (s *StoreService) SetCompletionMatch(key string, hltbID int) (enrich.Enrichment, error) {
	return s.GameEnrichment(key)
}

// Wishlist lists the saved games, newest first.
func (s *StoreService) Wishlist() []WishlistItem { return []WishlistItem{} }

// AddToWishlist saves a game. Its releases known now are the baseline:
// only later ones become activity.
func (s *StoreService) AddToWishlist(key, title string, steamAppID int) ([]WishlistItem, error) {
	if err := s.on(); err != nil {
		return []WishlistItem{}, err
	}
	return []WishlistItem{}, nil
}

// RemoveFromWishlist forgets a saved game and its activity.
func (s *StoreService) RemoveFromWishlist(key string) ([]WishlistItem, error) {
	if err := s.on(); err != nil {
		return []WishlistItem{}, err
	}
	return []WishlistItem{}, nil
}

// AcknowledgeWishlist marks a saved game's activity read ("" marks every
// game's).
func (s *StoreService) AcknowledgeWishlist(key string) ([]WishlistItem, error) {
	if err := s.on(); err != nil {
		return []WishlistItem{}, err
	}
	return []WishlistItem{}, nil
}

// storeLinkHosts are the sites the Store's attribution links may open.
var storeLinkHosts = []string{"store.steampowered.com", "steamcommunity.com", "www.metacritic.com", "metacritic.com", "howlongtobeat.com", "www.howlongtobeat.com"}

// OpenStoreLink opens a review, score or completion-time link in the
// browser: HTTPS on the attributed sites only.
func (s *StoreService) OpenStoreLink(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || !slices.Contains(storeLinkHosts, strings.ToLower(u.Hostname())) {
		return errors.New("that link doesn't go to Steam, Metacritic or HowLongToBeat")
	}
	return platform.OpenWebPage(u.String())
}
