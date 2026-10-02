# Private catalog sources

The adapters live in `internal/store/sources` in ApolloF/Seaglass-Store. `tools/cataloglab` is a CLI in the main module. In the desktop app, both Experimental Store and Browse repack sources must be enabled on this PC. Private describes this opt-in testing scope, not secrecy or access control.

## Sources and discovery

FitGirl uses its official RSS, article listings and WordPress search. Direct article/RSS magnets are normalized and deduplicated by info hash. DODI uses HTML listings, WordPress search and individual articles; its inspected RSS was malformed. No third-party feed, account or CAPTCHA solver is required for discovery. Availability of historic mirrors is not guaranteed.

The optional resolver supports ordinary File-Me free-download forms and bounded torrent metadata validation. Up-4ever can require Turnstile and is left for manual browser resolution. The original inspected SwiftUploads route returned 403. The 1337x fixture adapter remains exploratory and CLI-only.

## Desktop integration

The Store indexes FitGirl and DODI automatically on this PC ([experimental-store.md](experimental-store.md#automatic-discovery)): a persistent, paced crawler of listing pages and FitGirl's RSS feed, the sources' own search, and release details on demand. Indexing reads metadata only and never starts peers or downloads. File-host resolution runs only when the person chooses a release to install, at most one mirror per attempt. Unresolved, CAPTCHA, rate-limited, blocked and manual states stay visible. *Attach .torrent file* opens a local file picker and validates metadata without extracting or executing a payload.

Discovered releases no longer need a separate review step to be offered: a release with a validated torrent opens the install confirmation, which shows the source, version, languages, size and warnings. Summary-only releases are read first; update-only entries cannot become standalone installs. The queued offer passes the feed validator, and its notes retain the source page, document checksum, language claim and warnings. Publication dates are not converted to game build dates. Offers reviewed with the earlier flow stay in the catalog while *Browse repack sources* is on; the earlier preview APIs remain for compatibility.

The CLI and app share parsers, network checks and metadata validation. [store-release-selection.md](store-release-selection.md) describes comparisons, scan controls and language choices.

## CLI

From the repository root:

```powershell
go run ./tools/cataloglab --private-sources --source fitgirl --pages 3
go run ./tools/cataloglab --private-sources --source fitgirl --search witcher --follow-details 10
go run ./tools/cataloglab --private-sources --source dodi --pages 3
go run ./tools/cataloglab --private-sources --source dodi --search witcher --follow-details 10 --resolve-torrents 1
go run ./tools/cataloglab --private-sources --source dodi --url https://dodi-repacks.site/lies-of-p/ --resolve-torrents 1
# Offline HTML parsing makes no requests:
go run ./tools/cataloglab --private-sources --source dodi --input C:\path\article.html --url https://dodi-repacks.site/example/
# Attach manually obtained metadata to exactly one selected release:
go run ./tools/cataloglab --private-sources --source dodi --url https://dodi-repacks.site/example/ --torrent-file C:\path\example.torrent
```

JSON goes to stdout, failures to stderr. The CLI writes no files and starts no payload, peer connection, tracker request or installer. Private adapters require the explicit flag on every invocation. Source-site search differs from the optional local --query filter; search terms become the local filter unless a query is supplied.

CLI limits: 1-10 pages (default 1), 0-20 detail enrichments (default 5), 0-3 file-host attempts (default 0). An attempt makes an initial GET and at most two ordinary form submissions. Offline mode rejects network search, pagination and resolution. Desktop discovery uses one page, at most five enrichments, at most one separately enabled resolution and a three-minute deadline.

## Metadata and provenance

The preview schema is seaglass.catalog-preview/v1, distinct from the downloadable feed contract. It retains incomplete entries, source/page IDs, raw/cleaned titles, document hashes, publication/update dates, opaque version claims, tentative exact-title groups and review warnings. Documents record listing/detail evidence. collectedAt is snapshot creation time even offline, not a claimed historic retrieval time.

Every entry/group requires review. Matching normalized titles is a browsing hint, not identity proof. Release qualifiers are separated conservatively and edition names retained. Multiple version claims such as v1.31/v1.32 stay intact. Patch-from/update-only titles are flagged as update. Preview publication order is not game-version order.

Raw size claims remain available. Decimal and binary units differ. From is a lower bound. Recognized selective-download qualifiers are retained. Slash-separated variants become sizeOptionsBytes; no exact size is guessed until a real variant is selected. Split text/audio claims remain text; MULTi12 is not expanded into guessed language names.

A magnet must contain exactly one valid v1 btih in hex or base32. The preview retains normalized identity/display name and strips trackers, web seeds and other network-bearing parameters. The engine uses the person's network settings and DHT/peer configuration. Distinct alternate identities are retained. Reviewed offers keep source provenance, but a matching hash or size does not establish publisher authenticity, game identity, peer availability or installer safety.

## Network and file-host boundaries

Source requests stay on the selected exact HTTPS origin: fitgirl-repacks.site, dodi-repacks.site, or the CLI-only 1337x.to. Credentials, fragments, custom ports, proxy environment variables and browser sessions are not used. Redirects are bounded and revalidated. New connections resolve the hostname, reject the entire answer if any address is blocked/non-public, and pin a vetted address while TLS checks the original hostname.

HTTP mirrors remain unresolved. The article paragraph distinguishes torrent mirrors from full-payload host links; only torrent references can reach the resolver. File-Me HTTP links are upgraded to HTTPS without an HTTP request. Reviewed file-ID paths on file-me.top and up-4ever.net/www.up-4ever.net are supported. Only numeric svrN.file-me.top CDN hosts with a .torrent path are accepted for the observed metadata redirect. Other shorteners, hosts and executable endpoints remain unresolved.

Host cookies are ephemeral, public-suffix checked and separate from source clients. Forms are not cached. The resolver accepts reviewed fields only, requires the same file ID and same-origin action, uses the ordinary free-download choice, honors displayed waits up to 60 seconds and invents no challenge tokens. Real Turnstile/reCAPTCHA/hCaptcha widgets and cooldowns stop the attempt. No premium account, paid API or CAPTCHA service is configured.

Requests start at least two seconds apart per client and time out after 20 seconds. CLI cancellation is capped at ten minutes. Source bodies are capped at 4 MiB after decompression. HTML limits: 100000 tokens, 128 nesting levels before/after repair, 64 unclosed formatting elements, 500 entries per document and 1024 bytes per required title. Snapshots are capped at 5000 entries. References/transports have per-entry limits. Session ETags are capped at 25 documents and preserve final URLs for relative-link resolution.

Torrent metadata is capped at 2 MiB. Bencoding is canonical, depth/node bounded and rejects duplicate dictionary keys. Supported v1 single/multi-file layouts require nonnegative lengths, safe Windows path components, unique case-insensitive paths, no symlinks, valid piece tables and bounded total size. The info hash uses the original info bytes. Metadata SHA-256, torrent name and declared payload size are retained. V2/hybrid metadata is explicitly unsupported. Metadata validation does not scan or execute payloads. HTTP safeguards do not constrain peer traffic; the engine's interface binding implements that boundary.

## Verification

```powershell
go test ./internal/store/sources ./tools/cataloglab -count=1
go vet ./internal/store/sources ./tools/cataloglab
go build ./tools/cataloglab
go test ./internal/store/sources -fuzz '^FuzzParse$' -fuzztime=30s
go test ./internal/store/sources -fuzz '^FuzzTorrentMetadata$' -fuzztime=30s
$env:WL_CATALOG_LIVE='1'
go test ./internal/store/sources -run '^TestLiveSources$' -count=1 -v
$env:WL_CATALOG_HOSTS='1'
go test ./internal/store/sources -run '^TestLiveDODIHostChallenge$' -count=1 -v
```

The original prototype audit covered 33 documents, recent/older listings, searches, empty results and 20 release pages. Original File-Me resolution succeeded for two distinct DODI games; this is historical evidence, not a guarantee of current mirrors. Fixture tests cover URL/DNS/redirect validation, forms, cookies, cancellation, metadata bounds, paths, partial pagination, empty searches and CAPTCHA detection. The Store integration adds opt-in, review/cache, update and language workflow tests. See the current verification report for checks actually run on this integration.
