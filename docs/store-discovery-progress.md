# Store discovery: implementation progress

Tracks the implementation of [store-discovery-plan.md](store-discovery-plan.md) so another instance can resume. Branch: `ApolloF/Discovery` (worktree `orca/workspaces/Seaglass-Store/Discovery`), started from `main` at `f38705e`. Nothing is merged, tagged or released.

## Milestones

| # | Work package | Owner | Status | Commit |
|---|---|---|---|---|
| 1 | Contracts, persistence/migration, service interfaces, mock fixtures | coordinator | done | see log below |
| 2 | Discovery backend | coordinator | not started | |
| 3 | Metadata providers (`internal/store/enrich`) | Sonnet 5.5 worker, high effort | not started | |
| 4 | Desktop Store UI | Sonnet 5.5 worker, medium effort | not started | |
| 5 | Integration, wishlist, bindings, docs, verification | coordinator | not started | |

## Shared contracts (work package 1)

Owned by the coordinator. Workers propose changes; they don't make them.

- `internal/store/discovery/types.go`: persisted `Record`, `CrawlState`, `IdentityCorrection`; interface models `Status`, `SourceStatus`, `GameSummary`, `BrowseQuery`, `BrowsePage`, `SearchResult`, `ProviderProgress`, `SearchProgress`, `Home`, `Release`, `Identity`, `GameDetails`, `PreparedRelease`, `PreparedOffer`, `Change`; state, crawl, availability and sort constants.
- `internal/store/enrich/types.go`: `Popularity`, `Score`, `ReviewSummary`, `Review`, `ReviewQuery`, `ReviewPage`, `Critic`, `Completion`, `CompletionQuery`, `Candidate`, `Enrichment`; cache lifetimes. `client.go` holds the `Client` method set the app calls (stubs until work package 3).
- `internal/store/wishlist/types.go`: `Entry`, `Activity`, `File`, `Observation`, `ActivityView`.
- `internal/settings`: `StoreSettings.Sources` (per PC; `fitgirl`, `dodi`), `StoreSettings.SourceSetup` (`""` new Store user, `"ask"` existing Store user chooses once, `"done"`), `StoreTurnedOn` (turning the Store on as a new Store user chooses both sources and turns source browsing on), `DiscoveryOn`. Migration: a settings file without `store.sourceSetup` whose Store was on becomes `"ask"` with no sources; feeds, trust and `privateSources` are kept as they were.
- `internal/app/store_discovery_service.go`: new `StoreService` methods and events (stubs return empty answers until work packages 2 and 5):
  - Discovery: `DiscoveryStatus`, `SetupSources`, `RefreshDiscovery`, `StoreHome`, `BrowseGames` (local, at once), `SearchGames` (remote fill, newest call wins), `GameDetails`, `SetSteamMatch`, `PrepareRelease`, `AttachSourceTorrent`, `OpenSourceRelease`, `DownloadRelease`.
  - Enrichment: `EnrichGames` (cached for cards; the rest by event), `GameEnrichment` (page, fetches), `GameReviews`, `CompletionCandidates`, `SetCompletionMatch`.
  - Wishlist: `Wishlist`, `AddToWishlist`, `RemoveFromWishlist`, `AcknowledgeWishlist`; `WishlistItem`.
  - Events: `store:discovery` (Status), `store:games` (Change), `store:search` (SearchProgress), `store:enrichment` (Enrichment), `store:wishlist` ([]WishlistItem). Existing events and feed APIs are unchanged.
- Frontend: `frontend/src/lib/types.ts` mirrors all of the above; `api.ts` adds `api.store.discovery`, `api.store.enrich`, `api.store.wishlist`; `api.real.ts` wires them to the bindings; `api.mock.discovery.ts` serves fixtures (scenarios through `?store=1&discovery=setup|empty|offline|nochart|slow`).

### Persistence layout

