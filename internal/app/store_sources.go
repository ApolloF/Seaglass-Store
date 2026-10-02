package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	sources "github.com/ApolloF/Seaglass/internal/store/sources"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func (s *StoreService) privateOn() error {
	if err := s.on(); err != nil {
		return err
	}
	if !s.c.Settings.Get().Store.PrivateSources {
		return sources.ErrDisabled
	}
	return nil
}

// DiscoverReleases fetches metadata on demand; it never queues payload downloads.
func (s *StoreService) DiscoverReleases(source, query string, resolve bool) (sources.Snapshot, error) {
	if err := s.privateOn(); err != nil {
		return sources.Snapshot{}, err
	}
	if source != "fitgirl" && source != "dodi" {
		return sources.Snapshot{}, errors.New("choose FitGirl or DODI")
	}
	src, err := sources.PrivateSource(source, true)
	if err != nil {
		return sources.Snapshot{}, err
	}
	start := src.StartURL
	if strings.TrimSpace(query) != "" {
		start, err = src.SearchURL(query)
		if err != nil {
			return sources.Snapshot{}, err
		}
	}
	cs := s.c.catalog
	cs.fetchMu.Lock()
	defer cs.fetchMu.Unlock()
	if err := s.privateOn(); err != nil {
		return sources.Snapshot{}, err
	}
	client, err := sources.NewClient(source, true)
	if err != nil {
		return sources.Snapshot{}, err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(s.c.ctx, 3*time.Minute)
	defer cancel()
	budget := 0
	if resolve {
		budget = 1
	}
	snapshot, err := sources.Collect(ctx, client, start, sources.CollectOptions{Pages: 1, Details: 5, Resolve: budget, Query: query})
	if err != nil {
		return snapshot, err
	}
	if err := s.privateOn(); err != nil {
		return sources.Snapshot{}, err
	}
	cs.mu.Lock()
	if cs.previews == nil {
		cs.previews = map[string]sources.Snapshot{}
	}
	cs.previews[source] = snapshot
	cs.mu.Unlock()
	return snapshot, nil
}

func (cs *catalogState) preview(source, id string) (sources.Entry, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, entry := range cs.previews[source].Entries {
		if entry.ID == id {
			return entry, nil
		}
	}
	return sources.Entry{}, errors.New("search again: this release preview is no longer available")
}

// AttachReleaseTorrent validates user-selected metadata for an unresolved release.
func (s *StoreService) AttachReleaseTorrent(source, id string) (sources.Entry, error) {
	if err := s.privateOn(); err != nil {
		return sources.Entry{}, err
	}
	entry, err := s.c.catalog.preview(source, id)
	if err != nil {
		return entry, err
	}
	path, err := application.Get().Dialog.OpenFile().SetTitle("Choose torrent metadata for "+entry.Title).
		CanChooseFiles(true).CanChooseDirectories(false).AddFilter("Torrent metadata", "*.torrent").PromptForSingleSelection()
	if err != nil || path == "" {
		return entry, err
	}
	f, err := os.Open(path)
	if err != nil {
		return entry, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	closeErr := f.Close()
	if readErr != nil {
		return entry, readErr
	}
	if closeErr != nil {
		return entry, closeErr
	}
	transport, err := sources.TorrentMetadata(data)
	if err != nil {
		return entry, err
	}
	if err := s.privateOn(); err != nil {
		return entry, err
	}
	entry.Transports = append(entry.Transports, transport)
	entry.Warnings = append(entry.Warnings, "Manual torrent attached: confirm that its name matches the selected game.")
	cs := s.c.catalog
	cs.mu.Lock()
	defer cs.mu.Unlock()
	snapshot := cs.previews[source]
	i := slices.IndexFunc(snapshot.Entries, func(e sources.Entry) bool { return e.ID == id })
	if i < 0 {
		return entry, errors.New("the preview changed; search again")
	}
	snapshot.Entries = slices.Clone(snapshot.Entries)
	snapshot.Entries[i] = entry
	cs.previews[source] = snapshot
	return entry, nil
}

func sourceOffer(entry sources.Entry, transport int) (feed.Item, error) {
	if entry.ReleaseKind != "release" || entry.SummaryOnly {
		return feed.Item{}, errors.New("update-only and incomplete releases need manual review outside standalone installs")
	}
	if transport < 0 || transport >= len(entry.Transports) {
		return feed.Item{}, errors.New("choose a resolved torrent identity")
	}
	t, err := sources.Magnet(entry.Transports[transport].URI)
	if err != nil {
		return feed.Item{}, err
	}
	if t.InfoHash != entry.Transports[transport].InfoHash {
		return feed.Item{}, errors.New("torrent identity changed")
	}
	size := entry.Transports[transport].SizeBytes
	if size == 0 && !entry.SizeIsMinimum && len(entry.SizeOptionsBytes) == 0 {
		size = entry.SizeBytes
	}
	it := feed.Item{Title: entry.Title, Version: entry.Version, Magnet: t.URI, SizeBytes: size,
		InstalledSizeBytes: entry.InstalledSizeBytes, Notes: entry.PageURL + "\nDocument SHA-256: " + entry.DocumentSHA256 + "\n" + entry.LanguageClaim + "\n" + strings.Join(entry.Warnings, "\n")}
	data, err := json.Marshal(feed.Feed{Schema: feed.Schema, Name: "Reviewed source", Items: []feed.Item{it}})
	if err != nil {
		return it, err
	}
	f, skipped, err := feed.Parse(data)
	if err != nil {
		return it, err
	}
	if len(f.Items) != 1 {
		return it, fmt.Errorf("release is not downloadable: %s", strings.Join(skipped, "; "))
	}
	return f.Items[0], nil
}

// ReviewRelease adds only a resolved, explicitly reviewed offer to the catalog.
func (s *StoreService) ReviewRelease(source, id string, transport int) (string, error) {
	if err := s.privateOn(); err != nil {
		return "", err
	}
	cs := s.c.catalog
	cs.fetchMu.Lock()
	defer cs.fetchMu.Unlock()
	entry, err := cs.preview(source, id)
	if err != nil {
		return "", err
	}
	it, err := sourceOffer(entry, transport)
	if err != nil {
		return "", err
	}
	src, err := sources.PrivateSource(source, true)
	if err != nil {
		return "", err
	}
	f, err := cs.sourceFeed(source)
	if err != nil {
		return "", err
	}
	f.Schema, f.Name = feed.Schema, src.Name
	if !slices.ContainsFunc(f.Items, func(old feed.Item) bool { return old.Magnet == it.Magnet }) {
		f.Items = append(f.Items, it)
	}
	if len(f.Items) > 5000 {
		return "", errors.New("reviewed source cache is full")
	}
	data, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	path := cs.sourcePath(source)
	if err := os.MkdirAll(cs.cache.Dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return "", err
	}
	cs.rebuild()
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, game := range cs.entries {
		for _, offer := range game.Offers {
			if offer.Magnet == it.Magnet {
				return game.Key, nil
			}
		}
	}
	return "title:" + scan.Normalize(it.Title), nil
}

func (cs *catalogState) sourcePath(source string) string {
	return filepath.Join(cs.cache.Dir, "reviewed-"+feed.ID(source)+".json")
}

func (cs *catalogState) sourceFeed(source string) (feed.Feed, error) {
	data, err := os.ReadFile(cs.sourcePath(source))
	if errors.Is(err, os.ErrNotExist) {
		return feed.Feed{}, nil
	}
	if err != nil {
		return feed.Feed{}, err
	}
	f, skipped, err := feed.Parse(data)
	if len(skipped) > 0 {
		return feed.Feed{}, fmt.Errorf("reviewed source cache has invalid offers: %s", strings.Join(skipped, "; "))
	}
	if err != nil {
		return feed.Feed{}, err
	}
	return f, nil
}

func (cs *catalogState) reviewedSources() []catalog.Source {
	if !cs.c.Settings.Get().Store.PrivateSources {
		return nil
	}
	var out []catalog.Source
	for _, id := range []string{"fitgirl", "dodi"} {
		src, _ := sources.PrivateSource(id, true)
		f, err := cs.sourceFeed(id)
		if err != nil {
			logx.Printf("store: reviewed source %s: %v", id, err)
			continue
		}
		if len(f.Items) > 0 {
			out = append(out, catalog.Source{URL: src.StartURL, Name: src.Name, Feed: f})
		}
	}
	return out
}

// OpenReleasePage hands an unresolved release to the person's browser.
func (s *StoreService) OpenReleasePage(source, id string) error {
	if err := s.privateOn(); err != nil {
		return err
	}
	entry, err := s.c.catalog.preview(source, id)
	if err != nil {
		return err
	}
	src, err := sources.PrivateSource(source, true)
	if err != nil {
		return err
	}
	if _, err := src.ValidateURL(entry.PageURL); err != nil {
		return err
	}
	return platform.OpenWebPage(entry.PageURL)
}
