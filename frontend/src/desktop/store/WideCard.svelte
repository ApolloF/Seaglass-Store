<script lang="ts">
  // One game as a landscape capsule on a shelf. The whole card opens the game's page.
  import GameArt from "../../components/GameArt.svelte";
  import Icon from "../../components/Icon.svelte";
  import { reviewText } from "../../lib/storefront";
  import { availabilityText, mainStoryText } from "../../lib/storefront-home";
  import { shop } from "../../lib/shop.svelte";
  import { storefront } from "../../lib/storefront.svelte";
  import type { GameSummary } from "../../lib/types";

  let {
    game,
    why = "",
    onopen,
    downloading = false,
  }: { game: GameSummary; why?: string; onopen: (g: GameSummary) => void; downloading?: boolean } = $props();

  const g = $derived(storefront.card(game));
  const facts = $derived([reviewText(g), mainStoryText(g)].filter(Boolean).join(" · "));
</script>

<button type="button" class="card" data-key={g.key} onclick={() => onopen(game)}>
  <span class="cover">
    <GameArt game={{ key: g.key, meta: shop.art[g.key] }} kind="hero" />
    {#if downloading}<span class="flag"><Icon name="download" size={14} stroke={2.4} />In Downloads</span>
    {:else if g.installed?.update}<span class="flag"><Icon name="sparkle" size={14} stroke={2.4} />Update</span>
    {:else if g.activity}<span class="flag"><Icon name="star" size={14} stroke={2.4} />New</span>{/if}
    {#if g.popularRank > 0}<span class="rank" title="Place on Steam's most-played chart">#{g.popularRank}</span>{/if}
  </span>
  <span class="title">{g.title}</span>
  {#if why}<span class="line why">{why}</span>{/if}
  <span class="line">{availabilityText(g)}{g.version ? ` · ${g.version}` : ""}</span>
  {#if facts}<span class="line review">{facts}</span>{/if}
</button>

<style>
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
    min-width: 0;
  }
  .cover {
    position: relative;
    aspect-ratio: 16 / 9;
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
  .line.why,
  .line.review {
    color: var(--text-2);
  }
</style>
