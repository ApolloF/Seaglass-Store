package app

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
	sources "github.com/ApolloF/Seaglass/internal/store/sources"
)

func TestPrivateSourceIntegrationRequiresOptInAndReview(t *testing.T) {
	c := testStoreCore(t)
	svc := NewStoreService(c)
	// Turning the Store on chose the sources; the person can turn them off.
	if _, err := c.updateSettings(func(v *settings.Settings) { v.Store.PrivateSources = false }); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DiscoverReleases("fitgirl", "test", false); !errors.Is(err, sources.ErrDisabled) {
		t.Fatalf("disabled source: %v", err)
	}
	entry := sources.Entry{ID: "release-1", SourceID: "fitgirl", Title: "Tiny Game", Version: "v1.2", ReleaseKind: "release", NeedsReview: true, PageURL: "https://fitgirl-repacks.site/tiny-game/"}
	transport, err := sources.Magnet("magnet:?xt=urn:btih:" + strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	entry.Transports = []sources.Transport{transport}
	c.catalog.previews = map[string]sources.Snapshot{"fitgirl": {Entries: []sources.Entry{entry}}}
	if _, err := svc.ReviewRelease("fitgirl", entry.ID, 0); !errors.Is(err, sources.ErrDisabled) {
		t.Fatalf("review bypassed source switch: %v", err)
	}
	v := c.Settings.Get()
	v.Store.PrivateSources = true
	if _, err := c.Settings.Set(v); err != nil {
		t.Fatal(err)
	}
	key, err := svc.ReviewRelease("fitgirl", entry.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	e, err := svc.CatalogEntry(key)
	if err != nil || len(e.Offers) != 1 || e.Version != "v1.2" {
		t.Fatalf("reviewed offer: %+v %v", e, err)
	}
	if _, err := svc.ReviewRelease("fitgirl", entry.ID, 0); err != nil {
		t.Fatal(err)
	}
	if c.catalog.len() != 1 {
		t.Fatal("review created duplicate game")
	}
	v = c.Settings.Get()
	v.Store.PrivateSources = false
	c.Settings.Set(v)
	c.catalog.rebuild()
	if c.catalog.len() != 0 {
		t.Fatal("reviewed private games visible with sources disabled")
	}
	v.Store.PrivateSources = true
	c.Settings.Set(v)
	c.catalog.rebuild()
	if c.catalog.len() != 1 {
		t.Fatal("reviewed offer not restored from atomic cache")
	}
}

func TestIncompleteAndUpdateOnlySourceEntriesCannotInstall(t *testing.T) {
	transport, _ := sources.Magnet("magnet:?xt=urn:btih:" + strings.Repeat("a", 40))
	for _, entry := range []sources.Entry{
		{Title: "Game update", ReleaseKind: "update", Transports: []sources.Transport{transport}},
		{Title: "Game", ReleaseKind: "release", SummaryOnly: true, Transports: []sources.Transport{transport}},
		{Title: "Game", ReleaseKind: "release"},
	} {
		if _, err := sourceOffer(entry, 0); err == nil {
			t.Fatalf("unsafe standalone offer: %+v", entry)
		}
	}
}

func TestMixedReleaseVersionsDoNotOfferAnUnconfirmedUpdate(t *testing.T) {
	c := testStoreCore(t)
	old, err := c.store.jobs.Add(jobs.Job{Title: "Game", GameKey: "title:game", Version: "v1.2", InstallDir: filepath.Join(t.TempDir(), "Game")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c.store.jobs.Update(old.ID, func(j *jobs.Job) bool { j.State = jobs.Installed; return true })
	const magnet = "magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	c.catalog.entries = catalog.Build([]catalog.Source{{Name: "A", Feed: feed.Feed{Items: []feed.Item{{Title: "Game", Version: "Build 99999", Magnet: magnet}, {Title: "Game", Version: "v1.2 + 99 DLCs", Magnet: magnet}}}}}, nil)
	if len(c.catalog.updates()) != 0 {
		t.Fatal("repack metadata generated an update")
	}
	svc := NewStoreService(c)
	if _, err := svc.DownloadOffer("title:game", 0, InstallOptions{Update: true}); err == nil {
		t.Fatal("incompatible release overwrote installed game")
	}
	c.catalog.entries = catalog.Build([]catalog.Source{{Name: "A", Feed: feed.Feed{Items: []feed.Item{{Title: "Game", Version: "Build 99999", Magnet: magnet}, {Title: "Game", Version: "v1.3", Magnet: magnet}}}}}, nil)
	ups := c.catalog.updates()
	if len(ups) != 1 || ups[0].Offers[ups[0].Recommended.Offer].Version != "v1.3" {
		t.Fatalf("update recommendation should be the compatible version: %+v", ups)
	}
}

func TestEnglishAndScanningDefaultsSurviveOldSettings(t *testing.T) {
	c := testNewStoreUserCore(t)
	v := c.Settings.Get()
	// A new Store user starts with no source and source browsing off.
	if v.Store.Language != "English" || v.Store.DisablePayloadScanning || v.Store.PrivateSources || len(v.Store.Sources) != 0 {
		t.Fatalf("defaults: %+v", v.Store)
	}
	if _, err := c.updateSettings(func(v *settings.Settings) { v.Store.DisablePayloadScanning = true }); err != nil {
		t.Fatal(err)
	}
	if !c.Settings.Get().Store.DisablePayloadScanning {
		t.Fatal("scan choice not persisted")
	}
}
