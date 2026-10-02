# Seaglass Store: automatic discovery, search, reviews, and wishlist

Status: agreed implementation plan, 2 October 2026. Not implemented yet.

## 1. Goal and agreed decisions

Turn the existing Store into a Steam-like browsing experience that works without adding feed URLs or manually importing each release.

Work in **ApolloF/Seaglass-Store**. Start from the fork's current `main`; use any suitable checkout or isolated worktree. There is no requirement to use a particular worktree or branch. Preserve the functionality released in `v1.9.0-store.2` and integrate subsequent fork changes.

Agreed behavior:

- Index FitGirl and DODI locally on each PC.
- Show recent releases first, then progressively index older listings while idle.
- Browse source-backed games; search may also find Steam games with no known repack so they can be wishlisted.
- Rank **Popular** using Steam's most-played chart, intersected with source-backed games.
- Show Steam scores and reviews, Steam-provided Metacritic score/link, and HowLongToBeat completion times.
- Save games to a wishlist, following their future releases rather than bookmarking individual releases.
- Show wishlist activity through in-store badges; no desktop notifications.
- Automatically display discoveries; confirm the chosen release and languages when installing.
- Enable both sources when a new user enables Store. Existing users receive a one-time source setup choice.
- Deliver the desktop Store first, responsive to phone width. Preserve the existing Big Picture Downloads interface.

### Instructions for the implementing instance

This is an implementation handoff with product decisions already agreed by the maintainer. Proceed with these decisions without reopening settled preferences. Follow the repository's AGENTS.md, inspect the current implementation, and complete each runnable milestone. Record concrete blockers and continue independent work that remains feasible. Report what actually ran and avoid claiming unsupported results.

The task covers public release metadata discovery, game metadata enrichment, wishlist persistence, and integration with the existing user-initiated download/install pipeline. Do not add copy-protection bypasses, CAPTCHA bypasses, account/session harvesting, new payload execution paths, or automatic game downloads. Source transport limitations remain explicit in the UI. This plan does not assert rights or authenticity for any particular third-party release.

Do not merge, tag, or publish a release without a later instruction. Additional chats or agents should only be dispatched when requested; the work packages below can also be completed sequentially by one instance.

## 2. Backend and data flow

### Persistent discovery catalog

Extend the existing source adapters rather than introduce another scraper framework. Keep discovery metadata distinct from validated installable offers: an unresolved listing is still browseable.

Persist:

- Source entries keyed by source and canonical article URL.
- Publication/update timestamps, source claims, references, resolved torrent identities, and fetch provenance.
- Canonical game identities, optional Steam AppIDs, and user corrections.
- Per-source crawl cursor, refresh status, errors, and retry time.
- Versioned metadata caches.

Use the repository's JSON and atomic-write conventions. Put disposable discovery/provider caches under the local cache directory; put wishlist and user identity corrections under roaming app data. Wishlist remains local in this version; no Steam wishlist synchronization or cross-PC synchronization is required.

Merge entries across sources only through a trusted Steam identity or an unambiguous normalized title. Do not merge sequels, remasters, or DLC through loose similarity. Allow manual Steam-match correction on game details; retain that correction across refreshes.

Do not discard cached games because one fetch fails or an article disappears. Mark unavailable source entries appropriately and retain wishlisted games.

### Automatic refresh and backfill

- Load cached results immediately at startup.
- Refresh recent listings when six hours old; offer a manual refresh button.
- Start first-use discovery immediately after source setup.
- Use FitGirl's paginated article listings for historical backfill; retain RSS as a recent-release input.
- Process five listing pages per enabled source per background pass, with passes at most every 30 minutes.
- Persist pagination progress; stop at a confirmed end and retry interrupted pages.
- Pause background work while playing; cancel requests when Store/source browsing is disabled or the app exits.
- Keep one request in flight per source, spaced at least two seconds apart. Honor `Retry-After` and use capped exponential backoff.
- Fetch release details on demand, prioritizing opened games and wishlist entries. Do not eagerly resolve every file-host link.

Preserve existing URL validation, response limits, source boundaries, and torrent-metadata validation. Background indexing must not start torrent peers, download game payloads, or execute installers.

