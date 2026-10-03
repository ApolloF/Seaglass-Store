package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
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
	Origin     string                  `json:"origin"` // wishlist.Origin*: saved here or imported from Steam
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
	return s.c.discovery.status()
}

// baseDiscoveryStatus is the status the settings alone decide.
func baseDiscoveryStatus(v settings.Settings) discovery.Status {
	st := discovery.Status{SetupNeeded: v.ExperimentalStore && v.Store.SourceSetup == settings.SetupAsk, Sources: []discovery.SourceStatus{}}
	for _, p := range sources.Providers() {
		on := v.ExperimentalStore && v.Store.DiscoveryOn(p.ID)
		state := discovery.CrawlDisabled
		if on {
			state = discovery.CrawlIdle
			st.Enabled = true
		}
		st.Sources = append(st.Sources, discovery.SourceStatus{ID: p.ID, Name: p.Name, Enabled: on, State: state,
			Host: p.Host, Search: p.Search != "", Paged: p.Paged(), Torrents: p.Torrents, DefaultOn: p.DefaultOn, Notes: append([]string{}, p.Notes...)})
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
			return s.c.Settings.Get(), fmt.Errorf("unknown source %q", id)
		}
	}
	saved, err := s.c.updateSettings(func(v *settings.Settings) {
		v.Store.Sources = slices.Clone(chosen)
		v.Store.SourceSetup = settings.SetupDone
		v.Store.PrivateSources = len(chosen) > 0
	})
	if err == nil {
		s.c.discovery.start()
	}
	return saved, err
}

// PauseIndexing pauses or resumes background indexing on this PC. Paused,
// nothing is requested from any source until resumed; the index stays.
func (s *StoreService) PauseIndexing(paused bool) (discovery.Status, error) {
	return s.DiscoveryStatus(), errors.New("pausing isn't available yet")
}

// RefreshDiscovery fetches the newest listings of every chosen source now.
func (s *StoreService) RefreshDiscovery() (discovery.Status, error) {
	if err := s.discoveryOn(); err != nil {
		return s.DiscoveryStatus(), err
	}
	err := s.c.discovery.refresh()
	return s.DiscoveryStatus(), err
}

// StoreHome returns the Store's front page from the index.
func (s *StoreService) StoreHome() (discovery.Home, error) {
	if err := s.on(); err != nil {
		return discovery.Home{New: []discovery.GameSummary{}, Popular: []discovery.GameSummary{}, PopularState: discovery.StateUnavailable,
			Updated: []discovery.GameSummary{}, Wishlist: []discovery.GameSummary{}, Featured: []discovery.GameSummary{},
			Recommended: []discovery.Recommendation{}, Status: s.DiscoveryStatus()}, err
	}
	d := s.c.discovery
	view, annotate := d.currentView(), d.annotator()
	h := view.Home(annotate, s.c.popularState(), d.status())
	h.Recommended, h.RecommendedBasis = s.c.recommendations(view, annotate)
	return h, nil
}

// BrowseGames answers a browse or search query from the index alone, at
// once.
func (s *StoreService) BrowseGames(q discovery.BrowseQuery) (discovery.SearchResult, error) {
	if err := s.on(); err != nil {
		return emptySearch(q), err
	}
	d := s.c.discovery
	res := emptySearch(q)
	res.Page = d.currentView().Browse(q, d.annotator())
	// Answers from the last hour are shown at once, the rest after SearchGames.
	if hits, ok := d.search.Steam(q.Text, time.Now()); ok {
		res.Other = d.others(hits, q.Text)
	}
	return res, nil
}

// SearchGames searches the chosen source sites and Steam for q.Text to
// fill gaps in the index, then answers like BrowseGames. A newer call
// cancels an older one, which then returns its partial result.
func (s *StoreService) SearchGames(q discovery.BrowseQuery) (discovery.SearchResult, error) {
	if err := s.on(); err != nil {
		return emptySearch(q), err
	}
	d := s.c.discovery
	res := emptySearch(q)
	remote, hits, complete := d.searchRemote(q.Text)
	res.Remote, res.Complete = remote, complete
	res.Page = d.currentView().Browse(q, d.annotator())
	res.Other = d.others(hits, q.Text)
	return res, nil
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
	return s.c.discovery.details(key)
}

