<script lang="ts">
  // Picking a release: prepare it (the Go side fetches the article and tries
  // to resolve torrent metadata; it never downloads a game), then hand over to
  // the install confirmation, or say why it can't be installed yet.
  import { onMount } from "svelte";
  import { api } from "../../lib/api";
  import { lib } from "../../lib/store.svelte";
  import { notReadyText } from "../../lib/storefront";
  import type { CatalogEntry, PreparedRelease, Release } from "../../lib/types";
  import InstallDialog from "../InstallDialog.svelte";
  import Modal from "./Modal.svelte";

  let { gameKey, release, update = false, onclose }: { gameKey: string; release: Release; update?: boolean; onclose: () => void } = $props();

  let prepared = $state<PreparedRelease | null>(null);
  let feedEntry = $state<CatalogEntry | null>(null);
  let busy = $state(false);
  let closed = false;

  async function prepare() {
    busy = true;
    if (release.origin === "feed" && release.feedKey) {
      // A feed's offer is already validated: the existing path, unchanged.
      const e = await lib.run(() => api.store.catalogEntry(release.feedKey!));
      busy = false;
      if (closed) return;
      if (e) feedEntry = e;
      else onclose();
      return;
    }
    const p = await lib.run(() => api.store.discovery.prepareRelease(gameKey, release.id));
    busy = false;
    if (closed) return;
    if (p) prepared = p;
    else onclose();
  }
  onMount(() => {
    void prepare();
    return () => (closed = true);
  });

  async function attach() {
    busy = true;
    const p = await lib.run(() => api.store.discovery.attachTorrent(gameKey, release.id));
    busy = false;
    if (closed) return;
    if (p) prepared = p; // ready: the confirmation opens; otherwise the reason shows again
  }
  const openPage = () => lib.run(() => api.store.discovery.openRelease(gameKey, release.id));
  const notReadyTitle: Partial<Record<PreparedRelease["state"], string>> = {
    "update-only": "This is an update",
    preview: "Announced only",
    browser: "Opens in your browser",
  };
  const feedOffer = $derived(feedEntry ? Math.max(0, release.feedOffer) : 0);
</script>

{#if feedEntry}
  <InstallDialog entry={feedEntry} offer={feedOffer} {update} {onclose} />
{:else if prepared?.state === "ready" && prepared.ready && prepared.offers.length}
  <InstallDialog {prepared} {update} {onclose} />
{:else if prepared?.state === "unresolved"}
  <Modal label={`${release.title}: not ready yet`} {onclose}>
    <p class="why">{prepared.reason || "Seaglass couldn't get this release's torrent file yet."}</p>
    {#each prepared.warnings as w, i (i)}<p class="sf-muted">{w}</p>{/each}
    <p class="sf-muted">Seaglass checks an attached .torrent file before anything is downloaded.</p>
    <div class="actions">
      <button type="button" class="sf-btn" onclick={openPage}>Open release page</button>
      <button type="button" class="sf-primary" disabled={busy} onclick={attach}>{busy ? "Checking the file…" : "Attach .torrent file"}</button>
    </div>
  </Modal>
{:else if prepared}
  <Modal label={notReadyTitle[prepared.state] ?? "Not available"} {onclose}>
    <p class="why">{notReadyText(prepared)}</p>
    <div class="actions">
      {#if prepared.state === "browser" && release.pageUrl}<button type="button" class="sf-btn" onclick={openPage}>Open release page</button>{/if}
      <button type="button" class="sf-btn" onclick={onclose}>Close</button>
    </div>
  </Modal>
{:else}
  <Modal label="Checking the release…" {onclose}>
    <p class="why" aria-live="polite">Seaglass is reading the release page. This can take a few seconds.</p>
    <div class="actions">
      <button type="button" class="sf-btn" onclick={onclose}>Cancel</button>
    </div>
  </Modal>
{/if}

<style>
  .why {
    margin: 0;
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  .sf-muted {
    margin: 0;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 10px;
    margin-top: 4px;
  }
</style>
