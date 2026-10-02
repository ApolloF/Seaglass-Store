<script lang="ts">
  // Steam review summaries, the Metacritic score Steam gives, and
  // HowLongToBeat times. Nothing is shown that the providers didn't give.
  import { api } from "../../lib/api";
  import { lib } from "../../lib/store.svelte";
  import { countText, freshness, hoursText } from "../../lib/storefront";
  import type { Enrichment, ReviewScore } from "../../lib/types";

  let { e, onpick }: { e: Enrichment | null; onpick: () => void } = $props();

  const open = (url: string) => lib.run(() => api.store.enrich.openLink(url));
  const scoreLine = (s: ReviewScore) => [s.label, s.percent ? `${s.percent}%` : "", s.total ? countText(s.total, "review") : ""].filter(Boolean).join(" · ");
  const times = $derived(
    e && e.completion.hltbId
      ? ([
          ["Main Story", e.completion.main],
          ["Main + Extras", e.completion.mainExtras],
          ["Completionist", e.completion.completionist],
        ] as [string, number][])
      : [],
  );
</script>

<div class="ratings">
  <section class="box" aria-labelledby="sf-r-steam">
    <h3 id="sf-r-steam">Steam reviews</h3>
    {#if !e}
      <p class="sf-muted" aria-busy="true">Loading…</p>
    {:else if e.reviews.state === "loading"}
      <p class="sf-muted" aria-busy="true">Loading…</p>
    {:else if e.reviews.state === "unavailable" || e.reviews.state === "error" || !e.reviews.appId}
      <p class="sf-muted">{e.reviews.error || "No Steam reviews are available for this game."}</p>
    {:else}
      <dl>
        <dt>Overall</dt><dd>{scoreLine(e.reviews.overall) || "No user reviews"}</dd>
        <dt>Recent</dt><dd>{scoreLine(e.reviews.recent) || "No recent reviews"}</dd>
      </dl>
      {#if freshness(e.reviews.state, e.reviews.fetchedAt)}<p class="sf-muted">{freshness(e.reviews.state, e.reviews.fetchedAt)}{e.reviews.error ? `. ${e.reviews.error}` : ""}</p>{/if}
      {#if e.reviews.url}<button type="button" class="sf-link" onclick={() => open(e.reviews.url)}>Open on Steam</button>{/if}
    {/if}
  </section>

  {#if e && e.critic.score > 0}
    <section class="box" aria-labelledby="sf-r-meta">
      <h3 id="sf-r-meta">Metacritic</h3>
      <p class="big">{e.critic.score}</p>
      <p class="sf-muted">Metacritic score via Steam{freshness(e.critic.state, e.critic.fetchedAt) ? ` · ${freshness(e.critic.state, e.critic.fetchedAt)}` : ""}</p>
      {#if e.critic.url}<button type="button" class="sf-link" onclick={() => open(e.critic.url!)}>See it on Metacritic</button>{/if}
    </section>
  {/if}

  <section class="box" aria-labelledby="sf-r-hltb">
    <h3 id="sf-r-hltb">How long to beat</h3>
    {#if !e || e.completion.state === "loading"}
      <p class="sf-muted" aria-busy="true">Loading…</p>
    {:else if times.length && times.some(([, m]) => m > 0)}
      <dl>
        {#each times as [label, m] (label)}<dt>{label}</dt><dd>{hoursText(m) || "Not given"}</dd>{/each}
      </dl>
      {#if e.completion.title}<p class="sf-muted">Matched to {e.completion.title}{e.completion.corrected ? " (your choice)" : ""}</p>{/if}
      {#if freshness(e.completion.state, e.completion.fetchedAt)}<p class="sf-muted">{freshness(e.completion.state, e.completion.fetchedAt)}</p>{/if}
    {:else}
      <p class="sf-muted">Times unavailable{e.completion.error ? `: ${e.completion.error}` : ""}.</p>
    {/if}
    {#if e}
      <div class="row">
        {#if e.completion.url}<button type="button" class="sf-link" onclick={() => open(e.completion.url)}>HowLongToBeat</button>{/if}
        <button type="button" class="sf-link" onclick={onpick}>Wrong game?</button>
      </div>
    {/if}
  </section>
</div>

<style>
  .ratings {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
    gap: 10px;
  }
  .box {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 14px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
    min-width: 0;
  }
  h3 {
    margin: 0;
    font-size: 14px;
    color: var(--muted);
    font-weight: 700;
  }
  p {
    margin: 0;
  }
  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 4px 14px;
    margin: 0;
    font-size: 14px;
  }
  dt {
    color: var(--muted);
  }
  dd {
    margin: 0;
    color: var(--text);
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .big {
    font-family: var(--font-display);
    font-size: 34px;
    font-weight: 700;
    line-height: 1;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 6px 16px;
  }
  .box :global(.sf-link) {
    width: fit-content;
  }
</style>