### Unified search

Return cached matches immediately. After a 600 ms debounce and at least two characters, search enabled source sites and Steam to fill gaps. Cancel superseded requests and ignore stale responses.

Cache source search results for one hour. Paginate local results in groups of 60; expose remote-search progress separately. A failed provider must not erase local matches.

Steam-only results belong in an **Other games** group, labelled **No known source release**. They support wishlist and metadata actions, but have no install button.

### Public service interfaces

Extend `StoreService` and the real/mock frontend gateways with:

- Discovery setup, status, refresh, and paginated browse/search.
- Home sections and game details, including unresolved source releases.
- Wishlist add/remove/list and activity acknowledgement.
- Steam review pagination and completion-time metadata.
- Source-release preparation for the existing install confirmation flow.
- Manual Steam identity correction.

Use typed request/response models with loading, stale, unavailable, and error states. Emit catalog/discovery and wishlist-change events. Keep existing feed APIs compatible and regenerate Wails bindings after interfaces stabilize.

## 3. Store experience and enrichment

### Home, Browse, and Wishlist

Reuse existing components, typography, colors, and spacing. Use a restrained game storefront layout with artwork, readable metadata, and sections; no autoplay media.

**Home**

- New repacks: descending source publication date.
- Popular: current Steam chart order among source-backed games.
- Recently updated: changed source release entries.
- Wishlist activity: newly available releases and confirmed newer versions.

Keep source publication dates separate from original game release dates and game build dates. A changed article or repack timestamp alone must not create a newer-version claim. If no source-backed games match the Steam chart, show the section's empty state rather than relabel another metric as popularity.

**Browse**

Provide unified search and filters for source, language, genre, availability, and installed status. Offer sorting by title, source publication date, Steam popularity, and review score. Unknown fields remain unknown rather than failing filters through invented values.

**Wishlist**

Support saving source-backed and Steam-only games. Persist entries across restarts. Establish an activity baseline when a game is saved; historical backfill must not mark every old release as new. Preserve unread activity until acknowledged.

### Game details

Combine existing artwork and descriptions with:

- Source-labelled FitGirl/DODI release choices.
- Version, size, languages, publication date, and availability.
- Existing comparable-version recommendation and update guards.
- Steam overall/recent review summaries.
- Paginated Steam review text, helpfulness, playtime where available, and original links.
- Metacritic score/link when Steam supplies them.
- HowLongToBeat Main Story, Main + Extras, and Completionist times.
- Wishlist control and identity correction.

Render external review content as text, with attribution and source links. Do not scrape full critic articles.

Use Steam's current public `IUserReviewsService/GetAppReviews` interface. Cache popularity for one hour, review summaries for six hours, review pages for one hour, and critic metadata for seven days. Fetch enrichment for visible/opened/wishlisted games rather than the entire catalog.

Implement HLTB as an isolated Go provider with seven-day caching. Match conservatively and allow selecting a correct HLTB result. Verify search and game-detail retrieval early using live public responses, then capture fixtures. If retrieval is blocked or its format changes, retain cached times with a stale label; otherwise show unavailable times and a link. Do not substitute estimated completion times.

### Installation

Remove the separate catalog-import approval requirement for automatic discoveries.

Selecting a release prepares validated metadata, then opens the existing install confirmation with source, version, languages, size, and unresolved warnings. Keep English defaults, optional-pack handling, payload-scan preferences, and integrity checks.

For missing torrent metadata, attempt the existing supported resolver on demand. If it remains unresolved, offer browser handoff and torrent attachment. Do not show a functioning download action without a validated transport.

Browsing, wishlisting, and enrichment never initiate installation or payload downloading.

## 4. Work packages for later instances

Use separate branches/worktrees for concurrent implementation. One coordinator owns shared contracts, integration, generated bindings, and final verification.

