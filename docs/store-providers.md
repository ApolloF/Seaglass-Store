# Store source providers: the shared contract

The Store indexes release metadata from a small set of vetted providers. This page is the contract between the source layer (`internal/store/sources`) and everything that uses it (discovery scheduling, settings, app services, the interface). It was set up for adding providers beyond the first two.

## The registry

`internal/store/sources/registry.go` declares every provider once, as a `Provider`:

| Field | Meaning |
|---|---|
| `Source` (`ID`, `Name`, `Host`, `StartURL`) | identity, the only host its pages may come from, and where `tools/cataloglab` starts |
| `Discovery` | the Store indexes it and offers it in source setup |
| `DefaultOn` | a new Store user starts with it chosen. Only the two original providers. A provider added later stays off until the person chooses it |
| `Listing` | the first page of its newest-first catalog |
| `ListingPage` | listing page *n* (n ≥ 2) with `{n}`; empty for a catalog of one finite page |
| `Feed` | an RSS feed with the newest articles in full; empty when there is none |
| `Search` | its own site search with `{q}`; empty when it has none |
| `Torrents` | its release pages may carry torrent links Seaglass can validate. Without it, every release opens in the browser |
| `Notes` | verified limitations, as plain sentences the interface shows |

Read it through `Providers()` (discovery providers, display order), `DiscoveryIDs()`, `DefaultIDs()`, `Lookup(id)`, `Provider.ListingURL(n)` (false past a finite catalog's end), `Provider.Paged()` and `Provider.SearchURL(q)`. `PrivateSource` and `Source.SearchURL` read the registry too.

Users of the registry:
- `discovery.Sources`, `settings.DiscoverySources`: the discovery IDs. Settings keep only registered IDs in `store.sources`, so existing choices survive and unknown ones drop.
- `settings.StoreTurnedOn`: chooses `DefaultIDs()` for a new Store user.
- `discovery.Pass`: takes a `Provider`. It reads the feed when there is one, asks `ListingURL` for every page, and marks a finite catalog complete after its only page without requesting a second one.
- `discovery.SearchSource`: takes a `Provider`; a provider without `Search` isn't asked.
- `discovery.SourceStatus` (and `DiscoverySourceStatus` in `frontend/src/lib/types.ts`): carries `host`, `search`, `paged`, `torrents`, `defaultOn` and `notes`, so setup, filters and status are built from the registry instead of naming sources.

## Adding a provider

What the source side delivers for each provider:
1. A registry entry with verified URLs and `DefaultOn: false`.
2. Parsing in `sources.Parse` (dispatch on the provider ID) for its listing, search and detail pages: title, version, languages, sizes, dates when given, release links, and the split between installable releases, updates and announcements or previews. Detail URLs are kept as the provider writes them (for example `.html`).
3. Pagination that matches the catalog: `ListingPage` for numbered pages, or none for one finite page. `NextPage` must not invent `/page/N/` links.
4. The existing limits: the bounded HTTP client (one request per source, two seconds apart, `Retry-After`), `ValidateURL` on every page, `referenceURL` on every link, torrent metadata validation before anything is installable. Unsupported file hosts stay browser links; no new downloader.
5. Tests with fixtures: listings, details, previews, update and full-release separation, hostile URLs, unsupported hosts, missing fields and a changed layout. A live check gated by `WL_STORE_LIVE=1`.
6. A handoff: commits, behaviour, fixtures, exact checks and results, live observations, limitations, and anything the integration needs.

The Store side then adds the provider to setup, filters, indexing status, wishlist matching, game pages and the release notes; none of that names providers, so a correct registry entry is most of it.
