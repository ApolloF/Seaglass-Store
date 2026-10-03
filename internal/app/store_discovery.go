package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ApolloF/Seaglass/internal/logx"
	"github.com/ApolloF/Seaglass/internal/platform"
	"github.com/ApolloF/Seaglass/internal/scan"
	"github.com/ApolloF/Seaglass/internal/settings"
	"github.com/ApolloF/Seaglass/internal/store/catalog"
	"github.com/ApolloF/Seaglass/internal/store/discovery"
	"github.com/ApolloF/Seaglass/internal/store/feed"
	"github.com/ApolloF/Seaglass/internal/store/jobs"
	"github.com/ApolloF/Seaglass/internal/store/sources"
)

// discoveryState indexes the chosen repack sources in the background and
// answers the Store's queries from the index. It reads public release
// metadata only: no torrent peer, payload download or installer.
type discoveryState struct {
	c    *Core
	auto bool // background passes run (not in the test harness's frozen mode)

	openOnce sync.Once
	ix       *discovery.Index
	search   discovery.SearchCache

	kick chan struct{}

	mu          sync.Mutex
	fetchers    map[string]discovery.Fetcher // per source; one request in flight each
	passMu      map[string]*sync.Mutex       // one pass per source at a time
	active      map[string]string            // source → crawl state while a pass runs
	force       map[string]bool              // a pass that ignores the age of the newest listings
	runCtx      context.Context              // cancelled when discovery is turned off
	runCancel   context.CancelFunc
	view        *discovery.View
	viewKey     string         // index version, catalog version and sources the view was built from
	steamNames  map[int]string // Steam-only search results, for their pages and art
	searchStop  context.CancelFunc
	searchSeq   int
	detailQueue []detailRequest
	detailWake  chan struct{}
	queuedIDs   map[string]bool
}

type detailRequest struct{ src, id, key string }

func newDiscoveryState(c *Core) *discoveryState {
	d := &discoveryState{c: c, auto: true, kick: make(chan struct{}, 1), detailWake: make(chan struct{}, 1),
		fetchers: map[string]discovery.Fetcher{}, passMu: map[string]*sync.Mutex{}, active: map[string]string{}, force: map[string]bool{},
		steamNames: map[int]string{}, queuedIDs: map[string]bool{}}
	for _, src := range discovery.Sources {
		d.passMu[src] = &sync.Mutex{}
	}
	return d
}

// index opens the persisted index on first use: at startup the cached
// games are there at once, before any network request.
func (d *discoveryState) index() *discovery.Index {
	d.openOnce.Do(func() {
		if d.ix == nil {
			d.ix = discovery.OpenIndex(platform.CacheDir("store", "discovery"), filepath.Join(platform.AppDir(), "store-identity.json"))
		}
	})
	return d.ix
}

func (d *discoveryState) fetcher(src string) (discovery.Fetcher, sources.Provider, error) {
	s, ok := sources.Lookup(src)
	if !ok || !s.Discovery {
		return nil, s, fmt.Errorf("unknown source %q", src)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if f := d.fetchers[src]; f != nil {
		return f, s, nil
	}
	client, err := sources.NewClient(src, true)
	if err != nil {
		return nil, s, err
	}
	d.fetchers[src] = client
	return client, s, nil
}

// ctx is the context passes and searches run in. It's cancelled when the
// Store or source browsing is turned off, and with Seaglass.
func (d *discoveryState) ctx() context.Context {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.runCtx == nil || d.runCtx.Err() != nil {
		d.runCtx, d.runCancel = context.WithCancel(d.c.ctx)
	}
	return d.runCtx
}

func (d *discoveryState) wake() {
	select {
	case d.kick <- struct{}{}:
	default:
	}
}

// enabled lists the sources discovery may use now.
func (d *discoveryState) enabled() []string {
	v := d.c.Settings.Get()
	var out []string
	if !v.ExperimentalStore {
		return out
	}
	for _, src := range discovery.Sources {
		if v.Store.DiscoveryOn(src) {
			out = append(out, src)
		}
	}
	return out
}

func (d *discoveryState) playing() bool { return d.c.Launch != nil && d.c.Launch.Active() }

// loop runs background passes: the newest listings when six hours old,
// older pages every 30 minutes until a source's end, never while a game
// runs.
func (d *discoveryState) loop(ctx context.Context) {
	d.index()
	go d.detailLoop(ctx)
	if d.c.wishlist != nil && d.c.Settings.Get().ExperimentalStore {
		d.c.wishlist.observe(d)
	}
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		d.tick()
		select {
		case <-ctx.Done():
			d.save()
			return
		case <-t.C:
		case <-d.kick:
		}
	}
}

