<script lang="ts">
  // Steam review text for a game, a page at a time. Shown as plain text;
  // the original is opened through the backend, never from here.
  import { untrack } from "svelte";
  import Icon from "../../components/Icon.svelte";
  import { api } from "../../lib/api";
  import { lib } from "../../lib/store.svelte";
  import { dateText, hoursText } from "../../lib/storefront";
  import type { Review, ReviewPage } from "../../lib/types";

  let { appId }: { appId: number } = $props();

  let filter = $state<"helpful" | "recent">("helpful");
  let reviews = $state<Review[]>([]);
  let cursor = $state("");
  let more = $state(false);
  let loading = $state(false);
  let failed = $state("");
  let seq = 0;

  async function load(fresh: boolean) {
    const mine = ++seq;
    loading = true;
    failed = "";
    if (fresh) [reviews, cursor, more] = [[], "", false];
    let page: ReviewPage | undefined;
    try {
      page = await api.store.enrich.reviews({ appId, cursor: fresh ? "" : cursor, filter, language: "" });
    } catch (e) {
      if (mine === seq) failed = e instanceof Error ? e.message : "Reviews couldn't be loaded.";
    }
    if (mine !== seq) return;
    loading = false;
    if (!page) return;
    if (page.state === "error" || page.state === "unavailable") failed = page.error || "Reviews aren't available right now.";
    reviews = fresh ? page.reviews : [...reviews, ...page.reviews];
    cursor = page.cursor;
    more = page.more;
  }

  // Starts again when the game or the order changes.
  $effect(() => {
    void appId;
    void filter;
    untrack(() => void load(true));
  });
  const open = (url: string) => lib.run(() => api.store.enrich.openLink(url));
</script>

<section class="reviews" aria-labelledby="sf-reviews">
  <div class="head">
    <h2 class="sf-h2" id="sf-reviews">Steam reviews</h2>
    <div class="seg" role="group" aria-label="Order reviews">
      <button type="button" class:on={filter === "helpful"} aria-pressed={filter === "helpful"} onclick={() => (filter = "helpful")}>Helpful</button>
      <button type="button" class:on={filter === "recent"} aria-pressed={filter === "recent"} onclick={() => (filter = "recent")}>Recent</button>
    </div>
  </div>
  {#if failed}<p class="sf-muted err" role="status">{failed}{reviews.length ? " Showing what was loaded." : ""}</p>{/if}
  <ul class="list">
    {#each reviews as r (r.id)}
      <li>
        <div class="meta">
          <span class="sf-chip" class:accent={r.recommended} class:warn={!r.recommended}>{r.recommended ? "Recommended" : "Not recommended"}</span>
          <span class="author">{r.author}</span>
          <span class="sf-muted">{[r.playtimeAtReview > 0 && `${hoursText(r.playtimeAtReview)} at review`, r.posted > 0 && `Posted ${dateText(r.posted)}`].filter(Boolean).join(" · ")}</span>
        </div>
        <p class="text">{r.text}</p>
        <div class="foot">
          <span class="sf-muted">{r.helpful.toLocaleString("en")} {r.helpful === 1 ? "person finds" : "people find"} this helpful</span>
          {#if r.url}<button type="button" class="sf-link" onclick={() => open(r.url)}>Read on Steam</button>{/if}
        </div>
      </li>
    {/each}
  </ul>
  {#if loading}
    <p class="sf-muted" aria-busy="true">Loading reviews…</p>
  {:else if more}
    <button type="button" class="sf-btn more" onclick={() => load(false)}><Icon name="chevronDown" size={15} stroke={2.2} />Load more reviews</button>
  {:else if !reviews.length && !failed}
    <p class="sf-muted">No reviews to show.</p>
  {/if}
</section>

<style>
  .reviews {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-top: 10px;
  }
  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
  }
  .seg {
    display: flex;
    gap: 2px;
    padding: 3px;
    border-radius: 10px;
    background: var(--surface-2);
  }
  .seg button {
    height: 32px;
    padding: 0 14px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    font-size: 14px;
    font-weight: 700;
  }
  .seg button.on {
    background: var(--surface-3);
    color: var(--text);
  }
  .err {
    margin: 0;
    color: var(--warn);
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .list li {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 12px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .meta,
  .foot {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 10px;
  }
  .author {
    font-weight: 700;
    overflow-wrap: anywhere;
  }
  .text {
    margin: 0;
    color: var(--text-2);
    line-height: 1.5;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    user-select: text;
    -webkit-user-select: text;
  }
  .more {
    align-self: flex-start;
  }
</style>
