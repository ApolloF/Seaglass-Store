<script lang="ts">
  // Saved games and what happened to them, from the couch. Up / down picks a
  // game, left / right one of its buttons: open (which marks it read, as on
  // the desktop), mark read, remove.
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { availabilityText } from "../lib/bpstore";
  import { feedback, useInput } from "../lib/input.svelte";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import { activityText, dateText } from "../lib/storefront";
  import { storefront } from "../lib/storefront.svelte";
  import type { GameSummary, WishlistItem } from "../lib/types";
  import Glyph from "./Glyph.svelte";
  import Hints from "./Hints.svelte";

  let { onopen, onback }: { onopen: (g: GameSummary) => void; onback: () => void } = $props();

  type Row = { kind: "all" } | { kind: "item"; w: WishlistItem };
  type Btn = "open" | "read" | "remove" | "all";
  const rows = $derived<Row[]>([...(storefront.unread ? [{ kind: "all" as const }] : []), ...storefront.wishlist.map((w) => ({ kind: "item" as const, w }))]);
  const buttonsOf = (row: Row | undefined): Btn[] => (!row ? [] : row.kind === "all" ? ["all"] : row.w.unread ? ["open", "read", "remove"] : ["open", "remove"]);
  const keyOf = (row: Row | undefined) => (row?.kind === "item" ? row.w.key : (row?.kind ?? ""));

  let i = $state(0);
  let b = $state(0);
  let at = "";
  // The list changes under the focus (marked read, removed): it stays on its game, else its place.
  $effect(() => {
    const j = rows.findIndex((row) => keyOf(row) === at);
    if (j >= 0) i = j;
    else if (i >= rows.length) i = Math.max(0, rows.length - 1);
    b = Math.min(b, Math.max(0, buttonsOf(rows[i]).length - 1));
  });
  function focus(j: number, k = 0) {
    i = j;
    b = k;
    at = keyOf(rows[j]);
  }
  $effect(() => shop.requestArt(storefront.wishlist.map((w) => w.key)));

  async function acknowledge(key: string) {
    const list = await lib.run(() => api.store.wishlist.acknowledge(key));
    if (list) storefront.setWishlist(list);
  }
  function press(row: Row | undefined, btn: Btn | undefined) {
    if (!row || !btn) return feedback.edge();
    feedback.confirm();
    if (btn === "all") return void acknowledge("");
    if (row.kind !== "item") return;
    if (btn === "open") {
      if (row.w.unread) void acknowledge(row.w.key);
      onopen(row.w.game);
    } else if (btn === "read") void acknowledge(row.w.key);
    else storefront.toggleWish({ ...row.w.game, wishlisted: true });
  }

  let list: HTMLDivElement | undefined = $state();
  $effect(() => {
    list?.querySelector<HTMLElement>(`[data-row="${i}"]`)?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  });

  $effect(() =>
    useInput((intent) => {
      const btns = buttonsOf(rows[i]);
      switch (intent) {
        case "up":
        case "down": {
          const j = i + (intent === "up" ? -1 : 1);
          if (j < 0 || j >= rows.length) return feedback.edge();
          focus(j, Math.min(b, buttonsOf(rows[j]).length - 1));
          feedback.move();
          return;
        }
        case "left":
        case "right": {
          const k = b + (intent === "left" ? -1 : 1);
          if (k < 0 || k >= btns.length) return feedback.edge();
          b = k;
          feedback.move();
          return;
        }
        case "confirm":
          return press(rows[i], btns[b]);
        case "action":
          return rows[i]?.kind === "item" ? press(rows[i], "remove") : feedback.edge();
        case "back":
          onback();
          return;
      }
      return false;
    }),
  );

  const label: Record<Btn, string> = { open: "Open", read: "Mark read", remove: "Remove", all: "Mark all read" };
  const icon = { open: "play", read: "check", remove: "trash", all: "check" } as const;
</script>