func (d *discoveryState) tick() {
	srcs := d.enabled()
	if len(srcs) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, src := range srcs {
		d.mu.Lock()
		force := d.force[src]
		d.mu.Unlock()
		if !d.auto && !force {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := d.pass(src, force, false); err != nil {
				logx.Printf("store discovery: %s: %v", src, err)
			}
		}()
	}
	wg.Wait()
}

// pass runs one pass for a source unless one is running (wait: a manual
// refresh waits for it instead of skipping).
func (d *discoveryState) pass(src string, force, wait bool) (discovery.PassResult, error) {
	m := d.passMu[src]
	if wait {
		m.Lock()
	} else if !m.TryLock() {
		return discovery.PassResult{}, nil
	}
	defer m.Unlock()
	ix := d.index()
	recent, backfill := discovery.Due(ix.Crawl(src), time.Now(), force)
	if !recent && !backfill {
		d.mu.Lock()
		delete(d.force, src)
		d.mu.Unlock()
		return discovery.PassResult{}, nil
	}
	if d.playing() {
		d.emitStatus()
		return discovery.PassResult{Stopped: discovery.StopPaused}, nil
	}
	f, s, err := d.fetcher(src)
	if err != nil {
		return discovery.PassResult{}, err
	}
	d.setActive(src, map[bool]string{true: discovery.CrawlRecent, false: discovery.CrawlBackfill}[recent])
	res := discovery.Pass(d.ctx(), ix, s, f, force, time.Now, d.playing)
	d.setActive(src, "")
	if res.Stopped != discovery.StopPaused {
		d.mu.Lock()
		delete(d.force, src)
		d.mu.Unlock()
	}
	if res.Merged.Added > 0 || res.Merged.Changed > 0 {
		logx.Printf("store discovery: %s: %d new, %d changed releases (%d pages)", src, res.Merged.Added, res.Merged.Changed, res.Pages)
		d.indexChanged(res.Merged.IDs, src)
	}
	d.save()
	d.emitStatus()
	return res, res.Err
}

func (d *discoveryState) setActive(src, state string) {
	d.mu.Lock()
	if state == "" {
		delete(d.active, src)
	} else {
		d.active[src] = state
	}
	d.mu.Unlock()
	d.emitStatus()
}

// indexChanged tells the interface, and the wishlist, about new and
// changed releases.
func (d *discoveryState) indexChanged(ids []string, src string) {
	d.c.emit(EventStoreGames, discovery.Change{Keys: []string{}, All: true})
	if d.c.wishlist != nil {
		d.c.wishlist.observe(d)
	}
}

func (d *discoveryState) save() {
	if d.ix == nil {
		return
	}
	if err := d.ix.Save(); err != nil {
		logx.Printf("store discovery: saving the index: %v", err)
	}
}

