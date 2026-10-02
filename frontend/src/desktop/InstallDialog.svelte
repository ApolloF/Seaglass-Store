<script lang="ts">
  // Before a catalog game downloads: which version, which language, where
  // it goes, and whether to install it straight away.
  import { untrack } from "svelte";
  import Icon from "../components/Icon.svelte";
  import Toggle from "../components/Toggle.svelte";
  import { api } from "../lib/api";
  import { offerLine } from "../lib/catalog";
  import { bytes } from "../lib/format";
  import { lib } from "../lib/store.svelte";
  import type { CatalogEntry } from "../lib/types";

  let {
    entry,
    offer = 0,
    update = false,
    onclose,
    ondone,
  }: { entry: CatalogEntry; offer?: number; update?: boolean; onclose: () => void; ondone?: () => void } = $props();

  // Starts on the version picked on the page; changed here after that.
  let pick = $state(untrack(() => offer));
  const o = $derived(entry.offers[pick] ?? entry.offers[0]);
  let language = $state("English");
  let dir = $state("");
  let install = $state(true);
  let busy = $state(false);

  $effect(() => {
    if (update && entry.installed) dir = entry.installed.dir;
    else api.store.installFolder(entry.title).then((d) => (dir = d));
  });
  // A language the newly picked version doesn't have goes back to the default.
  $effect(() => {
    if (o.languages?.length && !o.languages.some((l) => l.toLowerCase() === language.toLowerCase())) language = o.languages.find((l) => l.toLowerCase() === "english") ?? o.languages[0];
  });

  async function choose() {
    const d = await lib.run(() => api.store.chooseInstallFolder(dir));
    if (d) dir = d;
  }
  async function start() {
    busy = true;
    const d = await lib.run(() => api.store.downloadOffer(entry.key, pick, { dir, language, install, update }));
    busy = false;
    if (d) {
      lib.toast(`${d.title} is downloading. Follow it in Downloads.`);
      ondone?.();
      onclose();
    }
  }

  let box: HTMLDivElement | undefined = $state();
  $effect(() => {
    box?.focus();
  });
</script>

<div class="scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dialog" role="dialog" aria-modal="true" aria-label={`${update ? "Update" : "Get"} ${entry.title}`} tabindex="-1" bind:this={box} onkeydown={(e) => e.key === "Escape" && onclose()}>
    <div class="top">
      <h2>{update ? "Update" : "Get"} {entry.title}</h2>
      <button type="button" class="close" aria-label="Close" onclick={onclose}><Icon name="close" size={18} stroke={2.2} /></button>
    </div>

    {#if entry.offers.length > 1}
      <label class="field">
        <span class="label">Version</span>
        <select bind:value={pick}>
          {#each entry.offers as x, i (i)}<option value={i}
              >{x.version || "Version not given"} · {x.feedName}{i === (entry.recommended?.offer ?? 0) ? " (recommended)" : ""}</option
            >{/each}
        </select>
      </label>
    {/if}
    <p class="sub">{offerLine(o)}</p>

    <label class="field">
      <span class="label">Language</span>
      <select bind:value={language} disabled={!o.languages?.length}>
        {#if !o.languages?.length}<option value="English">English (availability checked after metadata arrives)</option>{/if}
        {#each o.languages ?? [] as l (l)}<option value={l}>{l}</option>{/each}
      </select>
    </label>

    <div class="field">
      <span class="label">Folder</span>
      <span class="path" title={dir}>{dir}</span>
      {#if !update}<button type="button" class="btn" onclick={choose}>Change</button>{/if}
    </div>
    {#if update}
      <p class="sub">Installs over {entry.installed?.version} once downloaded and checked. Your saves usually stay; back them up first if the game keeps them in its folder.</p>
    {:else}
    <Toggle
      checked={install}
      title="Install when it's downloaded"
      detail="Installs after checks pass. Off: choose torrent or installer languages in Downloads before installing."
      onchange={(v) => (install = v)}
    />
    {/if}

    <p class="sub">
      {[o.sizeBytes && `Download ${bytes(o.sizeBytes)}`, o.installedSizeBytes && `${bytes(o.installedSizeBytes)} once installed`].filter(Boolean).join(" · ")}
    </p>

    <div class="actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="button" class="btn primary" disabled={busy || !dir} onclick={start}><Icon name="download" size={16} />{busy ? "Starting…" : "Download"}</button>
    </div>
  </div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 60;
    background: var(--scrim);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
  }
  .dialog {
    width: min(560px, 100%);
    max-height: 100%;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 22px 24px;
    border-radius: var(--radius-l);
    background: var(--surface);
    box-shadow: var(--shadow);
    outline: none;
  }
  .top {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  h2 {
    flex: 1;
    margin: 0;
    font-family: var(--font-display);
    font-size: 24px;
  }
  .close {
    width: 36px;
    height: 36px;
    border: 0;
    border-radius: 10px;
    background: transparent;
    color: var(--muted);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .close:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .field {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 44px;
    padding: 4px 6px 4px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
    font-size: 14px;
  }
  .label {
    width: 90px;
    flex-shrink: 0;
    font-weight: 600;
  }
  select {
    flex: 1;
    min-width: 0;
    height: 36px;
    padding: 0 10px;
    border-radius: 8px;
    border: 1px solid var(--line-strong);
    background: var(--surface);
    color: var(--text);
    font-size: 14px;
  }
  .path {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-2);
  }
  .sub {
    margin: -4px 0 0;
    font-size: 13.5px;
    color: var(--muted);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    margin-top: 4px;
  }
  .btn {
    height: 38px;
    padding: 0 14px;
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: transparent;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14.5px;
    font-weight: 700;
  }
  .btn:hover:not(:disabled) {
    background: var(--surface-3);
  }
  .btn.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--accent-ink);
  }
  .btn.primary:hover:not(:disabled) {
    background: var(--accent);
    filter: brightness(1.08);
  }
  .btn:disabled {
    opacity: 0.6;
  }
</style>
