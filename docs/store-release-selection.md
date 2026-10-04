# Release selection

Based on ApolloF/Seaglass-Store's Store Edition. The Go module path stays github.com/ApolloF/Seaglass for existing imports and bindings.

## Comparing releases

Game versions, build numbers and ISO game-build dates are compared only within compatible systems. Decimal components are compared numerically without integer overflow; trailing zero components are equivalent. DLC counts and parenthesized release notes do not increase the version. Slash-separated versions, unknown/prerelease labels that cannot be ordered reliably and conflicting version/build evidence stay unranked. Publication dates and repack revisions do not prove a game update. Deterministic display sorting does not establish comparability.

Recommendations claim newest only when all offers can be compared. Updates consider each offer against the installed version and recommend confirmed newer candidates only. DownloadOffer refuses to replace an installation with an equal, older or incomparable release. Use a separate installation to review such a release.

## Payload scans and false positives

Scan downloaded payloads defaults on and stays per PC. Off skips the custom Defender scan and optional VirusTotal hash lookup, records payloadSkipped, and displays Scan skipped in Downloads. It does not alter Windows antivirus settings. SHA-256 comparisons, signature information and structural/file checks still run. Checksum mismatches still block installation. Recheck applies the current setting to an existing download.

Explicit HackTool/PUA/PUP/Riskware classifications produce review warnings rather than hard blocks. They are not declared safe or automatically installed. Mixed malware/tool results and unknown threat names retain BlockDetections. VirusTotal tool-only classification requires a label for every flagged engine. No source name or hash is allowlisted. High archive compression warns because repacks can contain compressible data; declared expansion beyond 2 TB remains blocked.

## Choosing languages

English is the requested game-language default. Before downloading, choose among an offer's known game languages; disable automatic installation to choose after torrent/installer metadata arrives. In Downloads, Languages lists actual recognized optional torrent packs and, when available, Inno installer language names. Game packs and installer UI language are separate choices.

Only explicit optional/selective .bin/.doi/.pak/.arc/.zip/.7z language files can be deselected. Core files, executable helpers, ambiguous multi-language packs and unknown files retain their priorities. If an English optional pack cannot be confirmed, all packs are retained. All language packs restores every recognized pack. A previously skipped incomplete pack pauses the job for Resume, clears its safety report, and requires completion plus a fresh check before installation. Choices persist in downloads.json. Saving choices turns off automatic installation for that job.

Inno enumeration uses innoextract.exe from PATH when installed; no tool is downloaded automatically. Without it, the UI explains the limitation and Ask in the installer shows the installer's language/component controls. English installer UI is preferred when metadata lists it; otherwise its default is used. No game language is guessed to be an installer language/component ID. NSIS/MSI component choices use the interactive installer, with no invented component arguments.

## Verification

Tests cover incompatible release systems, zero-equivalent versions, large numbers, update replacement guards, source opt-in/review/cache restoration, skipped scanners with preserved integrity checks, tool-only versus mixed malware, optional pack priorities, unavailable languages, pending-pack completion and interactive arguments. Existing frontend tests and Svelte check remain required; service bindings are regenerated.
