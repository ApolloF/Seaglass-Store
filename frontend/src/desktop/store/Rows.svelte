<script lang="ts">
  // New releases, Popular and Recently updated as compact rows, with a
  // preview of the selected game beside them. Arrow keys move the selection,
  // Enter opens the game. Without room for the preview, a row opens the game.
  import { tick } from "svelte";
  import GameArt from "../../components/GameArt.svelte";
  import Icon from "../../components/Icon.svelte";
  import { countText, dateText, hoursText, sourceLabel } from "../../lib/storefront";
  import { homeTabs, moveIndex, previewKey, rowFacts, tabEmpty, tabGames, type HomeTab } from "../../lib/storefront-home";
  import { shop } from "../../lib/shop.svelte";
  import { storefront } from "../../lib/storefront.svelte";
  import type { GameSummary, StoreHome } from "../../lib/types";

  let { home, downloading, onopen }: { home: StoreHome; downloading: Set<string>; onopen: (g: GameSummary) => void } = $props();

  const PREVIEW_MIN = 640;
  const id = `sf-rows-${Math.random().toString(36).slice(2, 8)}`;
  let tab = $state<HomeTab>("new");
  let chosen = $state("");
  let width = $state(1000);
  let root: HTMLElement | undefined = $state();
  // Keep in step with the container query that hides the preview below.
  const narrow = $derived(width <= PREVIEW_MIN);

  const rows = $derived(tabGames(home, tab));
  const selectedKey = $derived(previewKey(rows, chosen));
  const selected = $derived(rows.find((g) => g.key === selectedKey));
  const shown = $derived(selected ? storefront.card(selected) : null);

  function pick(g: GameSummary) {
    if (narrow) onopen(g);
    else chosen = g.key;
  }
  async function focusRow(key: string) {
    await tick();
    root?.querySelector<HTMLElement>(`[data-row="${CSS.escape(key)}"]`)?.focus();
  }
  function rowKeys(e: KeyboardEvent, g: GameSummary) {
    if (e.key === "Enter") {
      e.preventDefault();
      onopen(g);
      return;
    }
    const to = moveIndex(rows.indexOf(g), e.key, rows.length);
    if (to < 0) return;
    e.preventDefault();
    chosen = rows[to].key;
    void focusRow(rows[to].key);
  }
  function tabKeys(e: KeyboardEvent) {
    const i = homeTabs.findIndex((t) => t.id === tab);
    const to = e.key === "ArrowRight" ? (i + 1) % homeTabs.length : e.key === "ArrowLeft" ? (i + homeTabs.length - 1) % homeTabs.length : e.key === "Home" ? 0 : e.key === "End" ? homeTabs.length - 1 : -1;
    if (to < 0) return;
    e.preventDefault();
    tab = homeTabs[to].id;
    void tick().then(() => root?.querySelector<HTMLElement>(`#${id}-${tab}`)?.focus());
  }
</script>

