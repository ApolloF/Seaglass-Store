<script lang="ts">
  // Browse: the local index with filters and sorting, filled out by a remote
  // search of the sources and Steam once typing stops.
  import { onDestroy, untrack } from "svelte";
  import Icon from "../../components/Icon.svelte";
  import { api } from "../../lib/api";
  import { shop } from "../../lib/shop.svelte";
  import { lib } from "../../lib/store.svelte";
  import { adoptPage, createSearchController, emptyQuery, PAGE_SIZE, providerText, queryKey, remoteShown, sourceFilters, type SearchController, type SearchView } from "../../lib/storefront";
  import { storefront } from "../../lib/storefront.svelte";
  import type { BrowseQuery, BrowseSort, GameSummary } from "../../lib/types";
  import GameCard from "./GameCard.svelte";

  let {
    text,
    version,
    downloading,
    onopen,
  }: { text: string; version: number; downloading: Set<string>; onopen: (g: GameSummary) => void } = $props();

  let sources = $state<string[]>([]);
  let language = $state("");
  let genre = $state("");
  let availability = $state<BrowseQuery["availability"]>("");
  let installed = $state<BrowseQuery["installed"]>("");
  let sort = $state<BrowseSort>("published");

  const query = $derived<BrowseQuery>({ ...emptyQuery(), text, sources: [...sources], language, genre, availability, installed, sort });
  const filtered = $derived(!!(sources.length || language || genre || availability || installed));

  let view = $state<SearchView>({ query: emptyQuery(), page: null, other: [], remote: [], searching: false, remoteError: "" });
  let games = $state<GameSummary[]>([]);
  let languages = $state<string[]>([]);
  let genres = $state<string[]>([]);
  let shownKey = "";
  let loadingMore = $state(false);

  function onView(v: SearchView) {
    view = v;
    if (!v.page) return;
    const key = queryKey(v.query);
    games = key === shownKey ? adoptPage(games, v.page) : v.page.games;
    shownKey = key;
    // The choices come from the whole index, so a narrowed page never shrinks them.
    if (v.page.languages.length) languages = v.page.languages;
    if (v.page.genres.length) genres = v.page.genres;
  }

  const controller: SearchController = createSearchController({
    browse: (q) => api.store.discovery.browse(q),
    search: (q) => api.store.discovery.search(q),
    onChange: onView,
    onError: (m) => lib.toast(m, "error"),
  });
  onDestroy(() => controller.dispose());

  $effect(() => {
    const q = query;
    untrack(() => controller.update(q));
  });
  // The index changed (a refresh, a release found): the local matches again, no new remote search.
  $effect(() => {
    void version;
    untrack(() => controller.refreshLocal());
  });
  $effect(() => api.store.discovery.onSearch((p) => controller.progress(p)));

  $effect(() => {
    const keys = [...games, ...view.other].map((g) => g.key);
    shop.requestArt(keys);
    storefront.ask(keys);
  });

  async function more() {
    const key = queryKey(view.query);
    const from = games.length;
    loadingMore = true;
    const r = await lib.run(() => api.store.discovery.browse({ ...view.query, offset: from }));
    loadingMore = false;
    if (r && key === queryKey(view.query) && from === games.length) games = [...games, ...r.page.games];
  }

  function toggleSource(id: string) {
    sources = sources.includes(id) ? sources.filter((s) => s !== id) : [...sources, id];
  }
  function clearFilters() {
    [sources, language, genre, availability, installed] = [[], "", "", "", ""];
  }

  const choices = $derived(sourceFilters(storefront.status));
  const total = $derived(view.page?.total ?? 0);
  const unknown = $derived(view.page?.unknown ?? 0);
  const unknownFields = $derived([language && "language", genre && "genre"].filter(Boolean).join(" or "));
  const providers = $derived(remoteShown(text, view.remote));
</script>

