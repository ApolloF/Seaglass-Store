# Store Edition: syncing and releasing

`ApolloF/Seaglass-Store` is Seaglass plus the experimental store ([experimental-store.md](experimental-store.md)). It shares Seaglass's history and keeps up with it by merging Seaglass's `main`. What makes it the edition is kept small, so those merges rarely conflict:

- `internal/edition`: its name, its repository (updates, releases, issues) and its version suffix
- `internal/update/keys.go`: its own release key
- `build/windows/info.json`: its product name
- the store's code and its few hooks in shared files (settings, core, services, `main.go`, the tray, the sidebar, settings and big picture screens)
- the README's top and *Intended use*, and this workflow and release setup

It keeps Seaglass's install folder, data folders and single-instance ID, so the two editions install over each other and share the library and settings.

## Getting Seaglass's work

`.github/workflows/sync.yml` runs every day at 04:17 UTC, and from *Actions → sync with Seaglass → Run workflow*:

1. It merges `ApolloF/Seaglass` `main` into this `main` and regenerates the Wails bindings.
2. It runs the frontend build, typecheck and tests, `go vet` and `go test`.
3. A clean merge with passing tests is pushed to `main`. One with conflicts or failing tests becomes a pull request (`sync/seaglass-<sha>`) to finish by hand: check out the branch, `git merge origin/main`, fix, push, merge.

GitHub doesn't let the workflow's token push changes to workflow files. When Seaglass changes its `.github/workflows`, merge by hand:

```bash
git fetch https://github.com/ApolloF/Seaglass.git main && git merge FETCH_HEAD
```

When Seaglass changes `.github/workflows/build.yml`, keep this repository's three additions in it:
- `HAS_RELEASE_KEY`
- the `-store.N` rule in *Version*
- the *Sign and publish* step

## Versions

`v<Seaglass version>-store.<n>`: `v1.9.0-store.1` is the first Store Edition release on Seaglass 1.9.0, `v1.9.0-store.2` the next store-only release, and `v1.10.0-store.1` the first after Seaglass 1.10.0 is merged in. The updater compares the store number after the version, and CI publishes these tags as full releases (other suffixes stay previews).

## Releasing

1. Write `docs/releases/vX.Y.Z-store.N.md` on `main`.
2. Tag and push from an up-to-date `main` whose CI passed:

   ```bash
   git tag vX.Y.Z-store.N && git push origin vX.Y.Z-store.N
   ```

3. CI builds and tests the tag, makes a draft release, then *Sign and publishes* it. It writes `SHA256SUMS` and `SHA256SUMS.sig` with the edition's release key from the `SEAGLASS_RELEASE_KEY` Actions secret, publishes the release as *latest*, and verifies the published signature.
4. `go run ./tools/release verify vX.Y.Z-store.N` checks it from any PC.

## The release key

- **Where it is:**
  - The edition has its own ed25519 key, made 2026-10-02; Seaglass's key isn't involved.
  - Its public half is in `internal/update/keys.go`.
  - The private half is DPAPI-encrypted on the maintainer's PC (`%APPDATA%\Seaglass-Store-release`, `go run ./tools/release pubkey`).
  - As the maintainer chose, it's also the repository's `SEAGLASS_RELEASE_KEY` Actions secret, so CI signs.
- **What that trades:** anyone who controls the GitHub account, the repository's Actions or its secrets can sign an update that Store Edition users install. Seaglass itself keeps its key off GitHub for exactly that reason.
- **Moving it off GitHub:**
  1. Delete the secret.
  2. CI then leaves releases as drafts.
  3. Sign and publish them on the maintainer's PC with `go run ./tools/release publish <tag>`.
- **Backup:** `go run ./tools/release backup <file>` (password-protected; keep it offline).
- **Putting it back into CI:** `go run ./tools/release ci-secret`. It hands the key to `gh` directly and never prints it.
