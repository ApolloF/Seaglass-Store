package enrich

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSteamID64Validation(t *testing.T) {
	for s, want := range map[string]bool{
		"76561198000000042":  true,
		"76561197960265729":  true, // the first individual account
		"76561202255233023":  true, // the last one
		"76561202255233024":  false,
		"76561202000000000":  true,  // a newer account the old 7656119 prefix check refused
		"76561197960265728":  false, // account number 0
		"76561190000000000":  false, // below account number 1, which the old check allowed
		"7656119800000004":   false, // 16 digits
		"765611980000000420": false,
		"86561198000000042":  false,
		"7656119800000004a":  false,
		" 76561198000000042": false,
		"":                   false,
	} {
		if got := ValidSteamID64(s); got != want {
			t.Errorf("%q: %v, want %v", s, got, want)
		}
	}
}

func TestInvalidSteamIDIsRefusedBeforeAnyRequest(t *testing.T) {
	c, fn, _ := newTestClient(t, func(recorded) answer { return status(500) })
	if _, err := c.SteamWishlist(context.Background(), "12345"); !errors.Is(err, ErrSteamID) {
		t.Errorf("an invalid SteamID: %v", err)
	}
	if fn.count() != 0 {
		t.Errorf("%d requests for an invalid SteamID", fn.count())
	}
}

func TestSteamWishlistReadsAppIDsInSteamsOrder(t *testing.T) {
	c, fn, _ := newTestClient(t, func(recorded) answer { return ok(fixture(t, "wishlist.json")) })
	ids, err := c.SteamWishlist(context.Background(), "76561198000000042")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != 1245620 || ids[1] != 2210110 || ids[2] != 9990001 {
		t.Errorf("AppIDs: %v (no zero, no duplicate)", ids)
	}
	r := fn.last()
	if r.Host != hostSteamAPI || r.Path != "/IWishlistService/GetWishlist/v1/" || r.Query != "steamid=76561198000000042" {
		t.Errorf("request: %+v", r)
	}
}

func TestPrivateWishlistIsToldApartFromAnEmptyOneAndFromFailures(t *testing.T) {
	answers := map[string]answer{
		"private": ok(fixture(t, "wishlist_private.json")),
		"empty":   ok(fixture(t, "wishlist_empty.json")),
		"broken":  ok([]byte("<html>")),
		"server":  status(502),
		"limited": status(429, "Retry-After", "60"),
	}
	for name, a := range answers {
		c, _, _ := newTestClient(t, func(recorded) answer { return a })
		ids, err := c.SteamWishlist(context.Background(), "76561198000000042")
		switch name {
		case "private":
			if !errors.Is(err, ErrWishlistPrivate) || !strings.Contains(err.Error(), "Public") {
				t.Errorf("private: %v", err)
			}
		case "empty":
			if err != nil || len(ids) != 0 {
				t.Errorf("empty: %v %v", ids, err)
			}
		case "broken":
			if err == nil || errors.Is(err, ErrWishlistPrivate) || !strings.Contains(err.Error(), "couldn't be read") {
				t.Errorf("broken: %v", err)
			}
		case "server", "limited":
			if err == nil || errors.Is(err, ErrWishlistPrivate) || !strings.Contains(err.Error(), "HTTP") {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
	c, _, _ := newTestClient(t, func(recorded) answer { return answer{err: errors.New("no such host")} })
	if _, err := c.SteamWishlist(context.Background(), "76561198000000042"); err == nil || !strings.Contains(err.Error(), "Couldn't reach Steam") {
		t.Errorf("network failure: %v", err)
	}
}

func TestSteamWishlistIsCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"response":{"items":[`)
	for i := 1; i <= MaxWishlist+10; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		b.WriteString(`{"appid":` + strconv.Itoa(i) + `}`)
	}
	b.WriteString(`]}}`)
	c, _, _ := newTestClient(t, func(recorded) answer { return ok([]byte(b.String())) })
	ids, err := c.SteamWishlist(context.Background(), "76561198000000042")
	if err != nil || len(ids) != MaxWishlist {
		t.Errorf("%d AppIDs, %v", len(ids), err)
	}
}

func TestSteamAppNameComesFromTheStore(t *testing.T) {
	c, fn, _ := newTestClient(t, func(r recorded) answer {
		if strings.Contains(r.Query, "appids=620") {
			return ok(fixture(t, "appdetails_basic.json"))
		}
		return ok(fixture(t, "appdetails_unknown.json"))
	})
	if name, err := c.SteamAppName(context.Background(), 620); err != nil || name != "Portal 2" {
		t.Errorf("Portal 2: %q %v", name, err)
	}
	if !strings.Contains(fn.last().Query, "filters=basic") {
		t.Errorf("asked for more than the basics: %s", fn.last().Query)
	}
	if name, err := c.SteamAppName(context.Background(), 99999999); err != nil || name != "" {
		t.Errorf("an unknown game: %q %v", name, err)
	}
}

// TestLiveSteamWishlist reads a real public wishlist and names its first
// game:
//
//	WL_STORE_LIVE=1 WL_STEAM_ID=7656119... go test -run LiveSteamWishlist -v ./internal/store/enrich
func TestLiveSteamWishlist(t *testing.T) {
	id := os.Getenv("WL_STEAM_ID")
	if os.Getenv("WL_STORE_LIVE") == "" || id == "" {
		t.Skip("set WL_STORE_LIVE=1 and WL_STEAM_ID to read a real wishlist")
	}
	c := New(Options{Dir: t.TempDir()})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ids, err := c.SteamWishlist(ctx, id)
	t.Logf("wishlist: %d games, %v", len(ids), err)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) > 0 {
		name, err := c.SteamAppName(ctx, ids[0])
		t.Logf("first game: %d %q %v", ids[0], name, err)
		if err != nil {
			t.Error(err)
		}
	}
}
