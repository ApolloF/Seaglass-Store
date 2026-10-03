package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/ApolloF/Seaglass/internal/store/wishlist"
)

// Steam wishlists. IWishlistService/GetWishlist answers without a key for
// a profile whose game details are public, with AppIDs only; a private
// profile gets an answer without its items. Names come from what Seaglass
// knows, or from the store's appdetails one game at a time.

// MaxWishlist bounds the games accepted from one wishlist: no more than the
// saved wishlist can hold.
const MaxWishlist = wishlist.MaxEntries

// An individual account's SteamID64 is 76561197960265728 plus its account
// number, which runs from 1 to 2^32-1.
const (
	firstSteamID64 uint64 = 76561197960265729
	lastSteamID64  uint64 = 76561202255233023
)

// ErrSteamID means the text isn't a SteamID64.
var ErrSteamID = errors.New("That isn't a SteamID64. It is the 17-digit number of a Steam profile, for example 76561198000000042.")

// ErrWishlistPrivate means Steam answered but shared no wishlist.
var ErrWishlistPrivate = errors.New("Steam didn't share a wishlist for that account. In Steam, set the profile and its game details to Public, then try again.")

// ValidSteamID64 reports whether s is an individual account's SteamID64.
func ValidSteamID64(s string) bool {
	if !validSteamID(s) {
		return false
	}
	id, err := strconv.ParseUint(s, 10, 64)
	return err == nil && id >= firstSteamID64 && id <= lastSteamID64
}

// SteamWishlist reads a public Steam wishlist: its games' AppIDs in
// Steam's order, at most MaxWishlist. It isn't cached: only the person
// asks for it.
func (c *Client) SteamWishlist(ctx context.Context, steamID string) ([]int, error) {
	if !ValidSteamID64(steamID) {
		return nil, ErrSteamID
	}
	u := "https://api.steampowered.com/IWishlistService/GetWishlist/v1/?steamid=" + steamID
	b, err := c.fetch(ctx, request{url: u})
	if err != nil {
		return nil, steamError(err)
	}
	ids, err := parseWishlist(b)
	var format *errFormat
	if errors.As(err, &format) {
		c.formatFailed(u)
		return nil, fmt.Errorf("Steam's answer couldn't be read (%s). Try again later.", format.what)
	}
	return ids, err
}

func parseWishlist(b []byte) ([]int, error) {
	var a struct {
		Response *struct {
			Items *[]struct {
				AppID int `json:"appid"`
			} `json:"items"`
		} `json:"response"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, &errFormat{"the wishlist isn't JSON"}
	}
	if a.Response == nil {
		return nil, &errFormat{"the wishlist has no response"}
	}
	if a.Response.Items == nil {
		return nil, ErrWishlistPrivate
	}
	seen := map[int]bool{}
	out := []int{}
	for _, it := range *a.Response.Items {
		if it.AppID <= 0 || seen[it.AppID] {
			continue
		}
		seen[it.AppID] = true
		out = append(out, it.AppID)
		if len(out) == MaxWishlist {
			break
		}
	}
	return out, nil
}

// SteamAppName asks the store for a game's name; "" when Steam doesn't
// know the game.
func (c *Client) SteamAppName(ctx context.Context, appID int) (string, error) {
	if appID <= 0 {
		return "", nil
	}
	u := "https://store.steampowered.com/api/appdetails?filters=basic&appids=" + strconv.Itoa(appID)
	b, err := c.fetch(ctx, request{url: u})
	if errors.Is(err, errNotFound) {
		return "", nil
	}
	if err != nil {
		return "", steamError(err)
	}
	name, err := parseAppName(b, appID)
	if err != nil {
		c.formatFailed(u)
	}
	return name, err
}

func parseAppName(b []byte, appID int) (string, error) {
	var a appDetailsAnswer
	if err := json.Unmarshal(b, &a); err != nil || a == nil {
		return "", &errFormat{"the store data isn't JSON"}
	}
	r, ok := a[strconv.Itoa(appID)]
	if !ok {
		return "", &errFormat{"the store data has no entry for this game"}
	}
	if !r.Success || len(r.Data) == 0 || r.Data[0] != '{' {
		return "", nil
	}
	var d struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(r.Data, &d); err != nil {
		return "", &errFormat{"the store data isn't readable"}
	}
	return cleanText(d.Name, 200), nil
}

// steamError says in plain words why Steam couldn't answer.
func steamError(err error) error {
	var back *errBackoff
	var status *statusError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.As(err, &back):
		return fmt.Errorf("Steam failed a moment ago, so Seaglass waits until %s before asking again.", back.until.Local().Format("15:04"))
	case errors.Is(err, errNotFound):
		return errors.New("Steam answered HTTP 404. Try again later.")
	case errors.As(err, &status):
		return fmt.Errorf("Steam answered HTTP %d. Try again later.", status.Status)
	}
	return fmt.Errorf("Couldn't reach Steam: %v", err)
}
