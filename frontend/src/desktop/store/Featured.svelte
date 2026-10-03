<script lang="ts">
  // The top of the front page: a few games in large landscape art. It moves
  // only when the person asks (buttons, dots or the arrow keys).
  import GameArt from "../../components/GameArt.svelte";
  import Icon from "../../components/Icon.svelte";
  import { reviewText, sourceLabel } from "../../lib/storefront";
  import { availabilityText, featuredWhy, mainStoryText, wrapIndex } from "../../lib/storefront-home";
  import { shop } from "../../lib/shop.svelte";
  import { storefront } from "../../lib/storefront.svelte";
  import type { GameSummary } from "../../lib/types";

  let { games, onopen }: { games: GameSummary[]; onopen: (g: GameSummary) => void } = $props();

  let at = $state(0);
  const index = $derived(Math.min(at, Math.max(games.length - 1, 0)));
  const game = $derived(games[index]);
  const g = $derived(game ? storefront.card(game) : null);
  const facts = $derived(g ? [availabilityText(g), g.version, reviewText(g), mainStoryText(g)].filter(Boolean) : []);

  const step = (by: number) => (at = wrapIndex(index, by, games.length));
  function keys(e: KeyboardEvent) {
    if (e.key === "ArrowLeft") step(-1);
    else if (e.key === "ArrowRight") step(1);
    else return;
    e.preventDefault();
  }
</script>

{#if g && game}
  <section class="featured" aria-roledescription="carousel" aria-label="Featured games">
    <button type="button" class="art" tabindex="-1" aria-hidden="true" onclick={() => onopen(game)}>
      <GameArt game={{ key: g.key, meta: shop.art[g.key] }} kind="backdrop" />
    </button>
    <div class="info" aria-live="polite">
      <span class="sf-chip accent">Featured</span>
      <h2>{g.title}</h2>
      <p class="why">{featuredWhy(g)}</p>
      <p class="sources">
        {#each g.sources as s (s)}<span class="sf-chip">{sourceLabel(s)}</span>{/each}
      </p>
      {#if facts.length}<p class="facts">{facts.join(" · ")}</p>{/if}
      <div class="actions">
        <button type="button" class="sf-primary" onclick={() => onopen(game)}>View game</button>
        <button type="button" class="sf-btn" aria-pressed={g.wishlisted} onclick={() => storefront.toggleWish(g)}>
          <Icon name="star" size={15} stroke={2.2} />{g.wishlisted ? "On wishlist" : "Add to wishlist"}
        </button>
      </div>
    </div>
    {#if games.length > 1}
      <div class="nav" role="toolbar" aria-label="Featured games" tabindex="-1" onkeydown={keys}>
        <button type="button" class="sf-btn arrow" aria-label="Previous featured game" onclick={() => step(-1)}><span class="turn left"><Icon name="chevronDown" size={18} stroke={2.4} /></span></button>
        <span class="dots">
          {#each games as f, i (f.key)}
            <button type="button" class="dot" class:on={i === index} aria-label={`Show ${f.title}`} aria-current={i === index} onclick={() => (at = i)}></button>
          {/each}
        </span>
        <button type="button" class="sf-btn arrow" aria-label="Next featured game" onclick={() => step(1)}><span class="turn right"><Icon name="chevronDown" size={18} stroke={2.4} /></span></button>
      </div>
    {/if}
  </section>
{/if}

<style>
  .featured {
    position: relative;
    display: grid;
    grid-template-columns: minmax(0, 1.6fr) minmax(260px, 1fr);
    grid-template-rows: 1fr auto;
    border-radius: var(--radius-l);
    background: var(--surface-2);
    border: 1px solid var(--line);
    overflow: hidden;
  }
  .art {
    position: relative;
    grid-row: 1 / 3;
    padding: 0;
    border: 0;
    background: var(--surface-3);
    min-height: 260px;
    aspect-ratio: 16 / 9;
  }
  .info {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
    padding: 22px 24px 8px;
    min-width: 0;
  }
  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 34px;
    line-height: 1.1;
    overflow-wrap: anywhere;
  }
  p {
    margin: 0;
  }
  .why {
    color: var(--text-2);
  }
  .sources {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
  }
  .facts {
    font-size: 13.5px;
    color: var(--muted);
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin-top: 4px;
  }
  .actions > * {
    flex-shrink: 1;
    min-width: 0;
    max-width: 100%;
  }
  .actions [aria-pressed="true"] {
    color: var(--accent-text);
  }
  .nav {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    padding: 10px 24px 18px;
  }
  .arrow {
    min-height: 34px;
    padding: 0 8px;
  }
  .turn {
    display: flex;
  }
  .turn.left {
    transform: rotate(90deg);
  }
  .turn.right {
    transform: rotate(-90deg);
  }
  .dots {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  /* A small dot with a bigger hit area. */
  .dot {
    position: relative;
    width: 24px;
    height: 24px;
    padding: 0;
    border: 0;
    background: none;
  }
  .dot::after {
    content: "";
    position: absolute;
    inset: 8px;
    border-radius: 50%;
    background: var(--line-strong);
  }
  .dot.on::after {
    background: var(--accent);
  }
  @container (max-width: 640px) {
    .featured {
      grid-template-columns: minmax(0, 1fr);
      grid-template-rows: auto auto auto;
    }
    .art {
      grid-row: auto;
      min-height: 0;
    }
    h2 {
      font-size: 26px;
    }
    .info {
      padding: 16px 16px 4px;
    }
    .nav {
      padding: 8px 16px 14px;
    }
  }
</style>
