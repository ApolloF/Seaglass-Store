<script lang="ts">
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { entryLine, offerLine } from "../lib/catalog";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import type { CatalogEntry, CatalogQuery } from "../lib/types";

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
  let open = $state<string | null>(null);

  // A game already on its way shows that instead of a second download.
  const downloading = $derived(new Set(shop.downloads.filter((d) => d.gameKey && d.state !== "failed").map((d) => d.gameKey)));
  async function get(e: CatalogEntry, offer: number) {
    const d = await lib.run(() => api.store.downloadOffer(e.key, offer));
    if (d) lib.toast(`${d.title} is downloading. Follow it in Downloads.`);
  }
</script>

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
      <ul class="list">
        {#each entries as e (e.key)}
          <li class="entry" class:open={open === e.key}>
            <div class="head">
              <button type="button" class="expand" aria-expanded={open === e.key} onclick={() => (open = open === e.key ? null : e.key)}>
                <span class="title">{e.title}</span>
                <span class="line">{entryLine(e)}</span>
              </button>
              {#if downloading.has(e.key)}
                <span class="on-way"><Icon name="download" size={16} />In Downloads</span>
              {:else}
                <button type="button" class="get" onclick={() => get(e, 0)}><Icon name="download" size={16} />Get{e.offers.length > 1 ? " newest" : ""}</button>
              {/if}
              <button type="button" class="icon" aria-label={open === e.key ? `Hide ${e.title}'s versions` : `Show ${e.title}'s versions`} onclick={() => (open = open === e.key ? null : e.key)}>
                <span class="chev" class:up={open === e.key}><Icon name="chevronDown" size={16} stroke={2.2} /></span>
              </button>
            </div>
            {#if open === e.key}
              <ul class="offers">
                {#each e.offers as o, i (i)}
                  <li>
                    <div class="text">
                      <span class="v">{o.version || "Version not given"}{#if i === 0 && e.offers.length > 1}<span class="newest">Newest</span>{/if}</span>
                      <span class="line">{offerLine(o)}</span>
                      {#if o.languages?.length}<span class="line">{o.languages.join(", ")}</span>{/if}
                      {#if o.notes}<span class="line">{o.notes}</span>{/if}
                    </div>
                    <button type="button" class="btn" disabled={downloading.has(e.key)} onclick={() => get(e, i)}>Download</button>
                  </li>
                {/each}
              </ul>
            {/if}
          </li>
        {/each}
      </ul>
      {#if entries.length < total}
        <button type="button" class="btn more" onclick={more}>Show more ({total - entries.length})</button>
      {/if}
    </div>
  {/if}
</div>

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
  .list,
  .offers {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .entry {
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 8px 8px 0;
  }
  .expand {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 6px 16px;
    border: 0;
    background: none;
    text-align: left;
  }
  .title {
    font-weight: 700;
    font-size: 16px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .line {
    font-size: 13.5px;
    color: var(--muted);
  }
  .get,
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
  .get,
  .primary {
    border: 0;
    background: var(--accent);
    color: var(--accent-ink);
  }
  .get:hover,
  .primary:hover {
    filter: brightness(1.08);
  }
  .btn {
    border: 1px solid var(--line-strong);
    background: transparent;
  }
  .btn:hover:not(:disabled) {
    background: var(--surface-3);
  }
  .btn:disabled {
    opacity: 0.5;
  }
  .more {
    align-self: center;
  }
  .on-way {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 8px;
    color: var(--accent-text);
    font-size: 14px;
    font-weight: 700;
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
  .chev {
    display: flex;
    transition: transform 0.2s var(--ease);
  }
  .chev.up {
    transform: rotate(180deg);
  }
  .offers {
    padding: 0 12px 12px;
    gap: 6px;
  }
  .offers li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 10px 10px 14px;
    border-radius: 10px;
    background: var(--surface-3);
  }
  .offers .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .v {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 700;
  }
  .newest {
    padding: 1px 7px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 11.5px;
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
  .primary {
    margin-top: 12px;
  }
</style>
