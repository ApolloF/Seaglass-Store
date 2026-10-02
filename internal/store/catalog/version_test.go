package catalog

import "testing"

func TestReleaseComparisonRequiresCompatibleClaims(t *testing.T) {
	for _, test := range []struct {
		a, b  string
		order int
		known bool
	}{
		{"v1.10 + 12 DLCs", "v1.9 + 99 DLCs", 1, true},
		{"v01.2.0", "1.2", 0, true},
		{"Build 15302", "Build 9876", 1, true},
		{"v1.2 build 300", "v1.2 build 299", 1, true},
		{"v1.3 build 301", "v1.2 build 300", 1, true},
		{"v1.3 build 299", "v1.2 build 300", 0, false},
		{"v1.2 build 300", "v1.2", 0, false},
		{"v1.31/v1.32", "v1.30", 0, false},
		{"Build 15302", "v1.2", 0, false},
		{"2026-10-02", "v1.2", 0, false},
		{"MULTi12 Repack 2", "Repack 1", 0, false},
		{"v1.2 beta 2", "v1.2", 0, false},
		{"", "v1.2", 0, false},
		{"2026-02-30", "2026-02-28", 0, false},
		{"v1.0.999999999999999999999999999999999", "v1.0.999999999999999999999999999999998", 1, true},
	} {
		order, known := CompareReleases(test.a, test.b)
		if order != test.order || known != test.known {
			t.Errorf("%q vs %q = %d,%v", test.a, test.b, order, known)
		}
		reverse, reverseKnown := CompareReleases(test.b, test.a)
		if reverse != -test.order || reverseKnown != test.known {
			t.Errorf("reverse %q vs %q = %d,%v", test.b, test.a, reverse, reverseKnown)
		}
	}
}

func TestUnknownReleaseDoesNotClaimToBeNewest(t *testing.T) {
	e := Entry{Offers: []Offer{offer("https://a", "A", "Build 99999", 10), offer("https://b", "B", "v1.2", 20)}}
	if NewestOffer(e.Offers) != -1 {
		t.Fatal("mixed version systems were ranked")
	}
	r := Recommend(e, Prefs{})
	for _, why := range r.Why {
		if why == "The newest version" {
			t.Fatal("unknown comparison advertised as newest")
		}
	}
}
