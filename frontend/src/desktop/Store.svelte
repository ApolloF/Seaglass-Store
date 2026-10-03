<script lang="ts">
  // The Store: Home, Browse and Wishlist over the automatically discovered
  // releases, with one search box and the status of discovery.
  import { tick, untrack } from "svelte";
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import { emptyQuery, storeMode } from "../lib/storefront";
  import { storefront } from "../lib/storefront.svelte";
  import type { GameSummary } from "../lib/types";
  import "./store/store.css";
  import Browse from "./store/Browse.svelte";
  import Home from "./store/Home.svelte";
  import SourceSetup from "./store/SourceSetup.svelte";
  import StatusBar from "./store/StatusBar.svelte";
  import Wishlist from "./store/Wishlist.svelte";
  import StoreGame from "./StoreGame.svelte";

  let { onsettings }: { onsettings: () => void } = $props();

  type Tab = "home" | "browse" | "wishlist";
  const tabs: { id: Tab; label: string }[] = [
    { id: "home", label: "Home" },
    { id: "browse", label: "Browse" },
    { id: "wishlist", label: "Wishlist" },
  ];
  let tab = $state<Tab>("home");
  let browsed = $state(false); // Browse keeps its filters once it has been opened
  let text = $state("");
  let version = $state(0); // bumped when games change
  let shown = $state(0); // games there are to show, from any source
  let probed = $state(false);
  let selected = $state<GameSummary | null>(null);
  let returnKey = "";
  let root: HTMLDivElement | undefined = $state();

  $effect(() => {
    shop.start();
    storefront.start();
    return api.store.discovery.onGames(() => version++);
  });

  // Whether anything is there to show decides the empty states, not the feed list.
  // Indexing changes the games many times a second: ask once it settles,
  // and keep only the newest answer (an older one could say 0).
  let probeSeq = 0;
  $effect(() => {
    void version;
    void storefront.status?.setupNeeded;
    void storefront.status?.games;
    const seq = ++probeSeq;
    const t = setTimeout(
      () =>
        api.store.discovery
          .browse({ ...emptyQuery(), limit: 1 })
          .then((r) => seq === probeSeq && (shown = r.page.total))
          .catch(() => {}) // keeps the last count; the status line says what's wrong
          .finally(() => (probed = true)),
      untrack(() => probed) ? 300 : 0,
    );
    return () => clearTimeout(t);
  });

  const mode = $derived(storeMode({ loaded: probed, status: storefront.status, shown }));
  const downloading = $derived(new Set(shop.downloads.filter((d) => d.gameKey && d.state !== "failed" && d.state !== "installed").map((d) => d.gameKey as string)));
  const refreshing = $derived(!!storefront.status?.refreshing);

  $effect(() => {
    if (tab === "browse") browsed = true;
  });
  // Typing searches: it belongs on the Browse tab.
  function typed() {
    if (text.trim()) tab = "browse";
  }
  function searchKeys(e: KeyboardEvent) {
    if (e.key === "Escape" && text) {
      e.stopPropagation();
      text = "";
    }
  }

  async function refresh() {
    const s = await lib.run(() => api.store.discovery.refresh());
    if (s) storefront.status = s;
  }

  function open(g: GameSummary) {
    returnKey = g.key;
    selected = g;
  }
  async function back() {
    selected = null;
    await tick();
    const card = returnKey ? root?.querySelector<HTMLElement>(`[data-key="${CSS.escape(returnKey)}"]`) : null;
    (card ?? root?.querySelector<HTMLElement>('[role="tab"][aria-selected="true"]'))?.focus();
  }

  function tabKeys(e: KeyboardEvent) {
    const i = tabs.findIndex((t) => t.id === tab);
    const to = e.key === "ArrowRight" ? (i + 1) % tabs.length : e.key === "ArrowLeft" ? (i + tabs.length - 1) % tabs.length : e.key === "Home" ? 0 : e.key === "End" ? tabs.length - 1 : -1;
    if (to < 0) return;
    e.preventDefault();
    tab = tabs[to].id;
    void tick().then(() => root?.querySelector<HTMLElement>(`#sf-tab-${tab}`)?.focus());
  }
</script>

