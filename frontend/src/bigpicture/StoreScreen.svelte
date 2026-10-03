<script lang="ts">
  // Big picture's Store: Home, Browse and Wishlist, switched with L2 / R2;
  // Y goes to Browse from the other two. A game opens over the tab, which
  // stays as it was underneath, so B lands back on the same card. After a
  // download is queued its progress shows over the game; B returns to it.
  import { STORE_TABS, type Section, type StoreTab } from "../lib/bpstore";
  import { feedback, useInput } from "../lib/input.svelte";
  import { shop } from "../lib/shop.svelte";
  import { storefront } from "../lib/storefront.svelte";
  import type { GameSummary } from "../lib/types";
  import BPDownloads from "./BPDownloads.svelte";
  import Glyph from "./Glyph.svelte";
  import Sections from "./Sections.svelte";
  import StoreBrowse from "./StoreBrowse.svelte";
  import StoreDetails from "./StoreDetails.svelte";
  import StoreHome from "./StoreHome.svelte";
  import StoreWishlist from "./StoreWishlist.svelte";

  let { width, height, active, onback, onsection }: { width: number; height: number; active: boolean; onback: () => void; onsection: (s: Section) => void } = $props();

  $effect(() => shop.start());
  $effect(() => storefront.start());

  let tab = $state<StoreTab>("home");
  let opened = $state<GameSummary | null>(null);
  let downloads = $state(false);

  const open = (g: GameSummary) => (opened = g);
  const home = () => ((tab = "home"), feedback.move());

  // Under the tabs: whatever a tab doesn't use goes on to the shell (L1 / R1, menu).
  $effect(() =>
    useInput((intent) => {
      if (opened) return false;
      if (intent === "lt" || intent === "rt") {
        const i = STORE_TABS.findIndex((t) => t.id === tab);
        tab = STORE_TABS[(i + (intent === "lt" ? STORE_TABS.length - 1 : 1)) % STORE_TABS.length].id;
        feedback.move();
        return;
      }
      if (intent === "info" && tab !== "browse") {
        tab = "browse";
        feedback.move();
        return;
      }
      return false;
    }),
  );
</script>

<div class="store">
  <div class="head">
    <Sections current="store" onpick={onsection} />
    {#if storefront.unread}<span class="unread">{storefront.unread} new on your wishlist</span>{/if}
    <div class="tabs">
      <Glyph button="lt" size={28} />
      {#each STORE_TABS as t (t.id)}
        <button type="button" tabindex="-1" class:on={t.id === tab} onclick={() => (tab = t.id)}>{t.label}</button>
      {/each}
      <Glyph button="rt" size={28} />
    </div>
  </div>

  {#if tab === "home"}
    <StoreHome {height} onopen={open} {onback} />
  {:else if tab === "browse"}
    <StoreBrowse {width} {height} active={active && !opened} onopen={open} onback={home} />
  {:else}
    <StoreWishlist onopen={open} onback={home} />
  {/if}

  {#if opened}
    {#key opened.key}
      <StoreDetails game={opened} onclose={() => ((opened = null), feedback.move())} ondownloads={() => (downloads = true)} />
    {/key}
  {/if}
  {#if downloads}
    <div class="downloads"><BPDownloads onback={() => ((downloads = false), feedback.move())} /></div>
  {/if}
</div>

<style>
  .store {
    position: absolute;
    inset: 0;
    background: #0a0e13;
    color: #e8edf2;
    overflow: hidden;
  }
  .head {
    position: absolute;
    left: 110px;
    right: 110px;
    top: 44px;
    height: 60px;
    display: flex;
    align-items: center;
    gap: 22px;
    z-index: 2;
  }
  .unread {
    font-size: 19px;
    font-weight: 700;
    color: oklch(0.88 0.09 205);
  }
  .tabs {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .tabs > :global(:first-child) {
    margin-right: 8px;
  }
  .tabs > :global(:last-child) {
    margin-left: 8px;
  }
  .tabs button {
    white-space: nowrap;
    height: 44px;
    padding: 0 18px;
    border: 0;
    border-radius: 22px;
    background: transparent;
    color: #9ba8b5;
    font-size: 19px;
    font-weight: 700;
  }
  .tabs button.on {
    background: rgba(79, 209, 232, 0.16);
    color: oklch(0.88 0.09 205);
  }
  .downloads {
    position: absolute;
    inset: 0;
    z-index: 40;
  }
</style>