// refresh fetches the newest listings of every chosen source now, waiting
// for a pass already running. Sources in backoff are skipped.
func (d *discoveryState) refresh() error {
	srcs := d.enabled()
	if len(srcs) == 0 {
		return sources.ErrDisabled
	}
	var mu sync.Mutex
	var errs []string
	var wg sync.WaitGroup
	for _, src := range srcs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := d.index().Crawl(src)
			if time.Now().Before(c.RetryAt) {
				name := d.sourceName(src)
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s asked to wait until %s", name, c.RetryAt.Local().Format("15:04")))
				mu.Unlock()
				return
			}
			if _, err := d.pass(src, true, true); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("couldn't reach %s: %v", d.sourceName(src), err))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; ") + ". Showing what was indexed before.")
	}
	return nil
}

// start makes the next tick a forced pass for every chosen source: the
// first discovery after setup starts at once.
func (d *discoveryState) start() {
	d.mu.Lock()
	for _, src := range discovery.Sources {
		d.force[src] = true
	}
	d.mu.Unlock()
	d.wake()
}

// settingsChanged cancels work for what was turned off and starts
// indexing what was turned on.
func (d *discoveryState) settingsChanged(old, saved settings.Settings) {
	if d == nil {
		return
	}
	wasOn := func(v settings.Settings, src string) bool { return v.ExperimentalStore && v.Store.DiscoveryOn(src) }
	turnedOff, turnedOn := false, false
	for _, src := range discovery.Sources {
		a, b := wasOn(old, src), wasOn(saved, src)
		turnedOff = turnedOff || (a && !b)
		turnedOn = turnedOn || (!a && b)
	}
	if turnedOff {
		d.mu.Lock()
		if d.runCancel != nil {
			d.runCancel()
		}
		if d.searchStop != nil {
			d.searchStop()
		}
		d.detailQueue, d.queuedIDs = nil, map[string]bool{}
		d.mu.Unlock()
	}
	if turnedOn {
		d.start()
	}
	if turnedOn || turnedOff {
		d.c.emit(EventStoreGames, discovery.Change{Keys: []string{}, All: true})
	}
	d.emitStatus()
}

func (d *discoveryState) sourceName(src string) string {
	s, err := sources.PrivateSource(src, true)
	if err != nil {
		return src
	}
	return s.Name
}

// status is what discovery is doing now.
func (d *discoveryState) status() discovery.Status {
	v := d.c.Settings.Get()
	st := baseDiscoveryStatus(v)
	ix := d.index()
	counts := ix.Counts()
	now := time.Now()
	playing := d.playing()
	d.mu.Lock()
	active := map[string]string{}
	for k, a := range d.active {
		active[k] = a
	}
	d.mu.Unlock()
	for i := range st.Sources {
		s := &st.Sources[i]
		c := ix.Crawl(s.ID)
		s.Releases = counts[s.ID]
		s.BackfillDone = c.BackfillDone
		if c.NextPage > 0 {
			s.BackfillPage = c.NextPage
		}
		if !c.RecentAt.IsZero() {
			s.RecentAt = c.RecentAt.Unix()
		}
		if now.Before(c.RetryAt) {
			s.RetryAt = c.RetryAt.Unix()
		}
		s.Error = c.Error
		if !s.Enabled {
			continue
		}
		switch {
		case active[s.ID] != "":
			s.State = active[s.ID]
			st.Refreshing = true
		case now.Before(c.RetryAt):
			s.State = discovery.CrawlBackoff
		case playing:
			s.State = discovery.CrawlPaused
		}
		if c.RecentAt.IsZero() || now.Sub(c.RecentAt) > discovery.RecentEvery || c.Error != "" {
			st.Stale = true
		}
		st.Releases += s.Releases
	}
	if st.Enabled {
		for _, g := range d.currentView().Games {
			if len(g.Records) > 0 {
				st.Games++
			}
		}
	}
	return st
}

func (d *discoveryState) emitStatus() { d.c.emit(EventStoreDiscovery, d.status()) }

