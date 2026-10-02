<script lang="ts">
  // Saved games and what happened to them: a first release or a confirmed
  // newer version. Unread activity stays highlighted until it is acknowledged.
  import GameArt from "../../components/GameArt.svelte";
  import Icon from "../../components/Icon.svelte";
  import { api } from "../../lib/api";
  import { shop } from "../../lib/shop.svelte";
  import { lib } from "../../lib/store.svelte";
  import { activityText, cardLine, dateText } from "../../lib/storefront";
  import { storefront } from "../../lib/storefront.svelte";
  import type { GameSummary, WishlistItem } from "../../lib/types";

  let { onopen, onbrowse }: { onopen: (g: GameSummary) => void; onbrowse: () => void } = $props();

  $effect(() => shop.requestArt(storefront.wishlist.map((w) => w.key)));

  async function acknowledge(key: string) {
    const list = await lib.run(() => api.store.wishlist.acknowledge(key));
    if (list) storefront.setWishlist(list);
  }
  function open(w: WishlistItem) {
    if (w.unread) void acknowledge(w.key);
    onopen(w.game);
  }
</script>

{#if storefront.wishlist.length === 0}
  <div class="sf-empty">
    <Icon name="star" size={40} stroke={1.6} />
    <h2>Your wishlist is empty</h2>
    <p>Save a game from its page. Seaglass tells you here when a release shows up or a newer version is confirmed.</p>
    <button type="button" class="sf-primary" onclick={onbrowse}>Browse games</button>
  </div>
{:else}
  <div class="wish">
    <div class="top">
      <p class="sf-muted">{storefront.wishlist.length} saved · {storefront.unread ? `${storefront.unread} new` : "nothing new"}</p>
      {#if storefront.unread}<button type="button" class="sf-btn" onclick={() => acknowledge("")}>Mark all read</button>{/if}
    </div>
    <ul class="list">
      {#each storefront.wishlist as w (w.key)}
        <li class:unread={w.unread > 0}>
          <button type="button" class="open" data-key={w.key} onclick={() => open(w)}>
            <span class="thumb"><GameArt game={{ key: w.key, meta: shop.art[w.key] }} /></span>
            <span class="info">
              <span class="name">{w.title}{#if w.unread}<span class="sf-chip accent">{w.unread} new</span>{/if}</span>
              <span class="sf-muted">{w.game.sourceBacked ? cardLine(storefront.card(w.game)) || "Released by a source" : "No known source release yet"}</span>
              <span class="sf-muted">Saved {dateText(w.addedAt)}</span>
            </span>
          </button>
          {#if w.activity.length}
            <ul class="acts">
              {#each w.activity as a (a.id)}
                <li class:fresh={!a.read}>
                  <Icon name={a.kind === "available" ? "sparkle" : "download"} size={14} stroke={2.2} />
                  <span>{activityText(a)}</span>
                  {#if !a.read}<span class="sf-chip accent">New</span>{/if}
                </li>
              {/each}
            </ul>
          {/if}
          <div class="btns">
            {#if w.unread}<button type="button" class="sf-btn" onclick={() => acknowledge(w.key)}><Icon name="check" size={15} stroke={2.4} />Mark read</button>{/if}
            <button type="button" class="sf-btn" aria-label={`Remove ${w.title} from the wishlist`} onclick={() => storefront.toggleWish({ ...w.game, wishlisted: true })}><Icon name="trash" size={15} />Remove</button>
          </div>
        </li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .wish {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .top {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
  }
  .top p {
    margin: 0;
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .list > li {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px;
    border-radius: var(--radius);
    background: var(--surface-2);
    border-left: 3px solid transparent;
  }
  .list > li.unread {
    border-left-color: var(--accent);
    background: var(--accent-soft);
  }
  .open {
    display: flex;
    gap: 14px;
    padding: 0;
    border: 0;
    background: none;
    text-align: left;
    border-radius: var(--radius-s);
    min-width: 0;
  }
  .thumb {
    position: relative;
    flex: 0 0 64px;
    aspect-ratio: 2 / 3;
    border-radius: var(--radius-s);
    overflow: hidden;
    background: var(--surface-3);
  }
  .info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    justify-content: center;
  }
  .name {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    font-size: 17px;
    font-weight: 700;
  }
  .acts {
    list-style: none;
    margin: 0;
    padding: 0 0 0 78px;
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13.5px;
    color: var(--text-2);
  }
  .acts li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .acts li.fresh {
    color: var(--text);
    font-weight: 600;
  }
  .btns {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    padding-left: 78px;
  }
  @media (max-width: 520px) {
    .acts,
    .btns {
      padding-left: 0;
    }
  }
</style>
