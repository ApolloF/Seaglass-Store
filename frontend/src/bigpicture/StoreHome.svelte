<script lang="ts">
  // The Store's front page from the couch: featured games, then shelves of
  // landscape art. Up / down picks a shelf, left / right a game; each shelf
  // keeps its place. Shelves fetched again (new releases, a wishlist change)
  // keep focus on the same game.
  import { api } from "../lib/api";
  import { focusKey, homeNotes, homeShelves, restoreFocus, shelfCol, shelfMove, startFocus, type Shelf } from "../lib/bpstore";
  import { feedback, useInput } from "../lib/input.svelte";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import { statusSummary, storeMode } from "../lib/storefront";
  import { storefront } from "../lib/storefront.svelte";
  import type { GameSummary } from "../lib/types";
  import Hints from "./Hints.svelte";
  import { rowOffset } from "./nav";
  import StoreCard from "./StoreCard.svelte";

  let { height, onopen, onback }: { height: number; onopen: (g: GameSummary) => void; onback: () => void } = $props();

  let shelves = $state<Shelf[]>([]);
  let notes = $state<string[]>([]);
  let loaded = $state(false);
  let f = $state(startFocus());
  let version = $state(0);
  let seq = 0;

  $effect(() => api.store.discovery.onGames(() => version++));
  // Fetched again when games or the wishlist change; the old shelves stay until the new ones arrive.
  $effect(() => {
    void version;
    void storefront.wishlist;
    const mine = ++seq;
    lib.run(() => api.store.discovery.home()).then((h) => {
      if (mine !== seq) return;
      loaded = true;
      if (!h) return;
      const at = focusKey(shelves, f);
      const next = homeShelves(h);
      f = restoreFocus(next, at, f);
      shelves = next;
      notes = homeNotes(h);
    });
  });
  $effect(() => {
    const keys = shelves.flatMap((s) => s.games.map((g) => g.key));
    shop.requestArt(keys);
    storefront.ask(keys);
  });

  const mode = $derived(storeMode({ loaded, status: storefront.status, shown: shelves.reduce((n, s) => n + s.games.length, 0) }));
  const summary = $derived(statusSummary(storefront.status));

  // Card sizes in design pixels (1920 wide); featured games are bigger.
  const W = 440;
  const H = 206;
  const BIG_W = 900;
  const BIG_H = 380;
  const GAP = 28;
  const TEXT = 112;
  const HEAD = 56;
  const ROW_GAP = 34;
  const size = (s: Shelf) => (s.big ? { w: BIG_W, h: BIG_H } : { w: W, h: H });
  const tops = $derived(shelves.reduce<number[]>((t, s, i) => [...t, i ? t[i - 1] + HEAD + size(shelves[i - 1]).h + TEXT + ROW_GAP : 0], []));
  const total = $derived(shelves.length ? tops[tops.length - 1] + HEAD + size(shelves[shelves.length - 1]).h + TEXT : 0);
  const TOP = 150;
  const BOTTOM = 110;
  const viewH = $derived(height - TOP - BOTTOM);
  const offY = $derived(Math.max(0, Math.min(tops[f.row] ?? 0, total - viewH + 24)));

  const focused = $derived.by(() => {
    const s = shelves[f.row];
    return s ? s.games[shelfCol(f, s.games.length)] : undefined;
  });
  const wished = $derived(focused ? storefront.wished(focused.key, focused.wishlisted) : false);

  function pick(row: number, col: number) {
    const cols = [...f.cols];
    cols[row] = col;
    f = { row, cols };
  }

  $effect(() =>
    useInput((intent) => {
      switch (intent) {
        case "up":
        case "down":
        case "left":
        case "right": {
          const next = shelfMove(f, shelves.map((s) => s.games.length), intent);
          if (next) ((f = next), feedback.move());
          else feedback.edge();
          return;
        }
        case "confirm":
          if (focused) (feedback.confirm(), onopen(focused));
          else feedback.edge();
          return;
        case "action":
          if (focused) (feedback.confirm(), storefront.toggleWish(storefront.card(focused)));
          else feedback.edge();
          return;
        case "back":
          onback();
          return;
      }
      return false;
    }),
  );