// currentView is the grouped index for the chosen sources and the
// person's feeds, rebuilt when either changed.
func (d *discoveryState) currentView() *discovery.View {
	ix := d.index()
	srcs := d.enabled()
	feeds, feedVersion := d.c.catalog.snapshot()
	key := fmt.Sprintf("%d/%d/%s", ix.Version(), feedVersion, strings.Join(srcs, ","))
	d.mu.Lock()
	if d.view != nil && d.viewKey == key {
		v := d.view
		d.mu.Unlock()
		return v
	}
	d.mu.Unlock()
	if !d.c.Settings.Get().ExperimentalStore {
		feeds = nil
	}
	v := discovery.NewView(discovery.Group(ix.Records(srcs), feeds, ix.SteamCorrections(), d.identify()))
	d.mu.Lock()
	d.view, d.viewKey = v, key
	d.mu.Unlock()
	return v
}

// Edition words that name a different Steam game: a remaster, a
// definitive edition or a director's cut is sold separately.
var separateEdition = regexp.MustCompile(`(?i)remaster|remake|definitive|director|final cut|enhanced|anniversary|reloaded|redux`)

// identify trusts Seaglass's game database only for exact titles, or for
// titles that differ by an edition sold as the same game ("Deluxe
// Edition"): looser matches could merge a sequel or a remaster.
func (d *discoveryState) identify() discovery.Identify {
	ix := d.c.Manifest.Index()
	return func(title string) (string, int) {
		m := ix.Identify(scan.Candidate{Title: title, TitleTrusted: true, Source: scan.Installer, AppIDFrom: "the release"})
		if m.SteamAppID == 0 {
			return "", 0
		}
		switch {
		case m.Confidence >= 85:
			return m.Title, m.SteamAppID
		case m.Confidence >= 75 && !separateEdition.MatchString(strings.TrimPrefix(title, scan.StripEdition(title))):
			return m.Title, m.SteamAppID
		}
		return "", 0
	}
}

// annotator adds what this PC knows to summaries: the chart, review
// scores and genres already fetched, what the Store installed, and the
// wishlist.
func (d *discoveryState) annotator() discovery.Annotate {
	installed := map[string]catalog.Installed{}
	inDownloads := map[string]bool{}
	for _, j := range d.c.store.jobs.All() {
		if j.GameKey == "" {
			continue
		}
		if j.State == jobs.Installed {
			if old, ok := installed[j.GameKey]; !ok || newerInstalled(j.Version, old.Version) {
				installed[j.GameKey] = catalog.Installed{Download: j.ID, Version: j.Version, Dir: j.InstallDir}
			}
		} else if j.State != jobs.Failed {
			inDownloads[j.GameKey] = true
		}
	}
	view := d.currentView()
	enrichNote := d.c.enrichAnnotator()
	wish := d.c.wishlistAnnotator()
	return func(s *discovery.GameSummary) {
		if in, ok := installed[s.Key]; ok {
			s.Installed = &discovery.Installed{Download: in.Download, Version: in.Version}
			if g, ok := view.Game(s.Key); ok {
				for _, r := range g.Releases(d.sourceName) {
					if r.Availability == discovery.AvailInstallable && r.Kind == "release" && newerInstalled(r.Version, in.Version) {
						s.Installed.Update = true
					}
				}
			}
		}
		if g := d.c.art.genres(s.Key); len(g) > 0 {
			s.Genres = g
		}
		if enrichNote != nil {
			enrichNote(s)
		}
		if wish != nil {
			wish(s)
		}
	}
}

