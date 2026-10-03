package app

import "errors"

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
	Available int            `json:"available"` // with a known source release now
	Searching int            `json:"searching"` // queued for a source search while idle
	Items     []WishlistItem `json:"items"`
}

// SteamWishlistAccount is the Steam account detected on this PC, to
// prefill the import.
func (s *StoreService) SteamWishlistAccount() SteamAccount {
	return SteamAccount{Error: "not available yet"}
}

// ImportSteamWishlist reads a public Steam wishlist and saves its games,
// merging by Steam AppID; it never removes a saved game. Only the person
// starts it.
func (s *StoreService) ImportSteamWishlist(steamID string) (WishlistImport, error) {
	return WishlistImport{Items: []WishlistItem{}}, errors.New("importing isn't available yet")
}
