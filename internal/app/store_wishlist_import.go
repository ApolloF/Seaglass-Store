package app

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/owned"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/enrich"
	"github.com/ApolloF/Seaglass/internal/store/wishlist"
)

// SteamAccount is the Steam account a wishlist import starts from.
type SteamAccount struct {
	SteamID  string `json:"steamId"`         // SteamID64; "" when none was found
	Detected bool   `json:"detected"`        // the account Steam signs in with on this PC
	Error    string `json:"error,omitempty"` // why none was found
}

// WishlistImport says what importing a Steam wishlist did.
type WishlistImport struct {
	SteamID   string         `json:"steamId"`
	Fetched   int            `json:"fetched"`   // games on the Steam wishlist
	Added     int            `json:"added"`     // newly saved
	Existing  int            `json:"existing"`  // already saved, matched by Steam AppID
	Skipped   int            `json:"skipped"`   // left out because the wishlist is full
	Available int            `json:"available"` // with a known source release now
	Searching int            `json:"searching"` // queued for a source search while idle
	Items     []WishlistItem `json:"items"`
}

// steamAccountID finds the SteamID64 Steam signs in with on this PC.
var steamAccountID = owned.SteamID

// SteamWishlistAccount is the Steam account detected on this PC, to
// prefill the import.
func (s *StoreService) SteamWishlistAccount() SteamAccount {
	id, err := steamAccountID()
	switch {
	case err != nil:
		return SteamAccount{Error: sentence(err.Error())}
	case !enrich.ValidSteamID64(id):
		return SteamAccount{Error: "Steam's account on this PC has no usable SteamID64."}
	}
	return SteamAccount{SteamID: id, Detected: true}
}

// sentence makes an error message read as a sentence.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// ImportSteamWishlist reads a public Steam wishlist and saves its games,
// merging by Steam AppID; it never removes a saved game. Only the person
// starts it.
func (s *StoreService) ImportSteamWishlist(steamID string) (WishlistImport, error) {
	steamID = strings.TrimSpace(steamID)
	out := WishlistImport{SteamID: steamID, Items: []WishlistItem{}}
	if err := s.on(); err != nil {
		return out, err
	}
	if !enrich.ValidSteamID64(steamID) {
		return out, enrich.ErrSteamID
	}
	ctx, cancel := context.WithTimeout(s.c.ctx, time.Minute)
	defer cancel()
	ids, err := s.c.enrich.client.SteamWishlist(ctx, steamID)
	if err != nil {
		return out, err
	}
	res, err := s.c.wishlist.importSteam(ids)
	res.SteamID = steamID
	return res, err
}

// importSteam saves the games of a Steam wishlist that aren't saved yet,
// with the releases known now as their baseline, and queues those without
// a known source release for idle source searches.
func (w *wishlistState) importSteam(ids []int) (WishlistImport, error) {
	d := w.c.discovery
	view := d.currentView()
	out := WishlistImport{Fetched: len(ids), Items: []WishlistItem{}}
	games := make([]wishlist.Imported, 0, len(ids))
	// Steam's order is its priority: when the wishlist is full, the first
	// games are the ones kept.
	for i := range ids {
		key := steamKey(ids[i])
		title := w.knownName(ids[i])
		if title == "" {
			title = wishlist.Placeholder(ids[i])
		}
		games = append(games, wishlist.Imported{Key: key, Title: title, AppID: ids[i], Known: w.observations(d, key)})
	}
	added, existing, err := w.store.Import(games, time.Now())
	out.Added, out.Existing, out.Skipped = added, existing, len(ids)-added-existing
	var unresolved []string
	for _, id := range ids {
		key := steamKey(id)
		if g, ok := view.Game(key); ok && hasRelease(g) {
			out.Available++
		} else if _, saved := w.store.Get(key); saved {
			unresolved = append(unresolved, key)
		}
	}
	if w.canSearch() {
		n, qerr := w.queue().Enqueue(unresolved, time.Now())
		w.logErr(qerr)
		out.Searching = n
		w.wakeSearches()
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, steamKey(id))
	}
	w.c.emit(EventStoreGames, discovery.Change{Keys: keys})
	w.changed()
	out.Items = w.items()
	return out, err
}

// hasRelease says whether a game has a source release; an announcement
// alone is not one.
func hasRelease(g *discovery.Game) bool {
	return slices.ContainsFunc(g.Records, func(r discovery.Record) bool { return r.Entry.ReleaseKind != "preview" })
}

func steamKey(appID int) string { return "steam:" + strconv.Itoa(appID) }

// knownName is a Steam game's name from what this PC knows: the index,
// Seaglass's game database, or a Steam search answered earlier. "" when
// none knows it.
func (w *wishlistState) knownName(appID int) string {
	d := w.c.discovery
	if g, ok := d.currentView().Game(steamKey(appID)); ok && g.Title != "" {
		return g.Title
	}
	if w.c.Manifest != nil {
		if n := w.c.Manifest.Index().SteamName(appID); n != "" {
			return n
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.steamNames[appID]
}

// nameImported replaces the placeholder titles of imported games whose
// names became known.
func (w *wishlistState) nameImported(entries []wishlist.Entry) bool {
	changed := false
	for _, e := range entries {
		if e.SteamAppID == 0 || e.Title != wishlist.Placeholder(e.SteamAppID) {
			continue
		}
		if n := w.knownName(e.SteamAppID); n != "" {
			if ok, _ := w.store.Name(e.Key, n); ok {
				changed = true
			}
		}
	}
	return changed
}