{#snippet row(game: GameSummary)}
  {@const g = storefront.card(game)}
  {@const f = rowFacts(g)}
  <li>
    <button
      type="button"
      class="row"
      class:on={g.key === selectedKey && !narrow}
      data-key={g.key}
      data-row={g.key}
      tabindex={g.key === selectedKey ? 0 : -1}
      aria-current={g.key === selectedKey && !narrow ? "true" : undefined}
      onclick={() => pick(game)}
      ondblclick={() => onopen(game)}
      onkeydown={(e) => rowKeys(e, game)}
    >
      <span class="capsule"><GameArt game={{ key: g.key, meta: shop.art[g.key] }} kind="hero" /></span>
      <span class="main">
        <span class="title">{g.title}</span>
        <span class="sub">
          {#if downloading.has(g.key)}<span class="sf-chip accent">In Downloads</span>
          {:else if g.installed?.update}<span class="sf-chip accent">Update</span>{/if}
          {#each g.sources as s (s)}<span class="sf-chip">{sourceLabel(s)}</span>{/each}
        </span>
      </span>
      <span class="facts">
        <span class:muted={!f.review}>{f.review || "No reviews yet"}</span>
        <span>{f.availability}{f.version ? ` · ${f.version}` : ""}</span>
        {#if f.mainStory}<span>{f.mainStory}</span>{/if}
        {#if f.published}<span class="muted">{f.published}</span>{/if}
      </span>
    </button>
  </li>
{/snippet}

<section class="rows" bind:this={root} bind:clientWidth={width} aria-label="Releases">
  <div class="tabs" role="tablist" aria-label="Release lists">
    {#each homeTabs as t (t.id)}
      <button type="button" role="tab" id={`${id}-${t.id}`} aria-selected={tab === t.id} aria-controls={`${id}-panel`} tabindex={tab === t.id ? 0 : -1} class:on={tab === t.id} onclick={() => (tab = t.id)} onkeydown={tabKeys}>
        {t.label}
      </button>
    {/each}
    {#if tab === "popular" && home.popularState === "stale"}<span class="sf-chip">Cached chart</span>{/if}
  </div>
  <div class="split" id={`${id}-panel`} role="tabpanel" aria-labelledby={`${id}-${tab}`}>
    {#if rows.length}
      <ul class="list">
        {#each rows as game (game.key)}{@render row(game)}{/each}
      </ul>
    {:else}
      <p class="empty sf-muted">{tabEmpty(tab, home.popularState)}</p>
    {/if}
    {#if shown && selected}
      <aside class="preview" aria-label={`${shown.title}: preview`}>
        <div class="shot"><GameArt game={{ key: shown.key, meta: shop.art[shown.key] }} kind="hero" /></div>
        <h3>{shown.title}</h3>
        {#if shown.genres.length}<p class="genres">{shown.genres.join(" · ")}</p>{/if}
        <dl>
          <dt>Game released</dt>
          <dd>{shown.releaseDate || "Not known"}</dd>
          <dt>Steam reviews</dt>
          <dd>
            {#if shown.reviewTotal}{shown.reviewLabel ?? ""} {shown.reviewPercent}% of {countText(shown.reviewTotal, "review")}{:else}Not known{/if}
          </dd>
          <dt>Main story</dt>
          <dd>{hoursText(shown.completionMain) || "Not known"}</dd>
          <dt>Sources</dt>
          <dd>{shown.sources.map(sourceLabel).join(" · ")}</dd>
          <dt>Availability</dt>
          <dd>{rowFacts(shown).availability}{shown.version ? ` · ${shown.version}` : ""}</dd>
          <dt>Source published</dt>
          <dd>{dateText(shown.publishedAt) || "Not known"}</dd>
        </dl>
        <div class="actions">
          <button type="button" class="sf-primary" onclick={() => onopen(selected)}>Open</button>
          <button type="button" class="sf-btn" aria-pressed={shown.wishlisted} onclick={() => storefront.toggleWish(shown)}>
            <Icon name="star" size={15} stroke={2.2} />{shown.wishlisted ? "On wishlist" : "Add to wishlist"}
          </button>
        </div>
      </aside>
    {/if}
  </div>
</section>

<style>
  .rows {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-width: 0;
  }
  .tabs {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    border-bottom: 1px solid var(--line);
  }
  [role="tab"] {
    min-height: 40px;
    padding: 0 14px;
    border: 0;
    border-bottom: 2px solid transparent;
    background: none;
    font-family: var(--font-display);
    font-size: 21px;
    color: var(--muted);
  }
  [role="tab"].on {
    color: var(--text);
    border-bottom-color: var(--accent);
  }
  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 340px;
    gap: 18px;
    align-items: start;
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }
  .row {
    width: 100%;
    display: grid;
    grid-template-columns: 120px minmax(0, 1fr) minmax(190px, 0.8fr);
    align-items: center;
    gap: 14px;
    padding: 6px 10px 6px 6px;
    border: 1px solid transparent;
    border-radius: var(--radius);
    background: var(--surface-2);
    text-align: left;
  }
  .row:hover {
    background: var(--surface-3);
  }
  .row.on {
    border-color: var(--accent);
    background: var(--accent-soft);
  }
  .capsule {
    position: relative;
    aspect-ratio: 16 / 9;
    border-radius: var(--radius-s);
    overflow: hidden;
    background: var(--surface-3);
  }
  .main {
    display: flex;
    flex-direction: column;
    gap: 5px;
    min-width: 0;
  }
  .title {
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sub {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .facts {
    display: flex;
    flex-direction: column;
    gap: 1px;
    font-size: 13px;
    color: var(--text-2);
    min-width: 0;
  }
  .facts span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .muted {
    color: var(--muted);
  }
  .empty {
    margin: 0;
    padding: 14px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .preview {
    position: sticky;
    top: 8px;
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 14px;
    border-radius: var(--radius-l);
    background: var(--surface-2);
    border: 1px solid var(--line);
    min-width: 0;
  }
  .shot {
    position: relative;
    aspect-ratio: 16 / 9;
    border-radius: var(--radius);
    overflow: hidden;
    background: var(--surface-3);
  }
  h3 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 26px;
    line-height: 1.1;
    overflow-wrap: anywhere;
  }
  .genres {
    margin: 0;
    font-size: 13.5px;
    color: var(--muted);
  }
  dl {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 6px 14px;
    margin: 0;
    font-size: 13.5px;
  }
  dt {
    color: var(--muted);
  }
  dd {
    margin: 0;
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }
  .actions > * {
    flex-shrink: 1;
    min-width: 0;
    max-width: 100%;
  }
  .actions [aria-pressed="true"] {
    color: var(--accent-text);
  }
  @container (max-width: 900px) {
    .split {
      grid-template-columns: minmax(0, 1fr) 300px;
    }
    .row {
      grid-template-columns: 104px minmax(0, 1fr);
    }
    .facts {
      grid-column: 2;
    }
  }
  @container (max-width: 640px) {
    .split {
      grid-template-columns: minmax(0, 1fr);
    }
    .preview {
      display: none;
    }
    [role="tab"] {
      padding: 0 10px;
      font-size: 18px;
    }
  }
</style>
