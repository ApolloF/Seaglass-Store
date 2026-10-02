<script lang="ts">
  // "Wrong game?": choose which HowLongToBeat entry belongs to this game,
  // or go back to the automatic match.
  import { api } from "../../lib/api";
  import { errText, lib } from "../../lib/store.svelte";
  import { hoursText } from "../../lib/storefront";
  import type { CompletionCandidate, Enrichment } from "../../lib/types";
  import Modal from "./Modal.svelte";

  let { gameKey, title, corrected, ondone, onclose }: { gameKey: string; title: string; corrected: boolean; ondone: (e: Enrichment) => void; onclose: () => void } = $props();

  let list = $state<CompletionCandidate[] | null>(null);
  let error = $state("");
  let busy = $state(false);

  $effect(() => {
    api.store.enrich
      .completionCandidates(gameKey, title)
      .then((l) => (list = l))
      .catch((e) => (error = errText(e)));
  });

  async function choose(id: number) {
    busy = true;
    const e = await lib.run(() => api.store.enrich.setCompletionMatch(gameKey, id));
    busy = false;
    if (e) {
      ondone(e);
      onclose();
    }
  }
</script>

<Modal label="Choose the HowLongToBeat game" {onclose}>
  <p class="sf-muted note">Pick the entry that is {title}. Times come from the one you choose.</p>
  {#if error}
    <p class="err">{error}</p>
  {:else if !list}
    <p class="sf-muted" aria-busy="true">Searching HowLongToBeat…</p>
  {:else if !list.length}
    <p class="sf-muted">HowLongToBeat has no similar games.</p>
  {:else}
    <ul class="list">
      {#each list as c (c.hltbId)}
        <li>
          <div class="text">
            <span class="t">{c.title}{c.year ? ` (${c.year})` : ""}</span>
            <span class="sf-muted">{[c.main && `Main ${hoursText(c.main)}`, c.mainExtras && `Extras ${hoursText(c.mainExtras)}`, c.completionist && `Full ${hoursText(c.completionist)}`].filter(Boolean).join(" · ") || "No times given"}</span>
          </div>
          <button type="button" class="sf-btn" disabled={busy} onclick={() => choose(c.hltbId)}>Use this</button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="actions">
    {#if corrected}<button type="button" class="sf-btn" disabled={busy} onclick={() => choose(0)}>Go back to the automatic match</button>{/if}
    <button type="button" class="sf-btn" onclick={onclose}>Cancel</button>
  </div>
</Modal>

<style>
  .note,
  .err {
    margin: 0;
  }
  .err {
    color: var(--warn);
  }
  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .list li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 10px 10px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .t {
    font-weight: 700;
    overflow-wrap: anywhere;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 10px;
  }
</style>
