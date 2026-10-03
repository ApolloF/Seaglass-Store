# Codex source expansion handoff

Status: implemented and committed; integration verification is incomplete.
Do not mark the release source gate fully verified or publish while the
blockers below remain. No merge, push, tag or release was performed.

## Branch and integration

- Dedicated clone: `C:\Users\Florian\Documents\Coding Projects\Seaglass-Store`.
- Codex worktree: `C:\Users\Florian\Documents\Coding Projects\Seaglass-Store-Codex`.
- Branch: `codex/source-expansion`, based on remote main `5cada503`.
- Claude shared contract: `b972f4ae2a0351b23a4d7a71f0665fc3384ac422`, consumed as
  cherry-pick `ec5ea14`. Do not cherry-pick `ec5ea14` back onto Claude's branch.
- Codex implementation: `4bf8bb8` (parser, captured/synthetic fixtures,
  behavior/transport tests) then `dae2713` (registry, public Parse dispatch,
  finite pagination, discovery integration tests, provenance, CLI help).
- The handoff document has its own final commit, listed in `git log`.

From Claude's worktree, fetch these locally and cherry-pick only Codex's
commits in order:

```powershell
git fetch 'C:\Users\Florian\Documents\Coding Projects\Seaglass-Store' codex/source-expansion
git cherry-pick 4bf8bb8 dae2713
# Then cherry-pick the docs-only handoff commit shown by git log FETCH_HEAD.
```

The earlier parser commit intentionally did not touch the registry while
Claude prepared the contract. The final implementation uses the public
source/discovery interfaces; no parallel source contract was introduced.
The session cannot select/confirm the requested primary model
GPT-6.1-sol/high; this limitation was reported before implementation.

## Delivered behavior and files

`internal/store/sources/elamigos.go` parses the actual finite homepage,
dated news rows and alphabetic index, plus `/data/*.html` detail pages.
Poster-only links and support/aggregation links are ignored. URLs are
validated on the selected origin. Repeated rows keep the first (newest
news-batch) occurrence. The alphabetic index does not inherit a news date.
The catalog has its own 5000-entry bound; the shared 4 MiB/token/nesting
bounds and the existing WordPress 500-entry limit remain unchanged.

The registry adds `elamigos`: discovery on, default off, finite listing,
no RSS, no server search and no verified torrent transport. FitGirl and
DODI remain the existing defaults and keep their URLs/capabilities. Settings
and cache persistence are provided by Claude's contract, without migration
or resetting preferences. No KaOs provider is registered.

`parse.go` dispatches to the adapter after existing document and challenge
validation. `discover.go` stops pagination for a registry-declared finite
catalog. `tools/cataloglab/main.go` describes registry IDs in help.

All catalog rows are summaries. Versions are deliberately left unknown
until detail retrieval: listing `[Update ...]` and `+[Update ...]` claims
cannot promote a base installer to a later version. Details retain only
the version stated in the base `ElAmigos release` paragraph. Later patch
headings become separate warnings; their distinct containers cannot become
base-release links, metadata or version claims. Standalone patch fixtures
are classified `update`. Explicit coming-soon/announcement detail fixtures
are `preview`, with no available references or transports.

Download size uses `Upload size / to download`, with the main heading's
size as a fallback. ISO size, RAR part size and installed size are not
substituted for download size. Language and audio claims are retained.
Original game release years are removed from the title and never become
source publication dates. Catalog batch dates and the date accompanying
the included version are source `UpdatedAt` claims, with explanatory
warnings; `PublishedAt` stays unknown when not supplied.

DDOWNLOAD/RAPIDGATOR container links are `Reference{Kind: "download",
State: "manual-required"}` with a browser-handoff reason. No file hosts are
requested, new downloader introduced, magnets synthesized, or payloads
downloaded/executed. The ordinary bounded HTTP client, origin/URL checks,
torrent validator and installer pipeline remain unchanged. Browser-only
links do not enter torrent resolution. Provider selection, document hashes
and valid torrent metadata do not establish payload safety.

Tests live beside the adapter: `elamigos_test.go`,
`elamigos_discovery_test.go` and `elamigos_live_test.go`.
`testdata/elamigos/README.md` identifies original URLs, byte counts,
SHA-256 hashes, captured versus synthetic fixtures and newline normalization.