</script>

<div class="home">
  {#if mode === "ready"}
    {#if summary.text || notes.length}
      <p class="status" class:warn={summary.tone === "warn"}>{[summary.text, ...notes].filter(Boolean).join(" · ")}</p>
    {/if}
    <div class="view" style:top="{TOP - 24}px" style:bottom="{BOTTOM}px">
      <div class="rows" style:transform="translateY({-offY}px)">
        {#each shelves as s, ri (s.id)}
          {@const sz = size(s)}
          {@const col = shelfCol(f, s.games.length)}
          <section class="shelf" style:top="{tops[ri]}px" aria-label={s.title}>
            <h2 class:on={ri === f.row}>{s.title}{#if s.note}<span class="note">{s.note}</span>{/if}</h2>
            <div class="track" style:transform="translateX({rowOffset(col, sz.w, GAP)}px)">
              {#each s.games as g, ci (g.key)}
                <StoreCard
                  game={g}
                  on={ri === f.row && ci === col}
                  width={sz.w}
                  height={sz.h}
                  big={s.big}
                  because={s.because[g.key] ?? ""}
                  onclick={() => (ri === f.row && ci === col ? onopen(g) : pick(ri, ci))}
                />
              {/each}
            </div>
          </section>
        {/each}
      </div>
    </div>
  {:else}
    <div class="empty">
      {#if mode === "loading"}
        <p>Loading the Store…</p>
      {:else if mode === "setup"}
        <h2>Choose your sources</h2>
        <p>The Store needs to know where to look for games. Choose your sources in desktop mode, on the Store page.</p>
      {:else if mode === "off"}
        <h2>Source browsing is off</h2>
        <p>Turn it on in desktop mode, in Settings → Experimental, to see games from your sources here.</p>
      {:else if mode === "unreachable"}
        <h2>The sources can't be reached</h2>
        <p>Seaglass tries again on its own. Games show here once a source answers.</p>
      {:else}
        <h2>Finding games</h2>
        <p>Seaglass is reading your sources. Games show here as they're found.</p>
      {/if}
    </div>
  {/if}
  <div class="hints">
    <Hints
      hints={[
        ...(focused ? [{ button: "confirm" as const, label: "Open" }, { button: "action" as const, label: wished ? "Remove from wishlist" : "Add to wishlist" }] : []),
        { button: "info", label: "Search" },
        { button: "lt", also: "rt", label: "Tabs" },
        { button: "back", label: "Back" },
      ]}
    />
  </div>
</div>

<style>
  .home {
    position: absolute;
    inset: 0;
  }
  .status {
    position: absolute;
    left: 110px;
    top: 112px;
    margin: 0;
    font-size: 18px;
    color: #9ba8b5;
  }
  .status.warn {
    color: #f3b35a;
  }
  /* Room above the first shelf for the focused card to grow into; the
     shelves fade out before the prompts. */
  .view {
    position: absolute;
    left: 0;
    right: 0;
    overflow: hidden;
    mask-image: linear-gradient(180deg, transparent 0, #000 24px, #000 calc(100% - 50px), transparent 100%);
  }
  .rows {
    position: absolute;
    left: 0;
    right: 0;
    top: 24px;
    transition: transform 0.4s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .shelf {
    position: absolute;
    left: 110px;
    right: 0;
  }
  h2 {
    display: flex;
    align-items: baseline;
    gap: 16px;
    height: 56px;
    margin: 0;
    font-size: 26px;
    font-weight: 700;
    color: #9ba8b5;
  }
  h2.on {
    color: #f3f5f7;
  }
  .note {
    font-size: 18px;
    font-weight: 500;
    color: #7f8c99;
  }
  .track {
    display: flex;
    gap: 28px;
    transition: transform 0.35s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .empty {
    position: absolute;
    left: 110px;
    top: 170px;
    max-width: 900px;
  }
  .empty h2 {
    height: auto;
    font-family: var(--font-display);
    font-size: 44px;
    color: #f3f5f7;
  }
  .empty p {
    font-size: 22px;
    line-height: 1.45;
    color: #9ba8b5;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 40px;
    left: 110px;
  }
</style>
