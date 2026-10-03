package app

import (
	"errors"

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

// Completion is a game's completion times: cached ones at once, fetched
// first when fetch is set and they're missing or older than seven days.
// A failure keeps cached times (stale) or says they're unavailable.
func (s *LibraryService) Completion(id int64, fetch bool) (LibraryCompletion, error) {
	return LibraryCompletion{GameID: id, Completion: enrich.Completion{State: enrich.StateUnavailable}}, errors.New("completion times aren't available yet")
}

// CompletionCandidates searches HowLongToBeat for the game's other
// possible matches ("" searches its title).
func (s *LibraryService) CompletionCandidates(id int64, query string) ([]enrich.Candidate, error) {
	return []enrich.Candidate{}, errors.New("completion times aren't available yet")
}

// SetCompletionMatch makes hltbID the game's match, shared with the
// Store's page for the same game (0 goes back to the automatic match).
func (s *LibraryService) SetCompletionMatch(id int64, hltbID int) (LibraryCompletion, error) {
	return s.Completion(id, true)
}

// OpenCompletionLink opens a HowLongToBeat link in the browser.
func (s *LibraryService) OpenCompletionLink(raw string) error {
	return errors.New("completion times aren't available yet")
}
