# Store feed format (schema 1)

The experimental store (*Settings → Experimental → Store*) shows the games in feeds the person adds by URL. Seaglass ships no feeds. A feed is one JSON document served over HTTPS (plain HTTP only from `localhost`, `127.0.0.1` or `::1`, for testing). Seaglass fetches each feed when it's added, every six hours after that, and on *Fetch now*. It sends `If-None-Match` when the feed gave an `ETag`.

```json
{
  "schema": 1,
  "name": "Example catalog",
  "homepage": "https://example.org",
  "items": [
    {
      "title": "Example Game",
      "version": "v1.4.2",
      "buildDate": "2026-09-12",
      "sizeBytes": 6657199308,
      "installedSizeBytes": 14495514624,
      "magnet": "magnet:?xt=urn:btih:<40 hex characters>&dn=Example+Game",
      "languages": ["English", "German", "French"],
      "platform": "windows",
      "installerType": "inno",
      "sha256": "<64 hex characters>",
      "steamAppId": 0,
      "notes": "Includes the first expansion."
    }
  ]
}
```

## Fields

| Field | Required | Meaning |
|---|---|---|
| `schema` | yes | Must be `1`. Seaglass rejects feeds with any other value. |
| `name` | yes | Shown in Settings and next to each version (up to 80 characters). |
| `homepage` | no | An HTTPS link. |
| `items` | yes | Up to 50,000 items. |

Each item is one downloadable version of a game.

| Field | Required | Meaning |
|---|---|---|
| `title` | yes | The game's name without version or edition noise (up to 200 characters). Seaglass matches it to a known game where it can. |
| `magnet` / `torrentUrl` | one of them | A magnet link with a BitTorrent v1 (hex or base32) or v2 info hash, or the HTTPS URL of a `.torrent` file. |
| `version` | no | Compared by its numbers: `v1.10` is newer than `v1.9`, and `Build 15302` is newer than `Build 9876`. Anything after ` +` or `(` doesn't count. |
| `buildDate` | no | `YYYY-MM-DD`. Breaks ties between equal versions and sorts *Recently updated*. |
| `sizeBytes` | no | Size of the download. Seaglass checks free space before it starts. |
| `installedSizeBytes` | no | Space the game needs once installed. |
| `languages` | no | Language names as the installer offers them. |
| `platform` | no | `windows` (the default). Seaglass leaves out items for other platforms. |
| `installerType` | no | One of `inno`, `nsis`, `msi`, `archive` or `portable`. |
| `sha256` | no | SHA-256 of the main installer. It's checked after downloading, before anything runs. |
| `steamAppId` | no | Links the item to a Steam game for art and descriptions, and groups it with other feeds' items for the same game. |
| `notes` | no | Shown with the version (up to 2,000 characters). |

## Checks

Seaglass checks every item and leaves out the ones that fail. Settings shows how many were left out, and the rest of the feed still loads. An item fails when:

- its title is missing;
- it has neither a magnet link nor a torrent URL, or both;
- its magnet link has no valid info hash;
- its torrent URL isn't HTTPS;
- its date is malformed;
- its size is impossible;
- its platform isn't Windows;
- its installer type is unknown;
- its `sha256` is malformed.

Control characters are removed and long text is cut. A whole feed fails when:

- it isn't valid JSON;
- it is larger than 32 MB;
- it has another schema;
- it has no name;
- it has more than 50,000 items.

A feed that is down or fails these checks keeps its last good copy.
