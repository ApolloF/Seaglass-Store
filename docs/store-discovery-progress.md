# Store discovery: implementation progress

Tracks the implementation of [store-discovery-plan.md](store-discovery-plan.md) so another instance can resume. Branch: `ApolloF/Discovery` (worktree `orca/workspaces/Seaglass-Store/Discovery`), started from `main` at `f38705e`. Nothing is merged, tagged or released.

## Milestones

| # | Work package | Owner | Status | Commit |
|---|---|---|---|---|
| 1 | Contracts, persistence/migration, service interfaces, mock fixtures | coordinator (Opus 5.5) | done | `b16ffd7`, `228f5c9` |
| 2 | Discovery backend | coordinator | done | `4842da5` |
| 3 | Metadata providers (`internal/store/enrich`) | Sonnet 5.5 worker, high effort | done, merged | `0783f2c`, merge `f60fd8e` |
| 4 | Desktop Store UI | Sonnet 5.5 worker, medium effort | done, merged | `2e0a538`, merge `1b35baa` |
| 5 | Integration, wishlist, bindings, docs, verification | coordinator | done | `75905a5`, `518eae0`, `5ef1998`, docs commit |

All work packages are complete. What is left is listed under [Remaining work](#remaining-work-and-limitations).

## Shared contracts (work package 1)

Owned by the coordinator. Workers propose changes; they don't make them.

- `internal/store/discovery/types.go`: persisted `Record`, `CrawlState`, `IdentityCorrection`; interface models `Status`, `SourceStatus`, `GameSummary`, `BrowseQuery`, `BrowsePage`, `SearchResult`, `ProviderProgress`, `SearchProgress`, `Home`, `Release`, `Identity`, `GameDetails`, `PreparedRelease`, `PreparedOffer`, `Change`; state, crawl, availability and sort constants.
- `internal/store/enrich/types.go`: `Popularity`, `Score`, `ReviewSummary`, `Review`, `ReviewQuery`, `ReviewPage`, `Critic`, `Completion`, `CompletionQuery`, `Candidate`, `Enrichment`; cache lifetimes. `client.go` holds the `Client` method set the app calls (stubs until work package 3).
- `internal/store/wishlist/types.go`: `Entry`, `Activity`, `File`, `Observation`, `ActivityView`.
- `internal/settings`: `StoreSettings.Sources` (per PC; `fitgirl`, `dodi`), `StoreSettings.SourceSetup` (`""` new Store user, `"ask"` existing Store user chooses once, `"done"`), `StoreTurnedOn` (turning the Store on as a new Store user chooses both sources and turns source browsing on), `DiscoveryOn`. Migration: a settings file without `store.sourceSetup` whose Store was on becomes `"ask"` with no sources; feeds, trust and `privateSources` are kept as they were.
- Later contract changes: `OpenStoreLink` (attribution links, `228f5c9`); `PreparedRelease.installed` and game details for Steam-only keys (`518eae0`).
- `internal/app/store_discovery_service.go`: new `StoreService` methods and events:
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

## File ownership while the workers ran (historical)

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

Run on 2 October 2026 on Windows 11 (Go 1.27.0, Node 24.19.0, Wails v3.0.0-beta.26), at `5ef1998` plus the documentation commit. "Not run" means not run.

| Check | Result |
|---|---|
| `wails3 generate bindings -f '-tags production' -clean=true -ts -i` | pass; output committed, no diff after the last run |
| `go vet ./...` | pass |
| `go test ./...` | pass (every package; the root package needs `frontend/dist`, built by `npm run build` first) |
| `go test -race ./internal/...` | pass (every package), after installing gcc (WinLibs MinGW-w64 16.2) on 2 October; earlier not run because the machine had no C toolchain |
| `cd frontend && npm run check` | pass, 0 errors, 0 warnings |
| `cd frontend && npm run test` | pass, 13 files, 86 tests |
| `cd frontend && npm run build` | pass |
| `wails3 build` | pass, `bin/Seaglass.exe` |
| `git diff --check` (working tree and `f38705e..HEAD`) | pass |
| Live: `WL_STORE_LIVE=1 go test -run LiveDiscovery -v ./internal/store/discovery` | pass: FitGirl 5 pages, 42 releases; DODI 5 pages, 48 releases; source search "witcher" 7 and 4 results; 92 games, 42 installable |
| Live: `WL_STORE_LIVE=1 go test -run Live -v ./internal/store/enrich` | pass: chart 100 games; Portal 2 summary 98 % of 467,428 (recent 98 % of 1,715, no Steam label); review pages 1 and 2 through the cursor; Metacritic 95 with link; HowLongToBeat search and detail id 7231, 515 / 826 / 1376 min |
| Real app: `node tools/harness/discovery.mjs` (dev build) | pass, 22 checks: Home filled from the sources without a feed; search; Enter opens a game, Escape goes back; game page with releases, reviews and times; wishlist saved and persisted; install confirmation opened and closed; nothing downloaded; no horizontal scrolling at 390 px in both themes on every screen |
| Mock UI (work package 4 worker, Playwright on Edge) | both themes at 1280 and 390 px for Home, Browse, Wishlist, game page, install dialog, Settings; `discovery=setup/empty/offline/nochart/slow` scenarios; keyboard pass |
| `WL_REAL_QBIT=1 go test -run Real -v ./internal/torrent/qbit` | pass on qBittorrent 5.2.4 after the login and add fix below; failed before it ("qBittorrent refused the login") |
| `node tools/harness/store.mjs` (download and install pipeline) | pass on qBittorrent 5.2.4: Get, download through the web seed, install into the games folder (joins the library's folders), uninstall. The harness's test data now answers source setup (feed only) and opens the feed game from Browse. |

Earlier: `go test ./...` before any change passed except the root package (no `frontend/dist` yet).

Bugs found by verification and fixed: a deadlock after the first remote search (found by the real-app harness, `5ef1998`); a feed or listing page with only announcements aborted a pass (found by tests, fixed in `4842da5`).

Pre-existing, found by `store.mjs` and fixed: qBittorrent 5.2 answers a login with an empty 204 (401 when refused) and an add with JSON counts, where 5.1 said `Ok.`/`Fails.`. Seaglass took the 204 for a refusal and restarted the sidecar with backoff, so every Store download stayed queued with any current qBittorrent. `internal/torrent/qbit/client.go` now accepts both versions' answers (`TestClientLogsInToQBittorrent52`).

Pre-existing, found while debugging and not changed: while qBittorrent failed to start, the Downloads page said *No downloads* although a job was queued, and the engine's error isn't shown anywhere on that page.

## Delegation

The in-session agent tool could not pin a subagent's effort: custom agent definitions in a new agents directory load only after a restart, and its isolated worktrees start from `main`. Work packages 3 and 4 therefore ran as headless Claude Code workers (`claude -p --model claude-sonnet-5-5 --effort high|medium`), each in its own git worktree created from `228f5c9` (`Discovery-providers`, `Discovery-ui`, since removed; the branches `store-discovery-providers` and `store-discovery-ui` remain). Both followed the ownership rules; their commits were reviewed and merged with `--no-ff`.

Worker proposals and what became of them:
- Prepared releases should carry the installed folder: done (`PreparedRelease.installed`, `518eae0`).
- Game details for Steam-only keys: done (`518eae0`).
- A type field (game, DLC, mod) on HowLongToBeat candidates: done (`Candidate.type`); the *Wrong game?* picker labels DLC and mods.
- A "being resolved" flag on summaries: not done; optional.
- Flush the enrichment cache on exit: done (`Core.Stop`).

## Decisions made during implementation

- Steam's keyless `IUserReviewsService/GetAppReviews` ignored the filter, language and day range, had no persona names and totals differing from the store page, so reviews use Steam's documented `store.steampowered.com/appreviews/<appid>?json=1`. The recent summary comes from Steam's review histogram (last 30 days); Steam gives it no label.
- HowLongToBeat uses the site's own anonymous search flow (`/api/search/site/init` token, then `/api/search/site`). Its init endpoint answered 403 without a `Referer: https://howlongtobeat.com/` header, so the provider sends that header and a descriptive User-Agent; no cookies, accounts, browser spoofing or challenge solving. If the site blocks again, cached times show as stale, otherwise "Times unavailable" with a link. Reviewed and kept by the maintainer on 2 October; the plan's rule no longer lists CAPTCHA bypasses.
- Seaglass's game database identifies releases for merging only on exact titles (confidence 85), or titles differing by an edition sold as the same game (75, unless the edition word is remaster, remake, definitive, director's cut, final cut, enhanced, anniversary, reloaded or redux). Looser matches don't merge, so some games have no Steam identity and therefore no reviews until corrected.
- A first pass reads the newest listing page and leaves older pages to backfill; later refreshes catch up page by page until a page brings nothing new. A search result published more than a week ago counts as backfill, so finding it is never wishlist news.
- Language claims are parsed to installer language names; a claim with an unknown part (MULTi9) doesn't restrict the language choice at install.
- In the test harness's frozen mode (`--dev-data`), discovery indexes only when asked (setup, refresh), like the rest of the store.
- The old manual review flow (Settings, Source discovery) is replaced in the interface; its service methods remain for compatibility.

## Remaining work and limitations

- Feed games show on Browse only, not on Home's shelves (they have no source publication date). Decided 2 October: no feed shelf; the maintainer uses the repack sources only.
- The Downloads page hides a queued job and the engine's error while qBittorrent can't start (pre-existing).
- The desktop shell (sidebar, 980 px minimum window) has no phone layout; the Store content itself is verified at 390 px with the sidebar hidden.
- The harness's `--dev-data` redirects roaming data only: discovery and enrichment caches go to the real `%LOCALAPPDATA%\Seaglass\store` (removed after the runs here).
- Big Picture keeps its existing Downloads screen; it has no discovery Store (as planned).
- Optional contract additions above. Nothing is merged, tagged or released.