<div class="browse">
  <div class="filters" role="group" aria-label="Filters">
    <div class="seg" role="group" aria-label="Source">
      {#each choices as f (f.id)}
        <button type="button" class:on={sources.includes(f.id)} aria-pressed={sources.includes(f.id)} onclick={() => toggleSource(f.id)}>{f.label}</button>
      {/each}
    </div>
    <label class="pick"><span class="sr-only">Language</span>
      <select class="sf-field" bind:value={language}>
        <option value="">Any language</option>
        {#each languages as l (l)}<option value={l}>{l}</option>{/each}
        {#if language && !languages.includes(language)}<option value={language}>{language}</option>{/if}
      </select>
    </label>
    <label class="pick"><span class="sr-only">Genre</span>
      <select class="sf-field" bind:value={genre}>
        <option value="">Any genre</option>
        {#each genres as x (x)}<option value={x}>{x}</option>{/each}
        {#if genre && !genres.includes(genre)}<option value={genre}>{genre}</option>{/if}
      </select>
    </label>
    <label class="pick"><span class="sr-only">Availability</span>
      <select class="sf-field" bind:value={availability}>
        <option value="">Any availability</option>
        <option value="installable">Installable</option>
        <option value="unresolved">Needs resolving</option>
      </select>
    </label>
    <label class="pick"><span class="sr-only">Installed</span>
      <select class="sf-field" bind:value={installed}>
        <option value="">Installed or not</option>
        <option value="installed">Installed</option>
        <option value="not-installed">Not installed</option>
      </select>
    </label>
    <label class="pick"><span class="sr-only">Sort by</span>
      <select class="sf-field" bind:value={sort}>
        <option value="published">Newest published</option>
        <option value="title">Title</option>
        <option value="popular">Steam popularity</option>
        <option value="reviews">Review score</option>
      </select>
    </label>
    {#if filtered}<button type="button" class="sf-btn" onclick={clearFilters}>Clear filters</button>{/if}
  </div>

  {#if providers.length || view.remoteError}
    <ul class="remote" aria-label="Search progress" aria-live="polite">
      {#each providers as p (p.id)}
        <li class="sf-chip" class:accent={p.state === "loading"} class:warn={p.state === "error" || p.state === "unavailable"}>{p.name}: {providerText(p)}</li>
      {/each}
      {#if view.remoteError}<li class="sf-chip warn">Online search failed: {view.remoteError}</li>{/if}
    </ul>
  {/if}

  {#if !view.page}
    <p class="sf-muted" aria-busy="true">Loading…</p>
  {:else}
    <p class="tally sf-muted">
      {total.toLocaleString("en")} {total === 1 ? "game" : "games"}{text.trim() ? ` for “${text.trim()}”` : ""}{games.length < total ? `, showing ${games.length}` : ""}
    </p>
    {#if unknown > 0 && unknownFields}
      <p class="unknown sf-muted">{unknown} {unknown === 1 ? "game was" : "games were"} left out because their {unknownFields} isn't known.</p>
    {/if}

    {#if games.length}
      <ul class="grid">
        {#each games as g (g.key)}
          <li><GameCard game={g} {onopen} downloading={downloading.has(g.key)} /></li>
        {/each}
      </ul>
      {#if games.length < total}
        <button type="button" class="sf-btn more" disabled={loadingMore} onclick={more}>{loadingMore ? "Loading…" : `Show more (${Math.min(PAGE_SIZE, total - games.length)} of ${total - games.length})`}</button>
      {/if}
    {:else if !view.searching}
      <div class="sf-empty">
        <Icon name="search" size={40} stroke={1.6} />
        <h2>Nothing matches</h2>
        <p>{filtered ? "Try fewer filters." : text.trim() ? "Nothing in the index yet. Online results appear below when found." : "No games are indexed yet."}</p>
      </div>
    {/if}

    {#if view.other.length}
      <section class="other" aria-labelledby="sf-other">
        <div class="ohead">
          <h2 id="sf-other">Other games</h2>
          <span class="sf-chip">No known source release</span>
        </div>
        <p class="sf-muted">Found on Steam. You can save them to your wishlist and be told when a release appears.</p>
        <ul class="grid">
          {#each view.other as g (g.key)}
            <li><GameCard game={g} {onopen} onwish={(x) => storefront.toggleWish(x)} /></li>
          {/each}
        </ul>
      </section>
    {/if}
  {/if}
</div>

<style>
  .browse {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-width: 0;
  }
  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 10px;
  }
  .pick {
    min-width: 0;
  }
  .pick select {
    max-width: 100%;
  }
  .seg {
    display: flex;
    gap: 2px;
    padding: 3px;
    border-radius: 10px;
    background: var(--surface-2);
  }
  .seg button {
    height: 32px;
    padding: 0 12px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    font-size: 14px;
    font-weight: 700;
  }
  .seg button.on {
    background: var(--surface-3);
    color: var(--text);
  }
  .remote {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .tally,
  .unknown {
    margin: 0;
  }
  .grid {
    list-style: none;
    margin: 0;
    padding: 4px;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
    gap: 18px 16px;
  }
  .more {
    align-self: center;
  }
  .other {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 8px;
    padding-top: 14px;
    border-top: 1px solid var(--line);
  }
  .other p {
    margin: 0;
  }
  .ohead {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
  }
  .ohead h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 24px;
  }
  @media (max-width: 700px) {
    .grid {
      grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
      gap: 16px 12px;
    }
    .filters .pick {
      flex: 1 1 140px;
    }
    .filters .pick select {
      width: 100%;
    }
  }
</style>