// details builds a game's page and queues missing release details.
func (d *discoveryState) details(key string) (discovery.GameDetails, error) {
	g, ok := d.currentView().Game(key)
	if !ok {
		// A Steam game without a source release still has a page: its
		// reviews, times and the wishlist, but nothing to install.
		if title, appID, found := d.gameRef(key); found && appID > 0 {
			return discovery.GameDetails{Summary: d.steamOnly(appID, title), Releases: []discovery.Release{}, Recommended: -1, Why: []string{},
				Identity: discovery.Identity{SteamAppID: appID, Name: title}}, nil
		}
		return discovery.GameDetails{}, errNotIndexed
	}
	s := g.Summary()
	d.annotator()(&s)
	rels := g.Releases(d.sourceName)
	out := discovery.GameDetails{Summary: s, Releases: rels, Recommended: -1, Why: []string{},
		Identity: discovery.Identity{SteamAppID: g.AppID, How: g.How, Corrected: g.Corrected}}
	if g.AppID > 0 {
		out.Identity.Name = g.Title
	}
	for _, r := range g.Records {
		if r.Entry.SummaryOnly && !r.Gone && r.FetchError == "" {
			out.Loading = true
			d.queueDetail(r.Entry.SourceID, r.Entry.ID, key, true)
		}
	}
	if s.Installed != nil {
		for i := range out.Releases {
			out.Releases[i].Newer = out.Releases[i].Availability == discovery.AvailInstallable && newerInstalled(out.Releases[i].Version, s.Installed.Version)
		}
	}
	out.Recommended, out.Why = d.recommend(out.Releases, s.Installed)
	return out, nil
}

// recommend picks the release to offer first with the existing
// comparable-version recommendation, among installable releases (newer
// ones only for an installed game).
func (d *discoveryState) recommend(rels []discovery.Release, in *discovery.Installed) (int, []string) {
	var offers []catalog.Offer
	var idx []int
	for i, r := range rels {
		if r.Availability != discovery.AvailInstallable || r.Kind != "release" || (in != nil && !r.Newer) {
			continue
		}
		offers = append(offers, catalog.Offer{Item: offerItem(r), FeedURL: r.Source, FeedName: r.SourceName})
		idx = append(idx, i)
	}
	if len(offers) == 0 {
		return -1, []string{}
	}
	cfg := d.c.Settings.Get().Store
	p := catalog.Prefs{Language: cfg.Language, Trust: map[string]int{}, Blocked: map[string]int{}}
	for _, f := range cfg.Feeds {
		p.Trust[f.URL] = f.Trust
	}
	rec := catalog.Recommend(catalog.Entry{Offers: offers}, p)
	if rec.Offer < 0 {
		return -1, []string{}
	}
	return idx[rec.Offer], rec.Why
}

func offerItem(r discovery.Release) feed.Item {
	return feed.Item{Title: r.Title, Version: r.Version, SizeBytes: r.SizeBytes, Languages: r.Languages}
}

// queueDetail asks for a release's article in the background; opened
// games go first.
func (d *discoveryState) queueDetail(src, id, key string, first bool) {
	d.mu.Lock()
	if d.queuedIDs[id] {
		d.mu.Unlock()
		return
	}
	d.queuedIDs[id] = true
	req := detailRequest{src, id, key}
	if first {
		d.detailQueue = append([]detailRequest{req}, d.detailQueue...)
	} else {
		d.detailQueue = append(d.detailQueue, req)
	}
	d.mu.Unlock()
	select {
	case d.detailWake <- struct{}{}:
	default:
	}
}

func (d *discoveryState) detailLoop(ctx context.Context) {
	for {
		d.mu.Lock()
		var req *detailRequest
		if len(d.detailQueue) > 0 {
			r := d.detailQueue[0]
			d.detailQueue = d.detailQueue[1:]
			req = &r
		}
		d.mu.Unlock()
		if req == nil {
			select {
			case <-ctx.Done():
				return
			case <-d.detailWake:
			}
			continue
		}
		if slices.Contains(d.enabled(), req.src) {
			if f, s, err := d.fetcher(req.src); err == nil {
				err := discovery.FetchDetail(d.ctx(), d.index(), s.Source, f, req.id, time.Now())
				if err != nil && !errors.Is(err, context.Canceled) {
					logx.Printf("store discovery: release details: %v", err)
				}
				d.save()
				d.c.emit(EventStoreGames, discovery.Change{Keys: []string{req.key}})
				if d.c.wishlist != nil {
					d.c.wishlist.observe(d)
				}
			}
		}
		d.mu.Lock()
		delete(d.queuedIDs, req.id)
		d.mu.Unlock()
	}
}

