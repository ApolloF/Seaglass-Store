<script lang="ts">
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { bytes } from "../lib/format";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import type { CatalogEntry, CatalogQuery } from "../lib/types";
  import StoreGame from "./StoreGame.svelte";

  let { onsettings }: { onsettings: () => void } = $props();

  const pageSize = 60;
  let text = $state("");
  let language = $state("");
  let sort = $state<CatalogQuery["sort"]>("title");
  let entries = $state<CatalogEntry[]>([]);
  let total = $state(0);
  let loaded = $state(false);
  let languages = $state<string[]>([]);
  let version = $state(0); // bumped when the catalog changes

  $effect(() => {
    shop.start();
    return api.store.onCatalog(() => version++);
  });
  $effect(() => {
    void version;
    api.store.catalogLanguages().then((l) => (languages = l));
  });
  // A new search starts from the top, a moment after typing stops.
  $effect(() => {
    const q: CatalogQuery = { text, language, sort, offset: 0, limit: pageSize };
    void version;
    const t = setTimeout(async () => {
      const p = await lib.run(() => api.store.catalog(q));
      if (p) [entries, total] = [p.entries, p.total];
      loaded = true;
    }, 150);
    return () => clearTimeout(t);
  });
  async function more() {
    const p = await lib.run(() => api.store.catalog({ text, language, sort, offset: entries.length, limit: pageSize }));
    if (p) [entries, total] = [[...entries, ...p.entries], p.total];
  }

  const hasFeeds = $derived((lib.settings?.store.feeds ?? []).some((f) => f.enabled));
  let selected = $state<CatalogEntry | null>(null);

  // Art for what's on screen, first things first.
  $effect(() => shop.requestArt(entries.map((e) => e.key)));
  // A game already on its way is marked.
  const downloading = $derived(new Set(shop.downloads.filter((d) => d.gameKey && d.state !== "failed").map((d) => d.gameKey)));
</script>

{#if selected}
  <StoreGame entry={selected} onback={() => (selected = null)} />
{:else}
<div class="store">
  <div class="toolbar">
    <h1>Store</h1>
    <span class="badge">Experimental</span>
    <label class="search">
      <Icon name="search" size={18} stroke={2} />
      <span class="sr-only">Search the catalog</span>
      <input type="search" placeholder={total ? `Search ${total} games` : "Search"} bind:value={text} onkeydown={(e) => e.key === "Escape" && (text = "")} />
    </label>
    <label class="pick">
      <span class="sr-only">Language</span>
      <select bind:value={language}>
        <option value="">Any language</option>
        {#each languages as l (l)}<option value={l}>{l}</option>{/each}
      </select>
      <Icon name="chevronDown" size={14} stroke={2.2} />
    </label>
    <label class="pick">
      <span class="sr-only">Sort by</span>
      <select bind:value={sort}>
        <option value="title">Title</option>
        <option value="updated">Recently updated</option>
        <option value="size">Size</option>
      </select>
      <Icon name="chevronDown" size={14} stroke={2.2} />
    </label>
    <div class="grow"></div>
    <button type="button" class="tool" aria-label="Settings" title="Settings (Ctrl+,)" onclick={onsettings}><Icon name="gear" size={20} /></button>
  </div>

  {#if !hasFeeds}
    <div class="empty">
      <Icon name="cloudDown" size={40} stroke={1.6} />
      <h2>No catalogs yet</h2>
      <p>The store shows the games in feeds you add. Seaglass comes with none: add the address of a feed you trust, for games you're allowed to download.</p>
      <button type="button" class="primary" onclick={onsettings}>Add a feed</button>
    </div>
  {:else if loaded && entries.length === 0}
    <div class="empty">
      <Icon name="search" size={40} stroke={1.6} />
      <h2>{text || language ? "Nothing matches" : "The feeds have no games yet"}</h2>
      {#if !text && !language}<p>Settings → Experimental shows how each feed's last fetch went.</p>{/if}
    </div>
  {:else}
    <div class="body">
      <ul class="grid">
        {#each entries as e (e.key)}
          <li>
            <button type="button" class="card" onclick={() => (selected = e)}>
              <span class="cover">
                <GameArt game={{ key: e.key, meta: shop.art[e.key] }} />
                {#if downloading.has(e.key)}<span class="flag"><Icon name="download" size={14} stroke={2.4} />In Downloads</span>{/if}
              </span>
              <span class="title">{e.title}</span>
              <span class="line">{[e.version, bytes(e.size)].filter(Boolean).join(" · ")}</span>
            </button>
          </li>
        {/each}
      </ul>
      {#if entries.length < total}
        <button type="button" class="btn more" onclick={more}>Show more ({total - entries.length})</button>
      {/if}
    </div>
  {/if}
</div>
{/if}

<style>
  .store {
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }
  .toolbar {
    min-height: 68px;
    flex-shrink: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px 12px;
    padding: 12px 28px;
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
  .search {
    width: min(300px, 100%);
    height: 40px;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: var(--surface-2);
    color: var(--muted);
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: 0;
    outline: none;
    background: none;
    font-size: 14.5px;
  }
  .search:focus-within {
    border-color: var(--accent);
  }
  .pick {
    position: relative;
    display: flex;
    align-items: center;
    color: var(--muted);
  }
  .pick select {
    appearance: none;
    height: 40px;
    padding: 0 32px 0 12px;
    border-radius: 10px;
    border: 1px solid var(--line);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 14px;
  }
  .pick :global(svg) {
    position: absolute;
    right: 10px;
    pointer-events: none;
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
    padding: 4px 28px 28px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .grid {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 18px 16px;
  }
  .card {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
  }
  .cover {
    position: relative;
    aspect-ratio: 2 / 3;
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--surface-2);
    margin-bottom: 4px;
    transition: transform 0.2s var(--ease), box-shadow 0.2s var(--ease);
  }
  .cover :global(img),
  .cover :global(.art) {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .card:hover .cover,
  .card:focus-visible .cover {
    transform: translateY(-3px);
    box-shadow: var(--shadow);
  }
  .flag {
    position: absolute;
    left: 8px;
    bottom: 8px;
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 3px 8px;
    border-radius: 999px;
    background: var(--accent);
    color: var(--accent-ink);
    font-size: 12px;
    font-weight: 700;
  }
  .title {
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .line {
    font-size: 13px;
    color: var(--muted);
  }
  .btn,
  .primary {
    flex-shrink: 0;
    height: 38px;
    padding: 0 14px;
    border-radius: 10px;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14.5px;
    font-weight: 700;
  }
  .primary {
    border: 0;
    background: var(--accent);
    color: var(--accent-ink);
    margin-top: 12px;
  }
  .primary:hover {
    filter: brightness(1.08);
  }
  .btn {
    border: 1px solid var(--line-strong);
    background: transparent;
  }
  .btn:hover {
    background: var(--surface-3);
  }
  .more {
    align-self: center;
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
    max-width: 460px;
  }
</style>
