<script lang="ts">
  // One game on a shelf or in a grid. The whole card opens the game's page.
  import GameArt from "../../components/GameArt.svelte";
  import Icon from "../../components/Icon.svelte";
  import { cardLine, reviewText, sourceLabel } from "../../lib/storefront";
  import { shop } from "../../lib/shop.svelte";
  import { storefront } from "../../lib/storefront.svelte";
  import type { GameSummary } from "../../lib/types";

  let {
    game,
    onopen,
    onwish,
    downloading = false,
  }: { game: GameSummary; onopen: (g: GameSummary) => void; onwish?: (g: GameSummary) => void; downloading?: boolean } = $props();

  const g = $derived(storefront.card(game));
  const review = $derived(reviewText(g));
  const names = $derived(g.sources.map(sourceLabel).join(" · "));
</script>

<div class="wrap">
  <button type="button" class="card" data-key={g.key} onclick={() => onopen(game)}>
    <span class="cover">
      <GameArt game={{ key: g.key, meta: shop.art[g.key] }} />
      {#if downloading}<span class="flag"><Icon name="download" size={14} stroke={2.4} />In Downloads</span>
      {:else if g.installed?.update}<span class="flag"><Icon name="sparkle" size={14} stroke={2.4} />Update</span>
      {:else if g.installed}<span class="flag quiet"><Icon name="check" size={14} stroke={2.4} />Installed</span>{/if}
      {#if g.activity}<span class="flag top"><Icon name="star" size={14} stroke={2.4} />New</span>{:else if g.wishlisted}<span class="flag top quiet" title="On your wishlist"><Icon name="star" size={14} stroke={2.4} /></span>{/if}
      {#if g.popularRank > 0}<span class="rank" title="Place on Steam's most-played chart">#{g.popularRank}</span>{/if}
    </span>
    <span class="title">{g.title}</span>
    <span class="line">{cardLine(g)}</span>
    {#if names}<span class="line">{names}</span>{/if}
    {#if review}<span class="line review">{review}</span>{/if}
  </button>
  {#if onwish}
    <button type="button" class="sf-btn wish" aria-pressed={g.wishlisted} onclick={() => onwish(g)}>
      <Icon name="star" size={15} stroke={2.2} />{g.wishlisted ? "On wishlist" : "Add to wishlist"}
    </button>
  {/if}
</div>

<style>
  .wrap {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-width: 0;
  }
  .card {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    border-radius: var(--radius);
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
  .flag.top {
    top: 8px;
    bottom: auto;
    left: auto;
    right: 8px;
  }
  .flag.quiet {
    background: var(--surface);
    color: var(--text-2);
  }
  .rank {
    position: absolute;
    right: 8px;
    bottom: 8px;
    padding: 3px 8px;
    border-radius: 999px;
    background: var(--scrim);
    color: #fff;
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
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .line.review {
    color: var(--text-2);
  }
  .wish {
    min-height: 34px;
    font-size: 13px;
  }
  .wish[aria-pressed="true"] {
    color: var(--accent-text);
  }
</style>