// release finds a game's source release.
func (d *discoveryState) release(key, id string) (*discovery.Game, discovery.Record, error) {
	g, ok := d.currentView().Game(key)
	if !ok {
		return nil, discovery.Record{}, errNotIndexed
	}
	for _, r := range g.Records {
		if r.Entry.ID == id {
			return g, r, nil
		}
	}
	return g, discovery.Record{}, errors.New("that release isn't listed anymore")
}

// prepare makes a release ready for the install confirmation: it reads
// the article when only a summary is known and tries the supported
// resolver once. Only metadata is fetched.
func (d *discoveryState) prepare(key, id string) (discovery.PreparedRelease, error) {
	_, r, err := d.release(key, id)
	if err != nil {
		return discovery.PreparedRelease{}, err
	}
	src := r.Entry.SourceID
	if !slices.Contains(d.enabled(), src) {
		return discovery.PreparedRelease{}, sources.ErrDisabled
	}
	ctx, cancel := context.WithTimeout(d.ctx(), 3*time.Minute)
	defer cancel()
	ix := d.index()
	if r.Entry.SummaryOnly || r.Detailed.IsZero() {
		f, s, err := d.fetcher(src)
		if err != nil {
			return discovery.PreparedRelease{}, err
		}
		if err := discovery.FetchDetail(ctx, ix, s.Source, f, id, time.Now()); err != nil && ctx.Err() != nil {
			return discovery.PreparedRelease{}, err
		}
	}
	if cur, ok := ix.Record(src, id); ok && discovery.ReleaseOf(cur, "").Availability == discovery.AvailUnresolved {
		resolver, err := sources.NewResolver(true)
		if err != nil {
			return discovery.PreparedRelease{}, err
		}
		discovery.Resolve(ctx, ix, src, id, resolver)
		resolver.Close()
	}
	d.save()
	d.c.emit(EventStoreGames, discovery.Change{Keys: []string{key}})
	return d.prepared(key, id)
}

// prepared describes a release's current state for the confirmation.
func (d *discoveryState) prepared(key, id string) (discovery.PreparedRelease, error) {
	_, r, err := d.release(key, id)
	if err != nil {
		return discovery.PreparedRelease{}, err
	}
	name := d.sourceName(r.Entry.SourceID)
	rel := discovery.ReleaseOf(r, name)
	p := discovery.PreparedRelease{GameKey: key, Release: rel, Offers: []discovery.PreparedOffer{}, Warnings: rel.Warnings}
	switch rel.Availability {
	case discovery.AvailInstallable:
		p.State, p.Ready = "ready", true
	case discovery.AvailUpdateOnly:
		p.State, p.Reason = "update-only", "This is an update for an installed copy, not a standalone game."
	case discovery.AvailGone:
		p.State, p.Reason = "unavailable", "The source no longer lists this release."
	default:
		p.State = "unresolved"
		p.Reason = "No validated torrent yet. Open the release page in your browser, get the .torrent file there, then attach it."
		if len(rel.Unresolved) > 0 {
			p.Reason = rel.Unresolved[0] + ". Open the release page in your browser, get the .torrent file there, then attach it."
		}
	}
	e := catalog.Entry{Key: key}
	d.c.catalog.annotator()(&e)
	if e.Installed != nil {
		p.Installed = &discovery.InstalledCopy{Version: e.Installed.Version, Dir: e.Installed.Dir}
	}
	langs, complete := discovery.ParseLanguages(r.Entry.LanguageClaim)
	if !complete {
		langs = []string{} // an incomplete claim mustn't limit the language choice
	}
	for _, i := range validTransports(r.Entry) {
		t := r.Entry.Transports[i]
		size := t.SizeBytes
		if size == 0 && !r.Entry.SizeIsMinimum && len(r.Entry.SizeOptionsBytes) == 0 {
			size = r.Entry.SizeBytes
		}
		p.Offers = append(p.Offers, discovery.PreparedOffer{Transport: i, Title: r.Entry.Title, Version: r.Entry.Version, SizeBytes: size,
			InstalledSizeBytes: r.Entry.InstalledSizeBytes, Languages: langs, TorrentName: t.TorrentName, InfoHash: t.InfoHash, SourceName: name})
	}
	if p.Ready && len(p.Offers) == 0 {
		p.Ready, p.State = false, "unresolved"
	}
	return p, nil
}

