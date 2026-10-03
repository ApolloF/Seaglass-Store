# Store experience and additional sources: progress

Tracks the work after v1.9.1-store.1: additional vetted sources (Part A, Codex), and the Store experience, integration and release (Part B, Claude). Branches: `claude/store-experience` (worktree `orca/workspaces/Seaglass-Store/Additions`) and `codex/source-expansion`. Both start from the source contract, commit `b972f4a` (`origin/source-contract`); the contract is described in [store-providers.md](store-providers.md).

## Release gate

The release (expected `v1.9.1-store.2`; recheck upstream versions and tags first) is published only when all of these are true:

- **Part A, additional sources (required, Codex):** ElAmigos is implemented as a provider on the contract, with its parser, transport behaviour, fixtures and tests, and a handoff with exact checks and live observations. KaOs is included only if implementation-time checks find usable public access (the planning check answered HTTP 403); no unverified mirror. A research-only document does not satisfy this gate. If ElAmigos fails verification, the blocker is reported and the release waits for a decision.
- Part A is integrated into setup, filters, indexing, wishlist matching, game pages and the release notes.
- Part B below is complete, reviewed and verified.
- Every check in [Verification](#verification) passes on the merged `main`.

## Milestones

| # | Work package | Owner | Status |
|---|---|---|---|
| 0 | Source contract (`b972f4a`) | coordinator (Opus 5.5 high) | done |
| 1 | Product contracts: types, service stubs, gateway, mocks | coordinator | done |
| A | Additional sources: ElAmigos (required), KaOs (conditional) | Codex (GPT-6.1-sol could not be selected; reported in the handoff), `codex/source-expansion` | done: ElAmigos (`4bf8bb8`, `dae2713`, handoff `e253089`); KaOs excluded (HTTP 403, no verified mirror) |
| 2 | Continuous indexing, pause/resume, Steam wishlist import, idle wishlist searches | worker, Opus 5.5 high | done |
| 3 | Library completion times (desktop details, Big Picture sheet) | worker, Sonnet 5.5 medium | done |
| 4 | Steam-style desktop Store, recommendations | worker, Sonnet 5.5 medium | done |
| 5 | Big Picture Store, download error visibility | worker, Opus 5.5 high | done |
| 6 | Integration of Part A (`04e9c26`), reviews (Opus 5.5 high), fixes (`claude/store-fix-indexing` Opus 5.5 high, `claude/store-fix-ui` Sonnet 5.5 medium, `claude/store-fix-followup` Opus 5.5 high), verification, release | coordinator | done |

## Product contracts (work package 1)

Owned by the coordinator; workers propose changes and don't make them.

- `discovery.Status.paused` / `playing`; `settings.StoreSettings.IndexingPaused` (per PC); `StoreService.PauseIndexing`.
- `discovery.GameSummary.releaseDate` (the game's own date from Steam, never a source date) and `completionMain` (cached HowLongToBeat Main Story minutes).
- `discovery.Home.featured`, `recommended` (`Recommendation{game, because, genres}`), `recommendedBasis` (`Basis*`); `discovery.Signal` for the recommender input.
- `wishlist.Entry.Origin` (`OriginManual`, `OriginSteam`); `app.WishlistItem.origin`; `app.SteamAccount`, `app.WishlistImport`; `StoreService.SteamWishlistAccount`, `StoreService.ImportSteamWishlist`.
- `app.LibraryCompletion`; `LibraryService.Completion`, `CompletionCandidates`, `SetCompletionMatch`, `OpenCompletionLink`.
- Frontend: `types.ts` mirrors all of it; `api.completion.*`, `api.store.discovery.pauseIndexing`, `api.store.wishlist.steamAccount` / `importSteam`; real and mock implementations (mock flags `?store=1&discovery=nosteam|private`).

## File ownership while workers run

- Worker 2 (indexing, wishlist import): `internal/store/discovery/crawl*.go`, `internal/app/store_discovery.go`, `internal/app/store_discovery_service.go` (except `StoreHome`), `internal/app/store_wishlist*.go`, `internal/store/wishlist/**` (except `types.go`), a new `internal/store/enrich/wishlist.go` with tests, `frontend/src/desktop/store/Wishlist.svelte`, `SourceSetup.svelte`, `StatusBar.svelte`, the sources section of `frontend/src/desktop/StoreSettings.svelte`, new files under `frontend/src/desktop/store/` named `Wishlist*`/`Indexing*`.
- Worker 3 (completion times): `internal/app/library_completion.go` and its test, `frontend/src/desktop/Details.svelte`, `frontend/src/bigpicture/GameSheet.svelte`, new `frontend/src/components/Completion*.svelte`, a new pure helper `frontend/src/lib/completion.ts` with tests.
- Worker 4 (desktop Store): `frontend/src/desktop/Store.svelte`, `StoreGame.svelte`, `frontend/src/desktop/store/**` except worker 2's files, `frontend/src/lib/storefront*.ts`, a new `internal/store/discovery/recommend.go` with tests, `Home()` in `internal/store/discovery/query.go`, `StoreHome` in `internal/app/store_discovery_service.go`, a new `internal/app/store_recommend.go`, the annotator in `internal/app/store_enrich.go`.
- Worker 5 (Big Picture Store, downloads): new `frontend/src/bigpicture/Store*.svelte` and `frontend/src/lib/bpstore*.ts`, `BigPicture.svelte`, `Sections.svelte`, `Hints.svelte`, `nav.ts` (additions), `BPDownloads.svelte`, `frontend/src/desktop/Downloads.svelte`, `frontend/src/lib/downloads.ts`.
- Coordinator: everything else, including `types.ts`, `api*.ts`, bindings, `internal/store/sources/**` outside Codex's work, docs. Workers may add fixture data (not shapes) to the mocks.

## Acceptance checklist

Each maps to tests named for the behaviour, or to a recorded manual check.

- [x] **Part A (required):** ElAmigos parsing; previews and announcements apart from releases; inline patch claims don't make a base installer a "latest version"; hostile URLs; unsupported hosts open in the browser; missing fields; changed layouts; finite catalog; `.html` detail URLs; live check (`WL_ELAMIGOS_LIVE=1`, see Verification). KaOs excluded: HTTP 403 at planning and at integration, no verified mirror.
- [x] Part A in setup, filters, indexing status, wishlist matching, game pages, release notes. ElAmigos is opt-in (off by default), browser-only (no download, attach or install; "Opens in your browser"), announcements are "Announced" and never activity or availability, the first catalog pass is history.
- [x] Indexing: continuous idle backfill in five-page batches, two seconds between requests per source, `Retry-After`, progress saved after each page, resume after restart, finite catalogs, pause/resume, paused while playing, stops when a provider or the Store is turned off, partial failures keep cached results.
- [x] Completion times: library games from every source, Store off, shared corrections, ambiguous identities, unavailable and stale answers, no remaining-time arithmetic from playtime.
- [x] Wishlist import: SteamID64 validation, detected account, AppID deduplication, manual entries kept, private profile and provider errors, games without releases kept, immediate index match, bounded idle source searches, baselines and backfill suppression kept, unread activity kept.
- [x] Desktop Store: featured area, artwork shelves, tabs (new, popular, updated), compact rows with a selected-game preview, source, availability, version, reviews, completion time, source dates apart from the game's release date, recommendations with their basis and the popular fallback, installed games excluded; both themes, keyboard, 390 px (Store content width; the app window's minimum is 980 px with a fixed sidebar, so the whole window can't be 390 px wide).
- [x] Big Picture Store: Home, Browse/search, Wishlist, game details, source choice, install confirmation; controller layers, button prompts, on-screen keyboard; focus kept when returning; 720p and 1080p.
- [x] Downloads: queued jobs and the engine's error show when qBittorrent can't start (desktop and Big Picture).

## Verification

Run for this release only; earlier logs are history. Results on `claude/store-experience` after the last fix merge (`de11ea8`), Windows 11, 2026-10-03.

Reviews (Opus 5.5 high, headless, read-only): the integration review of `04e9c26` (1 high, 11 medium, 12 low) was fixed in `claude/store-fix-indexing` and `claude/store-fix-ui`; the follow-up review of `16e81ab` confirmed those and found N1–N8 (3 medium), fixed in `claude/store-fix-followup`. A third headless review of that last diff was stopped by Claude Code because the PC ran low on memory; the coordinator reviewed the diff instead and found nothing blocking (one low note: any `*url.Error` counts as transient, so a lasting client error on a release page is asked again at most every 5 minutes).

| Check | Result |
|---|---|
| `wails3 generate bindings -f '-tags production' -clean=true -ts -i` | 317 packages, 10 services, 143 methods; no changes |
| `cd frontend && npm run check` | 349 files, 0 errors, 0 warnings |
| `npm run test` | 18 files, 153 tests pass |
| `npm run build` | ok |
| `go vet ./...` | ok |
| `go test ./... -count=1` | all pass except `internal/pad` `TestVirtualGamepad`, `TestPlugVirtual`, `TestPlugVirtualTwice`: a physical DualSense is connected to this PC (they fail the same way on `main` 5cada50) |
| `go test -race ./internal/... -count=1` (WinLibs gcc 16.2, `CGO_ENABLED=1`) | same three `internal/pad` failures, no races |
| `go test -race -count=5 ./internal/store/... ./internal/app/` | all pass |
| `wails3 build` | ok, `bin/Seaglass.exe` |
| `git diff --check origin/main...HEAD` | clean |
| `WL_STORE_LIVE=1` `TestLiveDiscovery` | pass, 23 s: FitGirl 5 pages / 41 releases, DODI 5 pages / 48, ElAmigos 1 page / 3439; searches 7 and 4 results |
| `WL_STORE_LIVE=1` `TestLiveProviders` | pass, 12 s: Steam chart 100 games |
| `WL_STEAM_ID=76561198198086967` `TestLiveSteamWishlist` | pass, 20 games |
| `WL_ELAMIGOS_LIVE=1` `TestLiveElAmigos` | pass, 6 s: catalog 510556 bytes, 3439 entries; 3 release pages with sizes, languages and 4 browser references each |
| KaOs (`curl`) | HTTP 403: excluded |
| `node tools/harness/discovery.mjs` | 23 of 23 checks pass (index, search, keyboard, game page, reviews, HowLongToBeat, wishlist, install confirmation closed, nothing downloaded; both themes; 390 px without horizontal scrolling) |
| `node tools/harness/bpstore.mjs` (virtual DualSense) | 8 of 8 checks pass: Home, game page, install confirmation, focus kept on return, on-screen keyboard, tabs; shots at 1280×720 and 1920×1080 |
| `node tools/harness/store.mjs` (qBittorrent 5) | ok: feed, download, safety checks, install into a library folder, uninstall |
| `node tools/harness/tour.mjs --input=pad --layouts=deck --sizes=1280x720,1920x1080` | 0 stuck presses; the layout audit's notes are on library and settings screens this work doesn't change |
| Mock mode (Edge, Playwright) | desktop Store at 1280 px and 390 px content width in both themes, ElAmigos opt-in with browser-only and announced releases, Downloads with the engine down, Big Picture Store at 720p and 1080p including the empty state while a game runs; no page errors |

Harnesses ran with `--dev-data` (roaming data and, new in this work, local caches kept apart); `%APPDATA%\Seaglass` was byte-for-byte unchanged after every run.

Known limits: the desktop window is at least 980 px wide with a fixed sidebar, so "390 px" is the Store's content width (Downloads job actions are cut at that width); big picture can't attach a .torrent (desktop mode can); ElAmigos is browser-only and has no site search; KaOs is not included.