## Current checks, 2026-10-03 Europe/Amsterdam

These checks were run on this Codex branch with Claude's source contract,
not on the final integrated product/release.

| Exact command | Result |
|---|---|
| `npm ci` in frontend | PASS, 79 packages, 0 reported vulnerabilities; npm warns about existing `minimum-release-age` config |
| `go test ./internal/store/sources ./internal/store/discovery ./tools/cataloglab ./internal/settings -count=1` | PASS, all four packages, including external discovery fixture integration |
| `go vet ./...` | PASS after the final test correction |
| `go test ./internal/store/sources -run '^$' -fuzz '^FuzzElAmigosParse$' -fuzztime=30s -parallel=2` | PASS, 153299 executions, 7 newly interesting inputs; no panic/invariant failure |
| `wails3 generate bindings -f '-tags production' -clean=true -ts -i` | PASS, 317 packages, 10 services, 136 methods; no additional tracked binding diff |
| frontend `npm run check` | PASS, 0 errors and 0 warnings |
| frontend `npm run test` | PASS, 13 files / 87 tests |
| frontend `npm run build` | PASS, Vite 8.3.1, 322 modules |
| `wails3 build` | PASS, `bin/Seaglass.exe`, Windows amd64, pinned Wails beta.26 |
| `git diff --check` | PASS |
| `go test ./...` | FAIL: app's `TestDiscoveryStatusFollowsSourceChoice` still hardcodes two providers; pad's `TestVirtualGamepad`, `TestPlugVirtual`, `TestPlugVirtualTwice` fail with a connected physical PS5/DualSense controller |
| `go test -race ./internal/...` | BLOCKED: `go: -race requires cgo; enable cgo by setting CGO_ENABLED=1` |
| `$env:CGO_ENABLED='1'; go test -race ./internal/...` | BLOCKED: `cgo: C compiler "gcc" not found`; no production configuration was changed |
| `$env:WL_ELAMIGOS_LIVE='1'; go test ./internal/store/sources -run '^TestLiveElAmigos$' -count=1 -v` | FAIL on repeated attempts: existing 20-second client timeout on homepage; trace shows the first origin IP timing out, then the second TCP connection succeeding without completing TLS inside the request budget |

An early verification command mistakenly ran Go package paths from
`frontend`; it failed with directory-not-found errors. It was rerun from
the repository root. An external integration test initially used incorrect
Release field types; that test was corrected and the package/vet checks
above passed afterward. Neither failure is counted as a passing check.

Captured full homepage replay:

```powershell
go run ./tools/cataloglab --private-sources --source elamigos --input "$env:TEMP/seaglass-source-assessment/elamigos.html" --url https://elamigos.site/
```

PASS: 3439 unique entries, all summaries, zero asserted installer versions;
document hash `b76541b58e907c8b142f9be9531c8b10c804beb1fe59b9d8c3241ea7ea2ed1e7`.
The captured full homepage is temporary evidence, not a required fixture.
Committed reduced and generated fixtures are sufficient for offline tests.

## Public provider observations

Normal unauthenticated `curl.exe` HTTPS GETs initially returned:

| URL | Status / body bytes |
|---|---|
| https://elamigos.site/ | 200 / 510556 |
| https://elamigos.site/data/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html | 200 / 3984 |
| https://elamigos.site/data/The_Blood_of_Dawnwalker_Eclipse_Edition_MULTi15_-_ElAmigos.html | 200 / 3803 |
| https://elamigos.site/data/Red_Dead_Redemption_2_MULTi13__ElAmigos_-_KnPzu8CD.html | 200 / 4164 |
| https://elamigos.site/data/Resonance_A_Plague_Tale_Legacy_MULTi17_-_ElAmigos.html | 200 / 3151 |
| https://elamigos.site/data/Mortal_Shell_II_Devout_Edition_MULTi15_-_ElAmigos.html | 200 / 3841 |
| https://kaoskrew.org/ | 403 / 5232 |
| https://kaoskrew.org/viewforum.php?f=12 | 403 / 5322 |