<div class="wish">
  <p class="count">
    {storefront.wishlist.length} saved · {storefront.unread ? `${storefront.unread} new` : "nothing new"}
  </p>
  {#if storefront.wishlistReady && storefront.wishlist.length === 0}
    <div class="empty">
      <h2>Your wishlist is empty</h2>
      <p>Save a game with <Glyph button="action" size={28} /> on its card or page. Seaglass tells you here when a release shows up or a newer version is confirmed.</p>
    </div>
  {/if}
  <div class="list" bind:this={list}>
    {#each rows as row, k (keyOf(row))}
      {#if row.kind === "all"}
        <div class="all" data-row={k}>
          <button type="button" class="btn" class:on={k === i} tabindex="-1" onclick={() => (focus(k), press(row, "all"))}><Icon name="check" size={22} />Mark all read</button>
        </div>
      {:else}
        {@const w = row.w}
        {@const g = storefront.card(w.game)}
        <div class="item" class:on={k === i} class:unread={w.unread > 0} data-row={k}>
          <span class="thumb"><GameArt game={{ key: w.key, meta: shop.art[w.key] }} kind="hero" /></span>
          <div class="info">
            <span class="name">{w.title}{#if w.unread}<span class="new">{w.unread} new</span>{/if}</span>
            <span class="line">{availabilityText(g)} · Saved {dateText(w.addedAt)}{w.origin === "steam" ? " · From your Steam wishlist" : ""}</span>
            {#each w.activity.slice(0, 3) as a (a.id)}
              <span class="act" class:fresh={!a.read}><Icon name={a.kind === "available" ? "sparkle" : "download"} size={18} stroke={2.2} />{activityText(a)}</span>
            {/each}
          </div>
          <div class="btns">
            {#each buttonsOf(row) as btn, n (btn)}
              <button type="button" class="btn" class:on={k === i && n === b} tabindex="-1" onclick={() => (focus(k, n), press(row, btn))}><Icon name={icon[btn]} size={20} />{label[btn]}</button>
            {/each}
          </div>
        </div>
      {/if}
    {/each}
  </div>
  <div class="hints">
    <Hints
      hints={[
        ...(rows.length ? [{ button: "confirm" as const, label: label[buttonsOf(rows[i])[b] ?? "open"] }] : []),
        ...(rows[i]?.kind === "item" ? [{ button: "action" as const, label: "Remove" }] : []),
        { button: "info", label: "Search" },
        { button: "lt", also: "rt", label: "Tabs" },
        { button: "back", label: "Back" },
      ]}
    />
  </div>
</div>

<style>
  .wish {
    position: absolute;
    inset: 0;
  }
  .count {
    position: absolute;
    left: 110px;
    top: 112px;
    margin: 0;
    font-size: 18px;
    color: #9ba8b5;
  }
  .empty {
    position: absolute;
    left: 110px;
    top: 170px;
    max-width: 900px;
  }
  .empty h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 44px;
  }
  .empty p {
    font-size: 22px;
    line-height: 1.45;
    color: #9ba8b5;
  }
  .empty p :global(span) {
    vertical-align: middle;
  }
  .list {
    position: absolute;
    left: 110px;
    right: 110px;
    top: 150px;
    bottom: 120px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 6px;
    mask-image: linear-gradient(180deg, transparent 0, #000 12px, #000 calc(100% - 40px), transparent 100%);
    scroll-padding-block: 18px 48px;
  }
  .all {
    flex-shrink: 0;
  }
  .item {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 26px;
    padding: 16px 20px;
    border-radius: 18px;
    background: #121a23;
  }
  .item.on {
    background: #1b2632;
  }
  .item.unread {
    box-shadow: inset 4px 0 0 oklch(0.8 0.12 205);
  }
  .thumb {
    position: relative;
    flex-shrink: 0;
    width: 280px;
    height: 132px;
    border-radius: 12px;
    overflow: hidden;
    background: #0a0e13;
  }
  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .name {
    display: flex;
    align-items: center;
    gap: 14px;
    font-size: 26px;
    font-weight: 700;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .new {
    padding: 3px 10px;
    border-radius: 8px;
    background: oklch(0.8 0.12 205);
    color: #06080b;
    font-size: 15px;
    font-weight: 800;
  }
  .line {
    font-size: 18px;
    color: #9ba8b5;
  }
  .act {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 18px;
    color: #7f8c99;
  }
  .act.fresh {
    color: oklch(0.88 0.09 205);
  }
  .btns {
    display: flex;
    gap: 12px;
    flex-shrink: 0;
  }
  .btn {
    height: 60px;
    display: inline-flex;
    align-items: center;
    gap: 10px;
    padding: 0 22px;
    border-radius: 30px;
    border: 1px solid rgba(255, 255, 255, 0.2);
    background: rgba(14, 18, 24, 0.6);
    color: #f3f5f7;
    font-size: 20px;
    font-weight: 700;
    white-space: nowrap;
  }
  .btn.on {
    background: #f3f5f7;
    color: #06080b;
    border-color: transparent;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 40px;
    left: 110px;
  }
</style>
