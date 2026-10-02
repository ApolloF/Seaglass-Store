<script lang="ts">
  // "Change Steam match": say which Steam game this is, or that it isn't on Steam.
  import { untrack } from "svelte";
  import { api } from "../../lib/api";
  import { errText, lib } from "../../lib/store.svelte";
  import type { GameDetails, StoreHit } from "../../lib/types";
  import Modal from "./Modal.svelte";

  let { gameKey, title, ondone, onclose }: { gameKey: string; title: string; ondone: (d: GameDetails) => void; onclose: () => void } = $props();

  let query = $state(untrack(() => title));
  let hits = $state<StoreHit[]>([]);
  let busy = $state(false);
  let searching = $state(false);
  let error = $state("");
  let input: HTMLInputElement | undefined = $state();
  let seq = 0;
  let searched = "";

  async function search() {
    const q = query.trim();
    if (!q) return;
    searched = q;
    const mine = ++seq;
    searching = true;
    error = "";
    try {
      const r = await api.searchSteam(q);
      if (mine === seq) hits = r;
    } catch (e) {
      if (mine === seq) error = errText(e);
    } finally {
      if (mine === seq) searching = false;
    }
  }

  async function pick(appId: number, name: string) {
    busy = true;
    const d = await lib.run(() => api.store.discovery.setSteamMatch(gameKey, appId, name));
    busy = false;
    if (d) {
      lib.toast(appId ? `${title} is now matched to ${name}.` : `${title} is marked as not on Steam.`);
      ondone(d);
      onclose();
    }
  }

  // Once, when it opens: search() reads the query and would run again on every key.
  $effect(() => {
    if (!input) return;
    input.focus();
    input.select();
    untrack(search);
  });
  // Results follow the text as it is typed.
  $effect(() => {
    const q = query;
    const t = setTimeout(() => q.trim() && q.trim() !== searched && search(), 400);
    return () => clearTimeout(t);
  });
</script>

<Modal label="Change Steam match" {onclose}>
  <form class="search" onsubmit={(e) => (e.preventDefault(), search())}>
    <label class="sr-only" for="sf-steam-q">Search Steam</label>
    <input id="sf-steam-q" class="sf-field" type="search" autocomplete="off" bind:value={query} bind:this={input} />
    <button type="submit" class="sf-btn" disabled={searching || !query.trim()}>Search</button>
  </form>
  {#if error}<p class="err">{error}</p>
  {:else if searching && !hits.length}<p class="sf-muted" aria-busy="true">Searching Steam…</p>
  {:else if !hits.length}<p class="sf-muted">Nothing found on Steam.</p>
  {:else}
    <ul class="list">
      {#each hits as h (h.appId)}
        <li>
          <div class="text"><span class="t">{h.name}</span><span class="sf-muted">AppID {h.appId}</span></div>
          <button type="button" class="sf-btn" disabled={busy} onclick={() => pick(h.appId, h.name)}>Use this</button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="actions">
    <button type="button" class="sf-btn" disabled={busy} onclick={() => pick(0, "")}>Not on Steam</button>
    <button type="button" class="sf-btn" onclick={onclose}>Cancel</button>
  </div>
</Modal>

<style>
  .search {
    display: flex;
    gap: 8px;
  }
  .search .sf-field {
    flex: 1;
    min-width: 0;
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
