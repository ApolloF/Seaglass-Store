<script lang="ts">
  import { api } from "../lib/api";
  import { lib } from "../lib/store.svelte";
  import InstallDialog from "./InstallDialog.svelte";
  import type { SourceRelease, CatalogEntry } from "../lib/types";

  let source = $state("fitgirl");
  let query = $state("");
  let resolve = $state(false);
  let busy = $state(false);
  let results = $state<SourceRelease[]>([]);
  let warnings = $state<string[]>([]);
  let selected = $state<CatalogEntry | null>(null);
  let pickedOffer = $state(0);
  let choices = $state<Record<string, number>>({});
  let confirmed = $state<Record<string, boolean>>({});
  const enabled = $derived(lib.settings?.experimentalStore && lib.settings.store.privateSources);
  $effect(() => { if (!enabled) { results = []; selected = null; } });

  async function search(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    results = []; confirmed = {}; choices = {}; warnings = [];
    const snapshot = await lib.run(() => api.store.discoverReleases(source, query, resolve));
    busy = false;
    if (snapshot && enabled) { results = snapshot.entries; warnings = snapshot.warnings ?? []; }
  }
  async function attach(entry: SourceRelease) {
    busy = true;
    const next = await lib.run(() => api.store.attachReleaseTorrent(entry.sourceId, entry.id));
    busy = false;
    if (next) results = results.map((e) => e.id === next.id ? next : e);
  }
  async function review(entry: SourceRelease) {
    busy = true;
    const key = await lib.run(() => api.store.reviewRelease(entry.sourceId, entry.id, choices[entry.id] ?? 0));
    if (key) {
      selected = await lib.run(() => api.store.catalogEntry(key)) ?? null;
      pickedOffer = Math.max(0, selected?.offers.findIndex((o) => o.magnet === entry.transports[choices[entry.id] ?? 0]?.uri) ?? 0);
    }
    busy = false;
  }
</script>

<div class="browser">
  <form onsubmit={search}>
    <label>Source<select bind:value={source} disabled={busy}><option value="fitgirl">FitGirl</option><option value="dodi">DODI</option></select></label>
    <label>Game<input bind:value={query} placeholder="Search or leave empty for recent releases" disabled={busy} maxlength="200" /></label>
    <label class="check"><input type="checkbox" bind:checked={resolve} disabled={busy} />Resolve one torrent mirror</label>
    <button type="submit" disabled={busy || !enabled}>{busy ? "Fetching metadata…" : "Find releases"}</button>
  </form>
  {#each warnings as warning, i (i)}<p>{warning}</p>{/each}
  {#each results as entry (entry.id)}
    <article>
      <strong>{entry.title} {entry.version ?? ""}</strong>
      <p>{entry.rawTitle}</p>
      <button type="button" disabled={busy} onclick={() => lib.run(() => api.store.openReleasePage(entry.sourceId, entry.id))}>Open source page</button>
      {#if entry.languageClaim}<p>Languages: {entry.languageClaim}</p>{/if}
      {#each entry.warnings as warning, i (i)}<p>{warning}</p>{/each}
      {#each entry.references.filter((r) => r.state && r.state !== "resolved") as ref, i (i)}<p>{ref.state}: {ref.reason}</p>{/each}
      {#if entry.releaseKind === "update" || entry.summaryOnly}<p>This is an update or incomplete listing. It cannot be installed as a standalone game.</p>
      {:else}
        {#if entry.transports.length}
          <label>Torrent identity<select bind:value={choices[entry.id]}>
            {#each entry.transports as transport, i (i)}<option value={i}>{transport.torrentName || transport.infoHash}</option>{/each}
          </select></label>
          <label class="check"><input type="checkbox" bind:checked={confirmed[entry.id]} />I checked that this release and torrent match the game I want.</label>
          <button type="button" disabled={busy || !confirmed[entry.id]} onclick={() => review(entry)}>Add to catalog and choose download options</button>
        {:else}<p>No resolved torrent. Obtain its metadata from the source, then attach it here.</p>{/if}
        <button type="button" disabled={busy} onclick={() => attach(entry)}>Attach .torrent metadata</button>
      {/if}
    </article>
  {/each}
</div>
{#if selected}<InstallDialog entry={selected} offer={pickedOffer} onclose={() => selected = null} />{/if}

<style>
  .browser, form, article { display: flex; flex-direction: column; gap: 10px; }
  article { border: 1px solid var(--line-strong); border-radius: var(--radius); padding: 12px; overflow-wrap: anywhere; }
  label { display: flex; flex-direction: column; gap: 5px; }
  .check { flex-direction: row; align-items: center; }
  input:not([type="checkbox"]), select, button { min-height: 38px; border: 1px solid var(--line-strong); border-radius: var(--radius); padding: 8px 12px; color: var(--text); background: var(--surface-2); min-width: 0; }
  button:disabled { opacity: .5; }
  p { margin: 0; color: var(--muted); font-size: 13px; }
</style>