// validTransports mirrors discovery's rule: magnets with a v1 info hash.
func validTransports(e sources.Entry) []int {
	var out []int
	for i, t := range e.Transports {
		if t.Kind != "magnet" || t.InfoHash == "" {
			continue
		}
		if m, err := sources.Magnet(t.URI); err == nil && m.InfoHash == t.InfoHash {
			out = append(out, i)
		}
	}
	return out
}

// steamOnly is a Steam game without a known source release: wishlist and
// metadata only.
func (d *discoveryState) steamOnly(appID int, name string) discovery.GameSummary {
	s := discovery.GameSummary{Key: "steam:" + strconv.Itoa(appID), Title: name, SteamAppID: appID, Sources: []string{}, Languages: []string{}, Genres: []string{}}
	d.annotator()(&s)
	return s
}

// gameRef finds a Store game's title and Steam AppID by key, for art,
// enrichment and the wishlist: a discovered game, a catalog game, or a
// Steam-only search result.
func (d *discoveryState) gameRef(key string) (string, int, bool) {
	if g, ok := d.currentView().Game(key); ok {
		return g.Title, g.AppID, true
	}
	if id, err := strconv.Atoi(strings.TrimPrefix(key, "steam:")); err == nil && strings.HasPrefix(key, "steam:") && id > 0 {
		d.mu.Lock()
		name := d.steamNames[id]
		d.mu.Unlock()
		if name == "" && d.c.wishlist != nil {
			name = d.c.wishlist.title(key)
		}
		return name, id, name != ""
	}
	return "", 0, false
}

