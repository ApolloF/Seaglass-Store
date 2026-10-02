<script lang="ts">
  // The front page: new repacks, what's popular on Steam, recent updates and
  // news about wishlisted games.
  import { api } from "../../lib/api";
  import { shop } from "../../lib/shop.svelte";
  import { lib } from "../../lib/store.svelte";
  import { storefront } from "../../lib/storefront.svelte";
  import type { GameSummary, StoreHome } from "../../lib/types";
  import GameCard from "./GameCard.svelte";
  import Shelf from "./Shelf.svelte";

  let {
    version,
    downloading,
    onopen,
  }: { version: number; downloading: Set<string>; onopen: (g: GameSummary) => void } = $props();

  let home = $state<StoreHome | null>(null);
  let seq = 0;

  // Refetched when games change; the old shelves stay until the new ones arrive.
  $effect(() => {
    void version;
    void storefront.wishlist;
    const mine = ++seq;
    lib.run(() => api.store.discovery.home()).then((h) => {
      if (!h || mine !== seq) return;
      home = h;
    });
  });
  $effect(() => {
    if (!home) return;
    const keys = [...home.new, ...home.popular, ...home.updated, ...home.wishlist].map((g) => g.key);
    shop.requestArt(keys);
    storefront.ask(keys);
  });
</script>

{#snippet cards(list: GameSummary[])}
  {#each list as g (g.key)}
    <li><GameCard game={g} {onopen} downloading={downloading.has(g.key)} /></li>
  {/each}
{/snippet}

{#if home}
  <div class="home">
    {#if home.wishlist.length}
      <Shelf title="Wishlist activity" count={home.wishlist.length}>{@render cards(home.wishlist)}</Shelf>
    {/if}
    <Shelf title="New repacks" count={home.new.length} empty="No releases yet.">{@render cards(home.new)}</Shelf>
    <Shelf
      title="Popular"
      count={home.popular.length}
      note={home.popularState === "stale" ? "Cached chart" : ""}
      empty={home.popularState === "unavailable" ? "Steam's most-played chart isn't available right now." : "None of the games on Steam's most-played chart have a release here yet."}
    >
      {@render cards(home.popular)}
    </Shelf>
    <Shelf title="Recently updated" count={home.updated.length} empty="No releases were changed recently.">{@render cards(home.updated)}</Shelf>
  </div>
{:else}
  <div class="home" aria-busy="true" aria-label="Loading">
    {#each [0, 1] as i (i)}
      <div class="ph"><span class="bar"></span><div class="row">{#each [0, 1, 2, 3, 4, 5] as j (j)}<span class="card"></span>{/each}</div></div>
    {/each}
  </div>
{/if}

<style>
  .home {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }
  .ph {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .bar {
    width: 160px;
    height: 24px;
    border-radius: 6px;
    background: var(--surface-2);
  }
  .row {
    display: flex;
    gap: 16px;
    overflow: hidden;
  }
  .card {
    flex: 0 0 132px;
    aspect-ratio: 2 / 3;
    border-radius: var(--radius);
    background: var(--surface-2);
    animation: pulse 1.4s ease-in-out infinite;
  }
  @keyframes pulse {
    50% {
      opacity: 0.55;
    }
  }
</style>
