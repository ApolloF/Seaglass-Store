<script lang="ts">
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { engineLabel, engineProblem, pageList } from "../lib/downloads";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import DownloadRow from "./DownloadRow.svelte";

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

  let sandbox = $state(false);
  $effect(() => {
    api.store.sandboxAvailable().then((v) => (sandbox = v));
  });

  async function chooseExe() {
    const s = await lib.run(() => api.store.chooseQBittorrent());
    if (s) lib.settings = s;
  }

  const list = $derived(pageList(shop.downloads));
  const eng = $derived(shop.engine);
  const problem = $derived(engineProblem(eng, shop.downloads));
  let retrying = $state(false);
  async function retry() {
    retrying = true;
    const s = await lib.run(() => api.store.startEngine());
    retrying = false;
    if (s) shop.engine = s;
  }
</script>

<div class="page">
  <div class="toolbar">
    <h1>Downloads</h1>
    <span class="badge">Experimental</span>
    {#if eng}<span class="state" class:warn={!!problem}>{engineLabel(eng, shop.downloads)}</span>{/if}
    <div class="grow"></div>
    {#if shop.engine}
      <button type="button" class="btn" onclick={() => lib.run(() => api.store.holdDownloads(!shop.engine?.held))}
        ><Icon name={shop.engine.held ? "play" : "stop"} size={14} />{shop.engine.held ? "Resume all" : "Pause all"}</button
      >
    {/if}
    <button type="button" class="tool" aria-label="Settings" title="Settings (Ctrl+,)" onclick={onsettings}><Icon name="gear" size={20} /></button>
  </div>

  <div class="body">
    {#if problem}
      <div class="banner problem" class:warn={problem.fix === "path"} role="alert">
        <Icon name={problem.fix === "path" ? "warn" : "info"} size={20} />
        <div class="text">
          <strong>{problem.title}</strong>
          {#if problem.error}<span class="err">{problem.error}</span>{/if}
          <span>{problem.next}</span>
          {#if problem.fix === "install"}<span>Seaglass runs it in the background with settings of its own, so your own qBittorrent and its torrents are left alone.</span>{/if}
        </div>
        <div class="fixes">
          {#if problem.fix === "install"}
            <button type="button" class="btn primary" onclick={() => lib.run(() => api.store.getQBittorrent())}><Icon name="link" size={16} />Get qBittorrent</button>
            <button type="button" class="btn" onclick={chooseExe}>Choose qbittorrent.exe</button>
          {:else}
            <button type="button" class="btn primary" disabled={retrying} onclick={retry}><Icon name="refresh" size={16} />{retrying ? "Starting…" : "Try again"}</button>
            <button type="button" class="btn" onclick={chooseExe}>Choose qbittorrent.exe</button>
            <button type="button" class="btn" onclick={onsettings}>Settings</button>
          {/if}
        </div>
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
          <DownloadRow {d} {eng} {sandbox} />
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
  .state {
    font-size: 13px;
    font-weight: 600;
    color: var(--muted);
  }
  .state.warn {
    color: var(--warn);
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
  .banner.problem {
    align-items: flex-start;
  }
  /* The ways out sit under the explanation, lined up with it. */
  .fixes {
    flex-basis: 100%;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding-left: 32px;
  }
  .banner .err {
    color: var(--warn);
    overflow-wrap: anywhere;
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
