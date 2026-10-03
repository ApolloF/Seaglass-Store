# Experimental store

An opt-in store, turned on per PC under *Settings → Experimental → Store*. While it's off, nothing of it runs or shows: no qBittorrent, no feed fetches, no source indexing, no art or review lookups. Games come from the repack sources Seaglass indexes on this PC (FitGirl and DODI, and ElAmigos when chosen; see [Automatic discovery](#automatic-discovery)) and from feeds the person adds by URL ([store-feed.md](store-feed.md)). The person is responsible for what they download and for being allowed to play it; Seaglass doesn't assert rights or authenticity for any release.

## Automatic discovery

*Settings → Experimental → Source discovery* (`internal/store/discovery`, `internal/app/store_discovery.go`).

- **Setup.** A person who turns the Store on for the first time gets both sources and *Browse repack sources* turned on, and indexing starts at once. Someone who used the Store before this version is asked once which sources to use; nothing is indexed until they choose, and their feeds, trust levels and reviewed offers stay as they were. The choice is per PC (`store.sources`, `store.sourceSetup`). ElAmigos is off until chosen: it lists its whole catalog on one page and offers its files through file hosts, so its releases open in the browser and are never downloaded, attached or installed by Seaglass; it has no site search. Its announcements show as *Announced* and never count as a release, availability or wishlist news.
- **Indexing.** Only public release metadata is read: no torrent peer, payload download or installer. The cached index shows at once at startup. The newest listings (FitGirl's RSS feed and the first listing page, catching up page by page after a long pause) are fetched again when six hours old, or on *Check for new releases*. Older listing pages are read continuously while the PC is idle, five per source per batch, until the source's confirmed end (a page past the last one); a finite catalog never asks for a second page, and when it is first indexed every row counts as history. Progress is saved after every page (a journal folded into the source file on save), survives restarts and crashes, and an interrupted page is retried; a source file that can't be read is indexed again from its newest page. One request per source is in flight, two seconds apart; `Retry-After` is honoured and failures back off exponentially (one minute doubling to an hour). Searches and release pages wait for the same `Retry-After`. Indexing pauses while a game runs and on *Pause indexing* (Store status bar or Settings, per PC), and stops when the Store or a source is turned off (only that source's work), or Seaglass closes.
- **Records.** Each article is kept by source and canonical URL with its claims, references, resolved torrent identities and provenance (first seen, backfill, last seen, changed, detailed). A failed fetch never drops cached games; an article that disappears is marked *no longer listed*, not deleted.
- **Games.** Releases join one game only through a trusted Steam identity (Seaglass's game database on an exact title, or a title that differs by an edition sold as the same game, the person's correction, or a feed's AppID) or an identical normalized title. Sequels, remasters, definitive editions and DLC stay apart. *Change Steam match* on a game's page (or *Not on Steam*) is kept across refreshes in `store-identity.json`. Feed games join the same games by key.
- **Search.** Matches from the index come at once; after 600 ms and two characters the chosen sources' own search and Steam's store search fill gaps (cached an hour each). A newer query cancels the older one. A failed provider says so and keeps the local matches. Steam games without a known source release show under *Other games, No known source release*, with wishlist and details but no install.
- **Home.** A featured area, Wishlist activity, recommendations from the genres of recently played library games and wishlisted games with the basis named (Steam's popular games when there is none), and tabs for New releases (source publication date), Popular (Steam's most-played chart among source-backed games; without the chart the tab says so instead of showing another ranking) and Recently updated, as compact rows with a preview of the selected game. Games installed in the library are left out of Featured and recommendations. Source publication dates are never shown as game release or build dates, and a changed article alone is never a newer version.
- **Game page.** Source-labelled releases with version, size, languages, publication date and availability (*installable*, *needs resolving*, *browser only*, *update only*, *details loading*, *no longer listed*), the comparable-version recommendation and update guards; Steam's overall and recent review summaries and paged review text with helpfulness, playtime and links; the Metacritic score and link Steam gives; HowLongToBeat Main Story, Main + Extras and Completionist times, with *Wrong game?* to choose the match (`internal/store/enrich`). Provider answers are cached (chart and review pages an hour, review summaries six hours, Metacritic and HowLongToBeat seven days); a failed provider shows the cached answer as stale, or *unavailable* with a link. Nothing is estimated. Only games on screen, opened or saved are looked up.
- **Wishlist.** Source-backed and Steam-only games can be saved (`%APPDATA%\Seaglass\wishlist.json`, this PC only). Saving sets a baseline; a first source release or a confirmed newer game version becomes unread activity in the Store until it is read. Releases found by backfill or old search results never count as news. No desktop notifications. *Import from Steam* reads a public Steam wishlist (`IWishlistService`) for the account found on this PC or a typed SteamID64 (individual accounts only): games are added by AppID in Steam's order up to the wishlist's limit, nothing saved is removed, and the result says how many were added, already saved, left out, available or being searched. Imported games without a release are searched on the chosen sources while the PC is idle, one game every two minutes, asking only the sources that haven't answered, and given up after three searches that missed a source.
- **Big picture.** The Store is a section of big picture with Home, Browse (search on the on-screen keyboard, source and availability filters), Wishlist, game pages and the install confirmation, all with the controller; going back returns focus to the game it came from. Attaching a .torrent file is desktop-only.
- **Installing.** Choosing a release reads its article when only a summary is known and tries the supported torrent-metadata resolver once ([private-catalog.md](private-catalog.md)), then opens the install confirmation with source, version, languages, size and warnings. Without a validated torrent there's no download button: *Release page* opens the article in the browser and *Attach .torrent file* validates a file got there. Downloads then go through the same checks as feed offers: English by default, optional language packs, the payload-scan setting, integrity checks and the update guard.

## How a game gets onto the PC

1. **Catalog.**
   - Feeds are fetched over HTTPS when added, every six hours after that, and on *Fetch now*. Each item is checked, and items that fail are left out.
   - Items are grouped into games, matched to the game database when the match is sure (`internal/store/catalog`).
   - Art and descriptions come from the library's own sources, for the games on screen first (`internal/app/store_art.go`).
2. **Choice.**
   - The game page lists every version, with one recommended (`catalog.Recommend`). A newest-version claim requires comparable game-version evidence. Recommendations also consider language, feed trust and prior blocks. Mixed/unknown version systems need review and cannot trigger automatic update replacement.
   - Versions with a checksum, or an installer that runs without questions, get a small bonus.
   - The install dialog asks for the version, the requested game language (English by default), the game's folder (new or empty) and whether to install straight away.
3. **Download** (`internal/torrent`, `internal/store/jobs`).
   - Seaglass runs the installed qBittorrent 5 hidden, as a sidecar with a profile of its own (`%LOCALAPPDATA%\Seaglass\store\qbittorrent`).
   - Its Web UI listens on a random localhost port, with a password made up at every start. It only starts while something needs it and stops after two idle minutes.
   - Downloads survive restarts, pause while a game runs (optional), stop when the disk won't hold them, and can all be held from the tray.
4. **Safety checks** (`internal/safety`), before anything in the download runs:
   - the installer's SHA-256 against the feed's
   - the Authenticode publisher
   - what's in the download: programs named like documents, scripts and shortcuts, zip bombs, an "installer" that isn't a program
   - a Microsoft Defender custom scan, with remediation off
   - optionally a VirusTotal lookup by hash with the person's own key; files are never uploaded

   The verdict is *clean*, *warn* or *block*. A blocked download installs only after the person types its name. *Try it in Windows Sandbox* opens the download read-only and offline when that feature is on.
5. **Install** (`internal/installer`).
   - Kinds handled: Inno Setup (`/VERYSILENT … /DIR= /LANG= /LOG=`), NSIS (`/S /D=`), Windows Installer (`msiexec /i … /qn`), archives (Windows' `tar`), and games that need no install (copied).
   - An installer that needs administrator rights gets Windows' prompt.
   - An installer Seaglass can't run silently shows its own questions.
   - The games folder becomes one of the library's folders, so the scanner adds the game; the download is deleted unless kept.
   - The uninstall command is taken from Windows' installed-apps list, or from the uninstaller Inno and NSIS leave behind.
6. **Updates.**
   - A newer version of a game the store installed shows as *Update*.
   - It installs over the old one in the same folder, and the older download is forgotten.

Big picture has a read-only Downloads screen in Quick access. Installing, pausing and removing are done in desktop mode.

## What the checks can't do

They catch what they know: a file that isn't what the feed listed, and what Defender or VirusTotal's engines recognise. They can't prove that a download is safe. Programs from unknown or untrusted feeds can do anything once installed. Repacked and modified games are often flagged, so a detection on one of them needs a careful look rather than an override by habit.

## Network settings

*Settings → Experimental → Network* maps onto qBittorrent's preferences (`internal/torrent/qbit/prefs.go`):

- binding to an interface and an address; while the interface is gone (a VPN that disconnected), qBittorrent sends nothing at all and Downloads says so
- the incoming port (0 picks a random one at each start)
- UPnP / NAT-PMP port forwarding
- a SOCKS5 or HTTP proxy for trackers, and optionally peers, with DNS through the proxy and the password stored with DPAPI
- encryption, DHT, PeX, LSD and anonymous mode
- speed limits, downloads at once, and when to stop sharing

## Data

| Where | What |
|---|---|
| `%APPDATA%\Seaglass\settings.json` (`store`) | feeds, folders, network, safety policy (this PC only, never synced) |
| `%APPDATA%\Seaglass\downloads.json` | downloads, their checks and what they installed |
| `%APPDATA%\Seaglass\secrets\store-*.bin` | proxy password and VirusTotal key (DPAPI) |
| `%APPDATA%\Seaglass\wishlist.json` | saved Store games and their activity (this PC only) |
| `%APPDATA%\Seaglass\store-identity.json` | the person's Steam and HowLongToBeat match corrections |
| `%LOCALAPPDATA%\Seaglass\store\` | feed copies, art index, install logs, Windows Sandbox configs, qBittorrent's profile |
| `%LOCALAPPDATA%\Seaglass\store\discovery\` | the source index and crawl progress (a cache: deleting it means indexing again) |
| `%LOCALAPPDATA%\Seaglass\store\enrich\` | Steam chart, reviews, Metacritic and HowLongToBeat answers (a cache) |

## Testing

- `go test ./internal/torrent/... ./internal/store/... ./internal/safety ./internal/installer ./internal/app`
- Live tests:
  - `WL_REAL_QBIT=1 go test -run RealSidecar -v ./internal/torrent/qbit` (the installed qBittorrent, a Creative Commons magnet added paused and removed)
  - `WL_REAL_DEFENDER=1 go test -run RealDefender -v ./internal/safety`
  - `WL_STORE_LIVE=1 go test -run LiveDiscovery -v ./internal/store/discovery` (one bounded pass per source and a source search)
  - `WL_STORE_LIVE=1 go test -run Live -v ./internal/store/enrich` (the chart, one game's reviews, Metacritic and HowLongToBeat)
  - `WL_STORE_LIVE=1 WL_STEAM_ID=<SteamID64> go test -run LiveSteamWishlist -v ./internal/store/enrich` (a public Steam wishlist)
  - `WL_ELAMIGOS_LIVE=1 go test -run LiveElAmigos -v ./internal/store/sources` (the ElAmigos catalog and three release pages)
- `tools/harness/store.mjs` runs the whole path in the real app against a feed and web seed served by the script itself.
- `tools/harness/discovery.mjs` drives discovery in the real app: indexing, search, a game page, the wishlist and the install confirmation (closed without downloading), in both themes and at 390 px.
- `tools/harness/bpstore.mjs` drives the Store in big picture with the virtual controller at 1280×720 and 1920×1080.
- Not yet covered by an automated run on a real installer: Inno Setup and NSIS silent installs (their command lines are unit-tested).

Release comparison, repack findings, payload scan settings and torrent/installer languages are documented in [store-release-selection.md](store-release-selection.md). Source discovery is documented in [private-catalog.md](private-catalog.md).