1. **Contracts and migration - coordinator.** Define typed discovery/game/wishlist/provider responses, persistent records, setup migration, mock fixtures, and acceptance scenarios. Commit this before parallel implementation.
2. **Discovery backend - instance A.** Implement persistent indexing, refresh/backfill, source search, identity grouping/corrections, status events, and release preparation. Own discovery modules and backend tests.
3. **Metadata providers - instance B.** Implement Steam popularity/reviews/critic fields and HLTB retrieval, matching, caching, and provider tests. Consume the agreed contracts.
4. **Desktop Store - instance C.** Implement Home/Browse/Wishlist, unified search, filters, detail panels, setup, and installation handoff against mock APIs. Own desktop components and frontend tests.
5. **Integration - coordinator.** Wire services to lifecycle/settings, integrate provider and discovery results, implement wishlist persistence/activity, regenerate bindings, run verification, and update documentation.

Each handoff reports its commit, changed behavior, checks run, and concrete limitations. Avoid simultaneous edits to shared types, generated bindings, or service registration. Keep every milestone runnable.

### Current implementation starting points

- `internal/store/sources`: bounded source parsing, collection, pagination, network validation, and metadata resolution.
- `internal/app/store_catalog.go` and `store_sources.go`: cached feeds, catalog rebuilding, on-demand previews, and explicit reviewed offers.
- `internal/store/catalog`: grouping, querying, release comparison, and recommendations.
- `internal/meta`: Steam search, game metadata, rate limiting, and artwork.
- `frontend/src/desktop/Store.svelte`, `StoreGame.svelte`, `SourceBrowser.svelte`, and `InstallDialog.svelte`: existing Store and installation UI.
- `frontend/src/lib/api*`, `types.ts`, and `shop.svelte.ts`: real/mock gateways, shared models, and Store state.

The current Store empty state checks configured feeds and can hide reviewed source-only catalogs. Replace that condition with actual discovery/catalog availability as part of the new Store experience.

## 5. Checks and completion criteria

Add behavior tests covering:

- Pagination, resume, duplicate entries, article updates, partial failures, cancellation, and backoff.
- Immediate cached search plus delayed remote results; superseded queries cannot overwrite current results.
- Conservative identity matching, sequel/DLC separation, and persisted corrections.
- Correct new/popular/updated ordering; missing popularity never becomes a fabricated rank.
- Wishlist persistence, Steam-only games, first availability, confirmed updates, acknowledgement, and historical-backfill suppression.
- Provider parsing, pagination, rate limits, stale caches, missing scores/times, and HLTB matching.
- Install-time validation, unresolved transport fallback, existing version guards, English defaults, and no background payload downloads.
- Migration of existing feeds, reviewed offers, disabled source preferences, and source setup.
- Keyboard navigation, loading/error/offline states, both themes, and 390 px layouts.

After integration, run and report:

```text
wails3 generate bindings -f '-tags production' -clean=true -ts -i
go vet ./...
go test ./...
go test -race ./internal/...
cd frontend: npm run check
cd frontend: npm run test
cd frontend: npm run build
wails3 build
git diff --check
```

The race detector requires a suitable Windows C toolchain. The `cd frontend:` notation identifies the working directory, not a literal shell command.

Run bounded live metadata checks against both release sources, Steam, and HLTB. Exercise discovery -> search -> game details -> wishlist -> install confirmation in the mock UI and a Windows build. Use the existing controlled installer fixture for pipeline regression; do not automatically execute third-party installers during validation.

Completion means the Store works without a feed URL, remains useful offline after indexing, shows honest source/metadata states, preserves existing download behavior, and passes the checks above. Report any device-dependent test failures separately and verify the complete suite in Windows CI.

## 6. Evidence and provider references

Planning-time checks on 2 October 2026 returned 100 entries from Steam's most-played endpoint, a Metacritic score/link from Steam game metadata, and HTTP 200 with embedded Next data from HLTB's public home page. These observations confirm initial feasibility, not completed provider integration; HLTB search/detail parsing still requires the early verification described above.

- [Steam review API documentation](https://partner.steamgames.com/doc/webapi/IUserReviewsService?l=english)
- [Steam most-played endpoint](https://api.steampowered.com/ISteamChartsService/GetMostPlayedGames/v1/)
- [HowLongToBeat](https://howlongtobeat.com/)
- Existing behavior: [Store release selection](store-release-selection.md), [private catalog sources](private-catalog.md), [experimental Store](experimental-store.md).
