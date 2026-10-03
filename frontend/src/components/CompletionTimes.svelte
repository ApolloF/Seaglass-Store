<script lang="ts">
  // A game's HowLongToBeat times: cached ones at once, fetched when the game
  // stays shown. An answer for a game that is no longer shown is dropped. In big picture
  // (`big`) it is plain text: no links or buttons a controller can't reach.
  import { api } from "../lib/api";
  import { completionLink, completionView, isFor, watchCompletion } from "../lib/completion";
  import { lib } from "../lib/store.svelte";
  import { title, type Game, type LibraryCompletion } from "../lib/types";
  import CompletionPicker from "./CompletionPicker.svelte";

  let { game, big = false }: { game: Game; big?: boolean } = $props();

  const gid = $derived(game.id);
  let answer = $state<LibraryCompletion | null>(null);
  let failed = $state(false);
  let picking = $state(false);

  $effect(() => {
    answer = null;
    failed = false;
    picking = false;
    return watchCompletion(gid, api.completion.get, { answer: (a) => (answer = a), failed: () => (failed = true) });
  });

  const view = $derived(completionView(failed ? { hltbId: 0, main: 0, mainExtras: 0, completionist: 0, url: "", corrected: false, fetchedAt: 0, state: "unavailable" } : (answer?.completion ?? null)));
  const link = $derived(completionLink(answer?.completion ?? null, title(game)));
  const open = () => lib.run(() => api.completion.openLink(link));
</script>

<section class="times" class:big aria-label="How long to beat" aria-busy={view.mode === "loading"}>
  <div class="head">
    <h3>How long to beat</h3>
    {#if !big}
      {#if view.mode !== "loading"}<button type="button" class="link" onclick={() => (picking = true)}>Wrong game?</button>{/if}
    {/if}
  </div>
  {#if view.mode === "loading"}
    <p class="note">Looking up the times…</p>
  {:else if view.mode === "times"}
    <dl>
      {#each view.rows as r (r.label)}
        <div><dt>{r.label}</dt><dd>{r.text}</dd></div>
      {/each}
    </dl>
    {#if view.stale}<p class="note">These may be out of date. HowLongToBeat couldn't be reached.</p>{/if}
    {#if view.matched}<p class="note">Matched to {view.matched} (your choice).</p>{/if}
  {:else}
    <p class="note">Times unavailable.</p>
  {/if}
  {#if view.mode !== "loading"}
    <p class="note source">
      {#if big}
        Times from HowLongToBeat
      {:else}
        Times from <button type="button" class="link" onclick={open}>HowLongToBeat</button>{#if view.mode === "none"}.
          <button type="button" class="link" onclick={open}>Search there</button>{/if}
      {/if}
    </p>
  {/if}
</section>

{#if picking}
  <CompletionPicker
    {game}
    corrected={!!answer?.completion.corrected}
    ondone={(a) => isFor(game.id, a) && (answer = a)}
    onclose={() => (picking = false)}
  />
{/if}

<style>
  .times {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  h3 {
    flex: 1;
    margin: 0;
    font-size: 15px;
    font-weight: 700;
  }
  dl {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 10px;
    margin: 0;
  }
  dl div {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  dt {
    font-size: 12.5px;
    font-weight: 600;
    color: var(--muted);
  }
  dd {
    margin: 0;
    font-size: 16px;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
  .note {
    margin: 0;
    font-size: 13px;
    line-height: 1.4;
    color: var(--muted);
  }
  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--accent-text);
    font: inherit;
    font-weight: 600;
  }
  .link:hover {
    text-decoration: underline;
  }
  @media (max-width: 420px) {
    dl {
      grid-template-columns: 1fr;
    }
    dl div {
      flex-direction: row;
      justify-content: space-between;
    }
  }

  /* Big picture: on the dark sheet, read from the couch. */
  .times.big {
    gap: 10px;
    padding: 0;
    background: none;
    color: #f3f5f7;
  }
  .big h3 {
    font-size: 18px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: rgba(243, 245, 247, 0.7);
  }
  .big dl {
    display: flex;
    flex-wrap: wrap;
    gap: 14px 48px;
  }
  .big dt {
    font-size: 18px;
    color: rgba(243, 245, 247, 0.65);
  }
  .big dd {
    font-size: 32px;
  }
  .big .note {
    font-size: 18px;
    color: rgba(243, 245, 247, 0.6);
  }
</style>