The ElAmigos origin labels itself "ElAmigos official site"; this is the
inspected origin, with no mirror substitution. Its claim is not independent
proof of authorship or safety. Search is inline JavaScript filtering a finite
catalog, not an HTTP search endpoint. No page-2 endpoint was requested.
There were 3593 release rows before deduplication. The sampled details use
h2 title/year/size, h3 release/metadata and labeled file-host sections.
The sampled pages contain available source-link claims rather than previews;
the preview fixture is explicitly synthetic.

Control's base paragraph claims 1.3.3, while separate patch headings claim
1.3.3 → 1.4.0 → 1.4.1. Red Dead's base paragraph claims 1311.23, while its
later patch heading claims 1491.50. The parser retains base versions.
Shared filecrypt/keeplinks containers are only source links; host availability
and their contents were not inspected.

Later curl requests also timed out, including a request using
`--noproxy '*'`. No proxy environment variables were present. Requests to
both DNS-returned public origin IPs timed out; these were diagnostic origin
checks, not alternate provider URLs or mirrors. Disabling TLS ML-KEM for one
diagnostic test did not resolve the timeout, and no TLS/network policy was
changed in the implementation. Current reliable live access through the
application client remains unverified and must be resolved before release.
An independent later `curl.exe --max-time 15 https://github.com/` check
returned 200 (13.36 seconds); the subsequent normal 20-second ElAmigos
request timed out before TCP connection. This is not a general proof that
all networking was unavailable.

## Claude integration requirements and blockers

1. Cherry-pick the two Codex implementation commits after the shared
   contract. Keep the source deliverable on the release acceptance checklist
   and in release notes; source implementation is present, live verification
   is not yet passing.
2. Update the app status test's `len(st.Sources) != 2` assumption to derive
   the registered provider count, and assert ElAmigos is initially disabled.
   Codex has not edited Claude-owned app files/tests.
3. Build source setup/filters/status from the registry and mirror the new
   disabled provider in the mock gateway. `Torrents: false`, `Search: ""`
   and `Paged: false` are purposeful. No site search should be scheduled for
   ElAmigos; local indexed-title search and wishlist matching work immediately
   after its catalog is indexed. Preserve existing FitGirl/DODI choices/caches.
4. Map `ReleaseKind: "preview"` explicitly to an unavailable/announcement
   display and prevent install/attach flows from treating it as a standalone
   release. The baseline discovery `availability` function only recognizes
   `update` and otherwise falls back to manual browser availability. Likewise
   honor browser-only provider capability in details/install actions. Preserve
   raw claims/warnings and never label the separately listed patch as included.
5. The catalog mixes recent news and historical alphabetic rows. Its first
   finite page must not create historical wishlist activity. The current
   shared crawler merges page 1 with `backfill: false`; consider undated
   alphabetic rows historical and retain the existing wishlist baseline and
   historical-suppression rules. This belongs to Claude's scheduling and
   persistence integration, not a guessed source publication date.
6. Resolve the current bounded-client live timeout and rerun
   `TestLiveElAmigos` without transport/security relaxation or a mirror.
   It performs at most four metadata GETs with the existing two-second pacing,
   normal origin validation, a 90-second outer deadline, and no host/payload
   traffic. KaOs stays excluded unless normal public access is reverified.
7. Obtain a working race-test C toolchain / CGO test environment, and run
   the whole suite without the connected physical controller interfering
   with virtual-pad tests. Do not alter production's no-cgo configuration.
8. Harnesses were NOT run: the installed real Seaglass was already running,
   and this shared-contract baseline's `platform.CacheDir` still points
   discovery/enrichment at real local user caches even under `--dev-data`.
   Windows known-folder lookup means setting APPDATA/LOCALAPPDATA alone is
   insufficient. Claude must isolate roaming and local data before desktop,
   controller and controlled-installation harnesses. Codex's fixture discovery
   integration uses separate `t.TempDir` roaming/local paths and touches none
   of the real application data. Mock theme/390px and Big Picture checks remain
   Claude's final integrated UI acceptance work.

This handoff is implementation evidence, not release acceptance. Re-run the
required merged-main checks and isolated Windows harnesses after integration.
