# ElAmigos fixtures

Captured with normal public HTTPS GETs on 2026-10-03 (Europe/Amsterdam).
The detail files are complete responses. Git normalizes their CRLF newlines;
the hashes below describe the original captured bytes, before normalization.

| Fixture | Origin URL | Original bytes | Original SHA-256 |
|---|---|---:|---|
| `base-with-patches.html` | https://elamigos.site/data/Control_Resonant_Deluxe_Edition_MULTi15_-_ElAmigos.html | 3984 | `ecb6d1080e428b961a1df8a355694aeddbd389c586d69f05e4404e0ae3584339` |
| `older-release.html` | https://elamigos.site/data/Red_Dead_Redemption_2_MULTi13__ElAmigos_-_KnPzu8CD.html | 4164 | `ffe9084d3647176f068720db0077ce23d252253f3f5abbf3c5aed75f87e60973` |
| `base-release.html` | https://elamigos.site/data/Resonance_A_Plague_Tale_Legacy_MULTi17_-_ElAmigos.html | 3151 | `8be414fc86ff2ffd96994ec1c0999c6c4a3d272289c466d58631a159483c074e` |

`catalog.html` is a reduced fixture assembled from observed news headings,
alphabetic index rows, duplicate links and poster/support links. The original
homepage was 510556 bytes, SHA-256
`b76541b58e907c8b142f9be9531c8b10c804beb1fe59b9d8c3241ea7ea2ed1e7`.
The large catalog bound is tested with generated 3500- and 5001-row documents.

`preview.html` is synthetic. No preview format was observed in the sampled
live detail pages; it tests explicit announcement/coming-soon labels and
ensures that placeholder containers cannot become available releases.
Other synthetic cases cover missing optional metadata, standalone patches,
unsafe URLs, changed layouts and patch containers distinct from base links.

Only metadata was read. File-host containers, archives and installers were
not requested. These fixtures establish parser behavior, not payload safety,
authorship of releases, Steam identity, or file-host availability.