// SetSteamMatch corrects which Steam game a game's releases are (appID 0:
// not on Steam). The correction survives refreshes. The game's key can
// change; the returned details carry the new one.
func (s *StoreService) SetSteamMatch(key string, appID int, name string) (discovery.GameDetails, error) {
	if err := s.discoveryOn(); err != nil {
		return discovery.GameDetails{}, err
	}
	if appID < 0 {
		return discovery.GameDetails{}, errors.New("that isn't a Steam game")
	}
	d := s.c.discovery
	g, ok := d.currentView().Game(key)
	if !ok || len(g.TitleKeys) == 0 {
		return discovery.GameDetails{}, errNotIndexed
	}
	name = strings.TrimSpace(name)
	if appID > 0 && name == "" {
		return discovery.GameDetails{}, errors.New("choose the Steam game by name")
	}
	var cs []discovery.IdentityCorrection
	for _, tk := range g.TitleKeys {
		cs = append(cs, discovery.IdentityCorrection{TitleKey: tk, SteamAppID: appID, Name: name, At: time.Now()})
	}
	if err := d.index().SetSteamCorrections(cs); err != nil {
		return discovery.GameDetails{}, err
	}
	newKey := "title:" + g.TitleKeys[0]
	if appID > 0 {
		newKey = "steam:" + strconv.Itoa(appID)
	}
	if s.c.wishlist != nil && newKey != key {
		s.c.wishlist.rekey(key, newKey, name, appID)
	}
	s.c.emit(EventStoreGames, discovery.Change{Keys: []string{key, newKey}})
	return d.details(newKey)
}

// PrepareRelease makes a release ready for the install confirmation: it
// fetches the article when only its summary is known and tries the
// supported torrent-metadata resolver once. Nothing is downloaded but
// metadata.
func (s *StoreService) PrepareRelease(key, releaseID string) (discovery.PreparedRelease, error) {
	if err := s.discoveryOn(); err != nil {
		return discovery.PreparedRelease{}, err
	}
	return s.c.discovery.prepare(key, releaseID)
}

// AttachSourceTorrent asks for a .torrent file the person got in their
// browser for an unresolved release and validates it.
func (s *StoreService) AttachSourceTorrent(key, releaseID string) (discovery.PreparedRelease, error) {
	if err := s.discoveryOn(); err != nil {
		return discovery.PreparedRelease{}, err
	}
	d := s.c.discovery
	g, r, err := d.release(key, releaseID)
	if err != nil {
		return discovery.PreparedRelease{}, err
	}
	path, err := application.Get().Dialog.OpenFile().SetTitle("Choose the .torrent file for "+g.Title).
		CanChooseFiles(true).CanChooseDirectories(false).AddFilter("Torrent metadata", "*.torrent").PromptForSingleSelection()
	if err != nil {
		return discovery.PreparedRelease{}, err
	}
	if path == "" {
		return d.prepared(key, releaseID)
	}
	data, err := readLimited(path, (2<<20)+1)
	if err != nil {
		return discovery.PreparedRelease{}, err
	}
	t, err := sources.TorrentMetadata(data)
	if err != nil {
		return discovery.PreparedRelease{}, err
	}
	if err := s.discoveryOn(); err != nil {
		return discovery.PreparedRelease{}, err
	}
	discovery.Attach(d.index(), r.Entry.SourceID, releaseID, t)
	d.save()
	s.c.emit(EventStoreGames, discovery.Change{Keys: []string{key}})
	return d.prepared(key, releaseID)
}

// readLimited reads at most limit bytes of a file the person chose.
func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// OpenSourceRelease opens a release's article in the browser.
func (s *StoreService) OpenSourceRelease(key, releaseID string) error {
	if err := s.discoveryOn(); err != nil {
		return err
	}
	_, r, err := s.c.discovery.release(key, releaseID)
	if err != nil {
		return err
	}
	src, err := sources.PrivateSource(r.Entry.SourceID, true)
	if err != nil {
		return err
	}
	if _, err := src.ValidateURL(r.Entry.PageURL); err != nil {
		return err
	}
	return platform.OpenWebPage(r.Entry.PageURL)
}

// DownloadRelease queues a prepared release's validated transport, after
// the person confirmed it, through the same checks as DownloadOffer.
func (s *StoreService) DownloadRelease(key, releaseID string, transport int, opts InstallOptions) (jobs.Job, error) {
	if err := s.discoveryOn(); err != nil {
		return jobs.Job{}, err
	}
	d := s.c.discovery
	g, r, err := d.release(key, releaseID)
	if err != nil {
		return jobs.Job{}, err
	}
	if r.Gone {
		return jobs.Job{}, errors.New("the source no longer lists this release")
	}
	if !slices.Contains(validTransports(r.Entry), transport) {
		return jobs.Job{}, errors.New("this release has no validated torrent: prepare it again")
	}
	it, err := sourceOffer(r.Entry, transport)
	if err != nil {
		return jobs.Job{}, err
	}
	if langs, complete := discovery.ParseLanguages(r.Entry.LanguageClaim); complete {
		it.Languages = langs
	}
	src, err := sources.PrivateSource(r.Entry.SourceID, true)
	if err != nil {
		return jobs.Job{}, err
	}
	e := catalog.Entry{Key: key, Title: g.Title, SteamAppID: g.AppID, Offers: []catalog.Offer{{Item: it, FeedURL: src.StartURL, FeedName: src.Name}}}
	s.c.catalog.annotator()(&e)
	return s.queueOffer(e, 0, opts)
}

