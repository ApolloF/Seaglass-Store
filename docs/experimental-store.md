# Experimental store

An opt-in store, turned on per PC under *Settings → Experimental → Store*. While it's off, nothing of it runs or shows: no qBittorrent, no feed fetches, no art lookups. Seaglass ships no catalogs. The person adds feeds by URL ([store-feed.md](store-feed.md)), and they're responsible for what those feeds offer and for being allowed to download and play it.

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
| `%LOCALAPPDATA%\Seaglass\store\` | feed copies, art index, install logs, Windows Sandbox configs, qBittorrent's profile |

## Testing

- `go test ./internal/torrent/... ./internal/store/... ./internal/safety ./internal/installer ./internal/app`
- Live tests:
  - `WL_REAL_QBIT=1 go test -run RealSidecar -v ./internal/torrent/qbit` (the installed qBittorrent, a Creative Commons magnet added paused and removed)
  - `WL_REAL_DEFENDER=1 go test -run RealDefender -v ./internal/safety`
- `tools/harness/store.mjs` runs the whole path in the real app against a feed and web seed served by the script itself.
- Not yet covered by an automated run on a real installer: Inno Setup and NSIS silent installs (their command lines are unit-tested).

Release comparison, repack findings, payload scan settings and torrent/installer languages are documented in [store-release-selection.md](store-release-selection.md). Source discovery is documented in [private-catalog.md](private-catalog.md).
