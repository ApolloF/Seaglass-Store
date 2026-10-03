<script lang="ts">
  // One Store game as a landscape card: its art, then the title, where it
  // comes from and whether it can be got, its reviews and completion time.
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import { availabilityText, factsText, sourcesText } from "../lib/bpstore";
  import { shop } from "../lib/shop.svelte";
  import { storefront } from "../lib/storefront.svelte";
  import type { GameSummary } from "../lib/types";

  let {
    game,
    on,
    width,
    height,
    big = false,
    because = "",
    onclick,
  }: { game: GameSummary; on: boolean; width: number; height: number; big?: boolean; because?: string; onclick: () => void } = $props();

  const g = $derived(storefront.card(game));
  const facts = $derived(factsText(g));
  const where = $derived(g.sourceBacked ? sourcesText(g) : "Steam only · wishlist only");
  const downloading = $derived(shop.downloads.some((d) => d.gameKey === g.key && d.state !== "failed" && d.state !== "installed"));
</script>

<button type="button" class="card" class:on class:big tabindex="-1" style:width="{width}px" {onclick} aria-label={g.title}>
  <span class="art" style:height="{height}px">
    <GameArt game={{ key: g.key, meta: shop.art[g.key] }} kind="hero" />
    {#if !shop.art[g.key]?.hero && !shop.art[g.key]?.cover}<span class="at">{g.title}</span>{/if}
    {#if downloading}<span class="flag"><Icon name="download" size={18} stroke={2.4} />In Downloads</span>
    {:else if g.installed?.update}<span class="flag"><Icon name="sparkle" size={18} stroke={2.4} />Update</span>
    {:else if g.installed}<span class="flag quiet"><Icon name="check" size={18} stroke={2.4} />Installed</span>{/if}
    {#if g.activity}<span class="flag top"><Icon name="star" size={18} stroke={2.4} />New</span>
    {:else if g.wishlisted}<span class="flag top quiet"><Icon name="star" size={18} stroke={2.4} /></span>{/if}
  </span>
  <span class="title">{g.title}</span>
  <span class="line">{[where, availabilityText(g)].filter(Boolean).join(" · ")}</span>
  {#if because}<span class="line accent">{because}</span>{:else if facts}<span class="line">{facts}</span>{/if}
</button>

<style>
  .card {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 0;
    border: 0;
    background: none;
    color: #9ba8b5;
    text-align: left;
  }
  .art {
    position: relative;
    display: block;
    width: 100%;
    border-radius: 14px;
    overflow: hidden;
    background: #121a23;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
    transition:
      transform 0.22s cubic-bezier(0.2, 0.8, 0.2, 1),
      box-shadow 0.22s;
  }
  .card.on .art {
    transform: scale(1.04);
    box-shadow:
      0 0 0 4px #fff,
      0 18px 44px rgba(0, 0, 0, 0.55);
  }
  .at {
    position: absolute;
    left: 18px;
    right: 18px;
    bottom: 16px;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 34px;
    line-height: 1;
    color: #fff;
    text-shadow: 0 2px 12px rgba(0, 0, 0, 0.6);
  }
  .big .at {
    font-size: 56px;
  }
  .flag {
    position: absolute;
    left: 12px;
    top: 12px;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 5px 11px;
    border-radius: 8px;
    background: oklch(0.8 0.12 205);
    color: #06080b;
    font-size: 15px;
    font-weight: 800;
  }
  .flag.quiet {
    background: rgba(8, 12, 16, 0.82);
    color: #fff;
  }
  .flag.top {
    left: auto;
    right: 12px;
  }
  .title {
    margin-top: 8px;
    font-size: 22px;
    font-weight: 700;
    color: #e8edf2;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .big .title {
    font-size: 30px;
  }
  .card.on .title {
    color: #fff;
  }
  .line {
    font-size: 17px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .line.accent {
    color: oklch(0.88 0.09 205);
  }
</style>