// EnrichGames returns the cached enrichment for these games (cards on
// screen, the first first) and fetches the rest; EventStoreEnrichment
// brings them.
func (s *StoreService) EnrichGames(keys []string) []enrich.Enrichment {
	if s.on() != nil || len(keys) > 500 {
		return []enrich.Enrichment{}
	}
	return s.c.enrich.request(keys)
}

// GameEnrichment returns everything known about a game for its page,
// fetching what is missing or old first (a few seconds at most).
func (s *StoreService) GameEnrichment(key string) (enrich.Enrichment, error) {
	if err := s.on(); err != nil {
		return enrich.Enrichment{}, err
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 30*time.Second)
	defer cancel()
	return s.c.enrich.game(ctx, key)
}

// GameReviews returns one page of a game's Steam reviews.
func (s *StoreService) GameReviews(q enrich.ReviewQuery) (enrich.ReviewPage, error) {
	if err := s.on(); err != nil {
		return enrich.ReviewPage{}, err
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 30*time.Second)
	defer cancel()
	return s.c.enrich.client.Reviews(ctx, q), nil
}

// CompletionCandidates searches HowLongToBeat for title, so the person can
// choose the right game.
func (s *StoreService) CompletionCandidates(key, title string) ([]enrich.Candidate, error) {
	if err := s.on(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" {
		t, _, ok := s.c.storeGameRef(key)
		if !ok {
			return nil, errNotIndexed
		}
		title = t
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, 30*time.Second)
	defer cancel()
	found, err := s.c.enrich.client.Candidates(ctx, title)
	if found == nil {
		found = []enrich.Candidate{}
	}
	return found, err
}

// SetCompletionMatch makes hltbID the game's HowLongToBeat match, kept
// across refreshes (0 goes back to the automatic match).
func (s *StoreService) SetCompletionMatch(key string, hltbID int) (enrich.Enrichment, error) {
	if err := s.on(); err != nil {
		return enrich.Enrichment{}, err
	}
	if hltbID < 0 {
		return enrich.Enrichment{}, errors.New("that isn't a HowLongToBeat game")
	}
	if _, _, ok := s.c.storeGameRef(key); !ok {
		return enrich.Enrichment{}, errNotIndexed
	}
	if err := s.c.discovery.index().SetCompletionMatch(key, hltbID); err != nil {
		return enrich.Enrichment{}, err
	}
	return s.GameEnrichment(key)
}

// Wishlist lists the saved games, newest first.
func (s *StoreService) Wishlist() []WishlistItem {
	if s.on() != nil {
		return []WishlistItem{}
	}
	return s.c.wishlist.items()
}

// AddToWishlist saves a game. Its releases known now are the baseline:
// only later ones become activity.
func (s *StoreService) AddToWishlist(key, title string, steamAppID int) ([]WishlistItem, error) {
	if err := s.on(); err != nil {
		return []WishlistItem{}, err
	}
	if err := s.c.wishlist.add(key, title, steamAppID); err != nil {
		return s.c.wishlist.items(), err
	}
	s.c.emit(EventStoreGames, discovery.Change{Keys: []string{key}})
	return s.c.wishlist.items(), nil
}

// RemoveFromWishlist forgets a saved game and its activity.
func (s *StoreService) RemoveFromWishlist(key string) ([]WishlistItem, error) {
	if err := s.on(); err != nil {
		return []WishlistItem{}, err
	}
	if err := s.c.wishlist.store.Remove(key); err != nil {
		return s.c.wishlist.items(), err
	}
	s.c.wishlist.changed()
	s.c.emit(EventStoreGames, discovery.Change{Keys: []string{key}})
	return s.c.wishlist.items(), nil
}

// AcknowledgeWishlist marks a saved game's activity read ("" marks every
// game's).
func (s *StoreService) AcknowledgeWishlist(key string) ([]WishlistItem, error) {
	if err := s.on(); err != nil {
		return []WishlistItem{}, err
	}
	if err := s.c.wishlist.store.Acknowledge(key); err != nil {
		return s.c.wishlist.items(), err
	}
	s.c.wishlist.changed()
	s.c.emit(EventStoreGames, discovery.Change{Keys: []string{}, All: key == ""})
	return s.c.wishlist.items(), nil
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