// searchRemote asks the chosen source sites and Steam for text, merges
// what the sites find into the index, and reports progress. A newer search
// cancels this one; it then returns what it has.
func (d *discoveryState) searchRemote(text string) ([]discovery.ProviderProgress, []discovery.SteamHit, bool) {
	text = strings.TrimSpace(text)
	srcs := d.enabled()
	progress := make([]discovery.ProviderProgress, 0, len(srcs)+1)
	for _, src := range srcs {
		progress = append(progress, discovery.ProviderProgress{ID: src, Name: d.sourceName(src), State: discovery.StateLoading})
	}
	progress = append(progress, discovery.ProviderProgress{ID: "steam", Name: "Steam", State: discovery.StateLoading})
	if len([]rune(scan.Normalize(text))) < discovery.MinSearch {
		for i := range progress {
			progress[i].State = discovery.StateSkipped
		}
		return progress, nil, true
	}
	run := d.ctx() // before d.mu: ctx takes it too
	d.mu.Lock()
	if d.searchStop != nil {
		d.searchStop()
	}
	ctx, cancel := context.WithTimeout(run, 45*time.Second)
	d.searchStop = cancel
	d.searchSeq++
	seq := d.searchSeq
	d.mu.Unlock()
	defer cancel()

	var mu sync.Mutex
	var steam []discovery.SteamHit
	added := false
	report := func(i int, fn func(*discovery.ProviderProgress)) {
		mu.Lock()
		fn(&progress[i])
		snapshot := slices.Clone(progress)
		mu.Unlock()
		d.mu.Lock()
		current := d.searchSeq == seq
		d.mu.Unlock()
		if current {
			d.c.emit(EventStoreSearch, discovery.SearchProgress{Text: text, Remote: snapshot})
		}
	}
	d.c.emit(EventStoreSearch, discovery.SearchProgress{Text: text, Remote: slices.Clone(progress)})
	var wg sync.WaitGroup
	for i, src := range srcs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			now := time.Now()
			if n, ok := d.search.Source(src, text, now); ok {
				report(i, func(p *discovery.ProviderProgress) { p.State, p.Found, p.Cached = discovery.StateOK, n, true })
				return
			}
			if c := d.index().Crawl(src); now.Before(c.RetryAt) {
				report(i, func(p *discovery.ProviderProgress) {
					p.State, p.Error = discovery.StateError, fmt.Sprintf("asked to wait until %s", c.RetryAt.Local().Format("15:04"))
				})
				return
			}
			f, s, err := d.fetcher(src)
			if err == nil {
				before := d.index().Counts()[src]
				var n int
				n, err = discovery.SearchSource(ctx, d.index(), s, f, text, time.Now())
				if err == nil {
					d.search.Put(src, text, n, nil, time.Now())
					if d.index().Counts()[src] > before {
						mu.Lock()
						added = true
						mu.Unlock()
					}
					report(i, func(p *discovery.ProviderProgress) { p.State, p.Found = discovery.StateOK, n })
					return
				}
			}
			report(i, func(p *discovery.ProviderProgress) { p.State, p.Error = discovery.StateError, searchError(err) })
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		i := len(progress) - 1
		if hits, ok := d.search.Steam(text, time.Now()); ok {
			mu.Lock()
			steam = hits
			mu.Unlock()
			report(i, func(p *discovery.ProviderProgress) { p.State, p.Found, p.Cached = discovery.StateOK, len(hits), true })
			return
		}
		if d.c.meta == nil {
			report(i, func(p *discovery.ProviderProgress) { p.State = discovery.StateSkipped })
			return
		}
		found, err := d.c.meta.client.SearchSteam(ctx, text)
		if err != nil {
			report(i, func(p *discovery.ProviderProgress) { p.State, p.Error = discovery.StateError, searchError(err) })
			return
		}
		hits := make([]discovery.SteamHit, 0, len(found))
		for _, h := range found {
			hits = append(hits, discovery.SteamHit{AppID: h.AppID, Name: h.Name})
		}
		d.search.Put("steam", text, len(hits), hits, time.Now())
		mu.Lock()
		steam = hits
		mu.Unlock()
		report(i, func(p *discovery.ProviderProgress) { p.State, p.Found = discovery.StateOK, len(hits) })
	}()
	wg.Wait()
	if added {
		d.save()
		d.indexChanged(nil, "")
	}
	complete := ctx.Err() == nil
	mu.Lock()
	defer mu.Unlock()
	for i := range progress {
		if progress[i].State == discovery.StateLoading {
			progress[i].State, progress[i].Error = discovery.StateSkipped, "a newer search replaced this one"
			complete = false
		}
	}
	return progress, steam, complete
}

func searchError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "stopped"
	}
	return err.Error()
}

// others are Steam hits without a source release in the index, under
// "Other games".
func (d *discoveryState) others(hits []discovery.SteamHit, text string) []discovery.GameSummary {
	out := []discovery.GameSummary{}
	view := d.currentView()
	for _, h := range hits {
		if h.AppID <= 0 || !discovery.Matches(h.Name, text) {
			continue
		}
		if _, ok := view.Game("steam:" + strconv.Itoa(h.AppID)); ok {
			continue
		}
		d.mu.Lock()
		d.steamNames[h.AppID] = h.Name
		d.mu.Unlock()
		out = append(out, d.steamOnly(h.AppID, h.Name))
		if len(out) == 20 {
			break
		}
	}
	return out
}
