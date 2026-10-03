package app

import (
	"testing"

	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// Attribution links open only Steam, Metacritic and HowLongToBeat pages.
func TestOpenStoreLinkRefusesOtherSites(t *testing.T) {
	s := NewStoreService(testStoreCore(t))
	for _, raw := range []string{"http://store.steampowered.com/app/1/", "https://evil.example/", "https://store.steampowered.com.evil.example/", "https://user@howlongtobeat.com/game/1", "https://howlongtobeat.com:8443/", "file:///C:/Windows", "javascript:alert(1)"} {
		if err := s.OpenStoreLink(raw); err == nil {
			t.Errorf("%s was opened", raw)
		}
	}
}

// The status follows the settings: an existing Store user is asked first,
// and chosen sources are enabled.
func TestDiscoveryStatusFollowsSourceChoice(t *testing.T) {
	c := testStoreCore(t)
	s := NewStoreService(c)
	st := s.DiscoveryStatus()
	if !st.Enabled || st.SetupNeeded || len(st.Sources) != len(sources.Providers()) || !st.Sources[0].Enabled {
		t.Fatalf("new Store user: %+v", st)
	}
	// Providers that are not on by default stay off for an existing user.
	for _, src := range st.Sources {
		if src.Enabled != src.DefaultOn {
			t.Errorf("%s enabled=%v, want its default %v", src.ID, src.Enabled, src.DefaultOn)
		}
	}
	if _, err := s.SetupSources([]string{"dodi"}); err != nil {
		t.Fatal(err)
	}
	st = s.DiscoveryStatus()
	if st.Sources[0].Enabled || !st.Sources[1].Enabled {
		t.Errorf("only DODI was chosen: %+v", st.Sources)
	}
	if _, err := s.SetupSources([]string{"1337x"}); err == nil {
		t.Error("an unknown source was accepted")
	}
	if v, _ := s.SetupSources(nil); v.Store.PrivateSources || s.DiscoveryStatus().Enabled {
		t.Error("choosing no source left discovery on")
	}
}
