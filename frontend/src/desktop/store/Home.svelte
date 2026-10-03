<script lang="ts">
  // The front page: featured games, news about wishlisted games, what to play
  // next, and the new, popular and updated releases as a list with a preview.
  import { api } from "../../lib/api";
  import { shop } from "../../lib/shop.svelte";
  import { lib } from "../../lib/store.svelte";
  import { storefront } from "../../lib/storefront.svelte";
  import { recommendationTitle, recommendationWhy } from "../../lib/storefront-home";
  import type { GameSummary, StoreHome } from "../../lib/types";
  import Featured from "./Featured.svelte";
  import Rows from "./Rows.svelte";
  import Shelf from "./Shelf.svelte";
  import WideCard from "./WideCard.svelte";

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
    const keys = [...home.featured, ...home.recommended.map((r) => r.game), ...home.new, ...home.popular, ...home.updated, ...home.wishlist].map((g) => g.key);
    shop.requestArt(keys);
    storefront.ask(keys);
  });
</script>

{#snippet cards(list: GameSummary[])}
  {#each list as g (g.key)}
    <li><WideCard game={g} {onopen} downloading={downloading.has(g.key)} /></li>
  {/each}
{/snippet}

{#if home}
  <div class="home">
    <Featured games={home.featured} {onopen} />
    {#if home.wishlist.length}
      <Shelf title="Wishlist activity" count={home.wishlist.length} wide>{@render cards(home.wishlist)}</Shelf>
    {/if}
    {#if home.recommended.length}
      <Shelf title={recommendationTitle(home.recommendedBasis, home.recommended)} count={home.recommended.length} wide>
        {#each home.recommended as r (r.game.key)}
          <li><WideCard game={r.game} why={recommendationWhy(r)} {onopen} downloading={downloading.has(r.game.key)} /></li>
        {/each}
      </Shelf>
    {/if}
    <Rows {home} {downloading} {onopen} />
  </div>
{:else}
  <div class="home" aria-busy="true" aria-label="Loading">
    {#each [0, 1] as i (i)}
      <div class="ph"><span class="bar"></span><div class="row">{#each [0, 1, 2, 3, 4, 5] as j (j)}<span class="card"></span>{/each}</div></div>
    {/each}
  </div>
{/if}

<style>
  /* The parts size themselves to the page, not the window: the sidebar takes room too. */
  .home {
    container-type: inline-size;
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
