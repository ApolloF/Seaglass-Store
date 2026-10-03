package app

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/library"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/store/enrich"
)

// LibraryCompletion is a library game's HowLongToBeat times. They work
// with the Store off: the cache, its seven-day lifetime and the person's
// match corrections are the Store's, shared through Key.
type LibraryCompletion struct {
	GameID int64 `json:"gameId"`
	// Key is the identity the times and a corrected match are kept under,
	// shared with the Store: "steam:<appid>" for a trusted Steam identity,
	// else "title:<normalized title>". Never the library's folder key.
	Key        string            `json:"key"`
	Completion enrich.Completion `json:"completion"`
}

// completionHosts are the only sites OpenCompletionLink opens.
var completionHosts = []string{"howlongtobeat.com", "www.howlongtobeat.com"}

// completionFetchTimeout bounds one lookup: a search, a token and a page.
const completionFetchTimeout = 30 * time.Second

// trustedSteamApp is the game's Steam app when its identity can be relied
// on, as the Store relies on it: Steam's own record, the person's
// confirmation, or a match at or above the confidence the metadata worker
// gives a store hit. MetaAppID only feeds metadata and is never an identity.
func trustedSteamApp(g library.Game) int {
	if g.SteamAppID <= 0 {
		return 0
	}
	if g.Source == "steam" || g.Confirmed || g.Confidence >= storeMatchConfidence {
		return g.SteamAppID
	}
	return 0
}

// completionIdentity is the key a game's times and corrections live under
// in the Store, and the title to look it up by (Steam's name for a Steam
// game, else the one shown).
func completionIdentity(g library.Game) (key, title string) {
	if appID := trustedSteamApp(g); appID > 0 {
		return "steam:" + strconv.Itoa(appID), g.Title
	}
	return "title:" + scan.Normalize(g.DisplayTitle()), g.DisplayTitle()
}

func (s *LibraryService) completionGame(id int64) (library.Game, string, string, error) {
	g, ok := s.c.Lib.Get(id)
	if !ok {
		return g, "", "", library.ErrNotFound
	}
	key, title := completionIdentity(g)
	if strings.TrimSpace(strings.TrimPrefix(key, "title:")) == "" {
		return g, "", "", errors.New("this game has no title to look up")
	}
	return g, key, title, nil
}

func (s *LibraryService) completionQuery(g library.Game, key, title string) enrich.CompletionQuery {
	q := enrich.CompletionQuery{Title: title, HLTBID: s.c.discovery.index().CompletionMatch(key)}
	if g.Meta != nil {
		q.Year = g.Meta.ReleaseYear
	}
	return q
}

// Completion is a game's completion times: cached ones at once, fetched
// first when fetch is set and they're missing or older than seven days.
// A failure keeps cached times (stale) or says they're unavailable.
func (s *LibraryService) Completion(id int64, fetch bool) (LibraryCompletion, error) {
	g, key, title, err := s.completionGame(id)
	if err != nil {
		return LibraryCompletion{GameID: id, Completion: enrich.Completion{State: enrich.StateUnavailable}}, err
	}
	q := s.completionQuery(g, key, title)
	out := LibraryCompletion{GameID: id, Key: key}
	if fetch {
		ctx, cancel := context.WithTimeout(s.c.ctx, completionFetchTimeout)
		defer cancel()
		out.Completion = s.c.enrich.client.Completion(ctx, q)
		return out, nil
	}
	if c, ok := s.c.enrich.client.CachedCompletion(q); ok {
		out.Completion = c
	} else {
		out.Completion = enrich.Completion{State: enrich.StateLoading}
	}
	return out, nil
}

// CompletionCandidates searches HowLongToBeat for the game's other
// possible matches ("" searches its title).
func (s *LibraryService) CompletionCandidates(id int64, query string) ([]enrich.Candidate, error) {
	_, _, title, err := s.completionGame(id)
	if err != nil {
		return []enrich.Candidate{}, err
	}
	if strings.TrimSpace(query) != "" {
		title = query
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, completionFetchTimeout)
	defer cancel()
	found, err := s.c.enrich.client.Candidates(ctx, title)
	if found == nil {
		found = []enrich.Candidate{}
	}
	return found, err
}

// SetCompletionMatch makes hltbID the game's match, shared with the
// Store's page for the same game (0 goes back to the automatic match).
func (s *LibraryService) SetCompletionMatch(id int64, hltbID int) (LibraryCompletion, error) {
	if hltbID < 0 {
		return LibraryCompletion{GameID: id, Completion: enrich.Completion{State: enrich.StateUnavailable}}, errors.New("that isn't a HowLongToBeat game")
	}
	_, key, _, err := s.completionGame(id)
	if err != nil {
		return LibraryCompletion{GameID: id, Completion: enrich.Completion{State: enrich.StateUnavailable}}, err
	}
	if err := s.c.discovery.index().SetCompletionMatch(key, hltbID); err != nil {
		return LibraryCompletion{GameID: id, Key: key, Completion: enrich.Completion{State: enrich.StateUnavailable}}, err
	}
	return s.Completion(id, true)
}

// completionLink checks that raw is an HTTPS HowLongToBeat page.
func completionLink(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") || !slices.Contains(completionHosts, strings.ToLower(u.Hostname())) {
		return "", errors.New("that link doesn't go to HowLongToBeat")
	}
	return u.String(), nil
}

// OpenCompletionLink opens a HowLongToBeat link in the browser.
func (s *LibraryService) OpenCompletionLink(raw string) error {
	link, err := completionLink(raw)
	if err != nil {
		return err
	}
	return platform.OpenWebPage(link)
}