{#if selected}
  <StoreGame gameKey={selected.key} initial={selected} onback={back} onkey={(k) => selected && (selected = { ...selected, key: k })} />
{/if}

<div class="store" bind:this={root} style:display={selected ? "none" : undefined}>
  <div class="toolbar">
    <h1>Store</h1>
    <span class="badge">Experimental</span>
    {#if mode === "ready"}
      <label class="search">
        <Icon name="search" size={18} stroke={2} />
        <span class="sr-only">Search games</span>
        <input type="search" placeholder="Search games" autocomplete="off" bind:value={text} oninput={typed} onkeydown={searchKeys} />
      </label>
    {/if}
    <div class="grow"></div>
    {#if mode === "ready" || mode === "finding" || mode === "unreachable"}
      <button type="button" class="tool" aria-label={refreshing ? "Checking for new releases" : "Check for new releases"} title="Check for new releases" disabled={refreshing || !storefront.status?.enabled} onclick={refresh}>
        <span class:spin={refreshing}><Icon name="refresh" size={20} /></span>
      </button>
    {/if}
    <button type="button" class="tool" aria-label="Settings" title="Settings (Ctrl+,)" onclick={onsettings}><Icon name="gear" size={20} /></button>
  </div>

  {#if mode === "loading"}
    <p class="loading sf-muted" aria-busy="true">Loading the Store…</p>
  {:else if mode === "setup"}
    <SourceSetup />
  {:else if mode === "off"}
    <div class="sf-empty fill">
      <Icon name="cloudDown" size={40} stroke={1.6} />
      <h2>No games to show</h2>
      <p>Source discovery is off and no feed has games. Turn on FitGirl or DODI, or add a feed, in Settings → Experimental.</p>
      <button type="button" class="sf-primary" onclick={onsettings}>Open settings</button>
    </div>
  {:else if mode === "finding"}
    <StatusBar />
    <div class="sf-empty fill" aria-live="polite">
      <Icon name="refresh" size={40} stroke={1.6} />
      <h2>Finding releases…</h2>
      <p>Seaglass is reading the newest release lists. This takes a minute the first time. Games appear here as they are found.</p>
    </div>
  {:else if mode === "unreachable"}
    <StatusBar />
    <div class="sf-empty fill">
      <Icon name="warn" size={40} stroke={1.6} />
      <h2>Couldn't reach the sources</h2>
      <p>Nothing is saved from an earlier visit yet. Check your connection, then try again.</p>
      <button type="button" class="sf-primary" disabled={refreshing} onclick={refresh}>Try again</button>
    </div>
  {:else}
    <StatusBar />
    <div class="tabs" role="tablist" aria-label="Store sections" tabindex="-1" onkeydown={tabKeys}>
      {#each tabs as t (t.id)}
        <button type="button" role="tab" id={`sf-tab-${t.id}`} aria-selected={tab === t.id} aria-controls={`sf-panel-${t.id}`} tabindex={tab === t.id ? 0 : -1} class:on={tab === t.id} onclick={() => (tab = t.id)}>
          {t.label}{#if t.id === "wishlist" && storefront.unread}<span class="count" aria-label={`${storefront.unread} new`}>{storefront.unread}</span>{/if}
        </button>
      {/each}
    </div>
    <div class="body">
      <div role="tabpanel" id="sf-panel-home" aria-labelledby="sf-tab-home" hidden={tab !== "home"}>
        {#if tab === "home"}<Home {version} {downloading} onopen={open} />{/if}
      </div>
      <div role="tabpanel" id="sf-panel-browse" aria-labelledby="sf-tab-browse" hidden={tab !== "browse"}>
        {#if browsed}<Browse {text} {version} {downloading} onopen={open} />{/if}
      </div>
      <div role="tabpanel" id="sf-panel-wishlist" aria-labelledby="sf-tab-wishlist" hidden={tab !== "wishlist"}>
        {#if tab === "wishlist"}<Wishlist onopen={open} onbrowse={() => (tab = "browse")} />{/if}
      </div>
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
    width: min(340px, 100%);
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
  .tool:hover:not(:disabled) {
    background: var(--surface-3);
  }
  .spin {
    display: flex;
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  .tabs {
    flex-shrink: 0;
    display: flex;
    gap: 4px;
    padding: 0 28px 8px;
    border-bottom: 1px solid var(--line);
  }
  .tabs button {
    min-height: 38px;
    padding: 0 16px;
    border: 0;
    border-radius: 10px;
    background: transparent;
    color: var(--muted);
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 15.5px;
    font-weight: 700;
  }
  .tabs button:hover {
    background: var(--surface-2);
  }
  .tabs button.on {
    background: var(--surface-3);
    color: var(--text);
  }
  .count {
    min-width: 20px;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--accent);
    color: var(--accent-ink);
    font-size: 12px;
    line-height: 20px;
    text-align: center;
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 16px 28px 28px;
  }
  .loading {
    padding: 20px 28px;
    margin: 0;
  }
  .fill {
    flex: 1;
  }
  @media (max-width: 700px) {
    .toolbar {
      padding: 10px 16px;
    }
    .search {
      order: 5;
      width: 100%;
    }
    .tabs {
      padding: 0 16px 8px;
    }
    .body {
      padding: 12px 16px 24px;
    }
  }
</style>
