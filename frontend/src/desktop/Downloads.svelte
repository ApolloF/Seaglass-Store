<script lang="ts">
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { progress, statusLine } from "../lib/downloads";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import type { Download, DownloadAction } from "../lib/types";

  let { onsettings }: { onsettings: () => void } = $props();

  $effect(() => shop.start());

  let link = $state("");
  let adding = $state(false);
  async function add(e: SubmitEvent) {
    e.preventDefault();
    if (!link.trim() || adding) return;
    adding = true;
    const d = await lib.run(() => api.store.addDownload(link.trim(), ""));
    adding = false;
    if (d) {
      link = "";
      lib.toast(`${d.title} is queued`);
    }
  }

  // The download whose removal is being confirmed.
  let removing = $state<string | null>(null);
  async function act(d: Download, action: DownloadAction, deleteFiles = false) {
    removing = null;
    await lib.run(() => api.store.action(d.id, action, deleteFiles));
    if (action === "remove") shop.downloads = shop.downloads.filter((x) => x.id !== d.id);
  }

  async function chooseExe() {
    const s = await lib.run(() => api.store.chooseQBittorrent());
    if (s) lib.settings = s;
  }

  // Newest first; finished ones after the ones still going.
  const list = $derived(
    [...shop.downloads].sort((a, b) => Number(a.state === "downloaded") - Number(b.state === "downloaded") || b.created - a.created),
  );
  const eng = $derived(shop.engine);
</script>

<div class="page">
  <div class="toolbar">
    <h1>Downloads</h1>
    <span class="badge">Experimental</span>
    <div class="grow"></div>
    <button type="button" class="tool" aria-label="Settings" title="Settings (Ctrl+,)" onclick={onsettings}><Icon name="gear" size={20} /></button>
  </div>

  <div class="body">
    {#if eng && !eng.installed}
      <div class="banner">
        <Icon name="info" size={20} />
        <div class="text">
          <strong>Downloads need qBittorrent</strong>
          <span>Seaglass runs it in the background with settings of its own, so your own qBittorrent and its torrents are left alone.</span>
        </div>
        <button type="button" class="btn primary" onclick={() => lib.run(() => api.store.getQBittorrent())}><Icon name="link" size={16} />Get qBittorrent</button>
        <button type="button" class="btn" onclick={chooseExe}>Choose qbittorrent.exe</button>
      </div>
    {:else if eng?.interfaceMissing}
      <div class="banner warn">
        <Icon name="warn" size={20} />
        <div class="text">
          <strong>Nothing is downloading</strong>
          <span>Downloads are bound to a network interface that's gone, such as a VPN that disconnected. They go on once it's back.</span>
        </div>
        <button type="button" class="btn" onclick={onsettings}>Network settings</button>
      </div>
    {/if}

    <form class="add" onsubmit={add}>
      <label class="sr-only" for="dl-link">Magnet link or link to a .torrent file</label>
      <input id="dl-link" type="text" autocomplete="off" spellcheck="false" placeholder="Paste a magnet link or a link to a .torrent file" bind:value={link} />
      <button type="submit" class="btn primary" disabled={!link.trim() || adding}><Icon name="download" size={16} />{adding ? "Adding…" : "Download"}</button>
    </form>

    {#if list.length === 0}
      <div class="empty">
        <Icon name="download" size={40} stroke={1.6} />
        <h2>No downloads</h2>
        <p>Downloads continue in the background while Seaglass waits in the tray.</p>
      </div>
    {:else}
      <ul class="list">
        {#each list as d (d.id)}
          <li class="item" class:failed={d.state === "failed"}>
            <div class="info">
              <span class="title">{d.title}</span>
              <span class="status">{statusLine(d, eng)}</span>
              {#if d.state !== "downloaded"}
                <div class="bar" role="progressbar" aria-label={`${d.title} progress`} aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(progress(d) * 100)}>
                  <span style:width={`${progress(d) * 100}%`} class:paused={d.state !== "downloading"}></span>
                </div>
              {/if}
            </div>
            {#if removing === d.id}
              <div class="confirm">
                <span>Remove it?</span>
                <button type="button" class="btn" onclick={() => act(d, "remove", false)}>Keep files</button>
                <button type="button" class="btn danger" onclick={() => act(d, "remove", true)}>Delete files</button>
                <button type="button" class="icon" aria-label="Don't remove" onclick={() => (removing = null)}><Icon name="close" size={16} /></button>
              </div>
            {:else}
              <div class="actions">
                {#if d.state === "queued" || d.state === "downloading"}
                  <button type="button" class="icon" aria-label={`Pause ${d.title}`} title="Pause" onclick={() => act(d, "pause")}><Icon name="stop" size={14} /></button>
                {:else if d.state === "paused" || d.state === "failed"}
                  <button type="button" class="icon" aria-label={`${d.state === "failed" ? "Try again" : "Resume"}: ${d.title}`} title={d.state === "failed" ? "Try again" : "Resume"} onclick={() => act(d, "resume")}
                    ><Icon name={d.state === "failed" ? "refresh" : "play"} size={16} /></button
                  >
                {/if}
                <button type="button" class="icon" aria-label={`Show ${d.title} in Explorer`} title="Show in Explorer" onclick={() => lib.run(() => api.store.showDownload(d.id))}><Icon name="folder" size={16} /></button>
                <button type="button" class="icon" aria-label={`Remove ${d.title}`} title="Remove" onclick={() => (removing = d.id)}><Icon name="trash" size={16} /></button>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .page {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .toolbar {
    height: 68px;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 0 28px;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 30px;
    font-weight: 700;
  }
  .badge {
    padding: 2px 8px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 12px;
    font-weight: 700;
  }
  .grow {
    flex: 1;
  }
  .tool {
    width: 40px;
    height: 40px;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: var(--surface-2);
    color: var(--text-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .tool:hover {
    background: var(--surface-3);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: 4px 28px 28px;
  }
  .banner {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 12px;
    padding: 14px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
    color: var(--text-2);
  }
  .banner.warn {
    color: var(--warn);
  }
  .banner .text {
    flex: 1;
    min-width: 220px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    color: var(--text-2);
  }
  .banner strong {
    color: var(--text);
  }
  .add {
    display: flex;
    gap: 10px;
  }
  .add input {
    flex: 1;
    min-width: 0;
    height: 40px;
    padding: 0 12px;
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: var(--surface-2);
    outline: none;
    font-size: 14.5px;
  }
  .add input:focus {
    border-color: var(--accent);
  }
  .btn {
    flex-shrink: 0;
    height: 40px;
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
    background: var(--surface-2);
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
  .btn.danger {
    color: var(--danger);
  }
  .btn:disabled {
    opacity: 0.5;
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .title {
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .status {
    font-size: 13.5px;
    color: var(--muted);
  }
  .item.failed .status {
    color: var(--warn);
  }
  .bar {
    height: 6px;
    margin-top: 4px;
    border-radius: 99px;
    background: var(--surface-3);
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--accent);
    transition: width 0.6s var(--ease);
  }
  .bar span.paused {
    background: var(--muted);
  }
  .actions,
  .confirm {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .confirm {
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  .confirm span {
    font-size: 14px;
    color: var(--text-2);
  }
  .icon {
    width: 34px;
    height: 34px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .icon:hover {
    background: var(--surface-3);
    color: var(--text);
  }
  .empty {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 40px;
    text-align: center;
    color: var(--muted);
  }
  .empty h2 {
    margin: 8px 0 0;
    color: var(--text);
    font-family: var(--font-display);
    font-size: 26px;
  }
  .empty p {
    margin: 0;
    max-width: 440px;
  }
</style>