- `%LOCALAPPDATA%\Seaglass\store\discovery\`: index records per source, crawl state, search cache (disposable).
- `%LOCALAPPDATA%\Seaglass\store\enrich\`: provider caches (disposable).
- `%APPDATA%\Seaglass\wishlist.json`: wishlist and activity (kept).
- `%APPDATA%\Seaglass\store-identity.json`: Steam and HowLongToBeat match corrections (kept).

All JSON, written with tmp + rename.

### Game keys

`steam:<appid>` when a trusted Steam identity exists (Seaglass's game database at confidence 72 or more, a feed's AppID, or the person's correction), else `title:<scan.Normalize(title)>`, the same scheme as `catalog.Entry.Key`, so installed games, downloads (`Job.GameKey`), art and updates line up. Entries merge across sources only through that key: a trusted Steam identity or an identical normalized title. Sequels, remasters and DLC stay apart.

## File ownership while workers run

- Worker A (providers, work package 3): `internal/store/enrich/**` except `types.go`. May add hosts to its own allowlist inside the package. Nothing else.
- Worker B (desktop Store, work package 4): `frontend/src/desktop/Store.svelte`, `StoreGame.svelte`, `InstallDialog.svelte`, `SourceBrowser.svelte`, the source section of `StoreSettings.svelte`, new files under `frontend/src/desktop/store/`, new pure helpers `frontend/src/lib/storefront*.ts` with tests, a new state module `frontend/src/lib/storefront.svelte.ts`. May add fixture data (not shapes) to `api.mock.discovery.ts`.
- Coordinator: everything else, including `types.ts`, `api.ts`, `api.real.ts`, all Go outside `internal/store/enrich`, bindings, docs.

## Acceptance scenarios

Each maps to tests named for the behaviour.

1. A new person turns the Store on: both sources are chosen, indexing starts at once, Home fills from recent listings without any feed URL.
2. An existing Store user (Store on before this version) sees the one-time source choice; nothing is indexed until they choose; their feeds, trust levels and reviewed offers still show.
3. Restarting shows the cached index at once; listings older than six hours refresh in the background; Refresh does it now.
4. Backfill fetches five older listing pages per source per pass, at most every 30 minutes, resumes after a restart at the saved page, retries an interrupted page, and stops at a confirmed end.
5. A failed page or source keeps every cached game; a vanished article is marked unavailable, never deleted while wishlisted or installed.
6. Background work pauses while a game runs and stops when the Store or source browsing is turned off or the app exits. One request per source at a time, at least two seconds apart, `Retry-After` honoured, capped exponential backoff.
7. Search returns cached matches at once; after 600 ms and two characters it searches the chosen sources and Steam; a newer query cancels the older one and stale answers are ignored; a failing provider keeps the local matches and says it failed.
8. Steam games without a source release show under Other games, "No known source release", with wishlist but no install.
9. Popular follows Steam's chart among source-backed games; without the chart the shelf shows its empty state.
10. Identity: identical normalized titles and trusted Steam AppIDs merge; sequels, remasters and DLC don't; a manual Steam correction survives refreshes.
11. Wishlist: saving sets a baseline; a first source release and a confirmed newer version become unread activity; backfilled history never does; activity stays until acknowledged; Steam-only games can be saved; the wishlist survives restarts.
12. Game page: source-labelled releases with version, size, languages, publication date and availability; Steam overall and recent reviews; paged review text with helpfulness, playtime and links; Metacritic when Steam gives it; HowLongToBeat times or a stale or unavailable label with a link; choose another HowLongToBeat match.
13. Installing: preparing a release fetches its article and tries the supported resolver once; unresolved releases offer the browser and torrent attachment; no download button without a validated transport; English default, optional packs, payload scan preference, integrity checks and version guards as before. Browsing, wishlisting and enrichment never download a payload or run an installer.

## Verification log

Filled in as checks run. "Not run" means not run.

| Check | Result |
|---|---|
| `go test ./...` (baseline, before changes) | pass, except the root package, which needs `frontend/dist` (built by `npm run build`) |

## Remaining work

See the milestone table.
