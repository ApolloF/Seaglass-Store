<script lang="ts">
  // "Wrong game?": choose which HowLongToBeat entry belongs to a library
  // game, or go back to the automatic match. The choice is shared with the
  // Store's page for the same game.
  import { untrack } from "svelte";
  import Icon from "./Icon.svelte";
  import { api } from "../lib/api";
  import { candidateTimes } from "../lib/completion";
  import { errText, lib } from "../lib/store.svelte";
  import { title, type CompletionCandidate, type Game, type LibraryCompletion } from "../lib/types";

  let { game, corrected, ondone, onclose }: { game: Game; corrected: boolean; ondone: (a: LibraryCompletion) => void; onclose: () => void } = $props();

  let query = $state(untrack(() => title(game)));
  let list = $state<CompletionCandidate[] | null>(null);
  let error = $state("");
  let busy = $state(false);
  let box: HTMLDivElement | undefined = $state();
  let seq = 0;

  async function search() {
    const n = ++seq;
    list = null;
    error = "";
    try {
      const found = await api.completion.candidates(game.id, query.trim());
      if (n === seq) list = found;
    } catch (e) {
      if (n === seq) error = errText(e);
    }
  }

  async function choose(id: number) {
    busy = true;
    const a = await lib.run(() => api.completion.setMatch(game.id, id));
    busy = false;
    if (a) {
      ondone(a);
      onclose();
    }
  }

  // Focus moves in when it opens and back to where it was when it closes.
  $effect(() => {
    const before = document.activeElement as HTMLElement | null;
    box?.focus();
    untrack(search);
    return () => before?.isConnected && before.focus();
  });

  // Tab stays inside the dialog.
  function keys(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      onclose();
    } else if (e.key === "Tab" && box) {
      const items = [...box.querySelectorAll<HTMLElement>("button:not(:disabled), input")].filter((el) => el.offsetParent !== null);
      if (!items.length) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (e.shiftKey && (document.activeElement === first || document.activeElement === box)) (e.preventDefault(), last.focus());
      else if (!e.shiftKey && document.activeElement === last) (e.preventDefault(), first.focus());
    }
  }
</script>

<div class="scrim" role="presentation" onmousedown={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Choose the HowLongToBeat game" tabindex="-1" bind:this={box} onkeydown={keys}>
    <div class="head">
      <h2>Which game is this?</h2>
      <button type="button" class="close" aria-label="Close" onclick={onclose}><Icon name="close" size={18} stroke={2.2} /></button>
    </div>
    <p class="hint">Pick the HowLongToBeat entry for {title(game)}. The times come from the one you choose.</p>
    <form
      class="search"
      onsubmit={(e) => {
        e.preventDefault();
        search();
      }}
    >
      <Icon name="search" size={18} stroke={2} />
      <input bind:value={query} aria-label="Game title" placeholder="Game title" />
      <button type="submit" class="go" disabled={list === null && !error}>Search</button>
    </form>
    <div class="results" aria-live="polite">
      {#if error}
        <p class="msg error">{error}</p>
      {:else if !list}
        <p class="msg" aria-busy="true">Searching HowLongToBeat…</p>
      {:else if !list.length}
        <p class="msg">HowLongToBeat has no similar games. Try a shorter title.</p>
      {:else}
        {#each list as c (c.hltbId)}
          <div class="hit">
            <div class="text">
              <span class="name">{c.title}{c.year ? ` (${c.year})` : ""}{#if c.type && c.type !== "game"}<span class="kind">{c.type}</span>{/if}</span>
              <span class="times">{candidateTimes(c)}</span>
            </div>
            <button type="button" class="use" disabled={busy} onclick={() => choose(c.hltbId)}>Use this</button>
          </div>
        {/each}
      {/if}
    </div>
    {#if corrected}
      <button type="button" class="auto" disabled={busy} onclick={() => choose(0)}>Use the automatic match</button>
    {/if}
  </div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 60;
    background: var(--scrim);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 16px;
  }
  .dialog {
    width: min(600px, 100%);
    max-height: min(640px, 100%);
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 22px 24px 24px;
    border-radius: var(--radius-l);
    background: var(--surface);
    border: 1px solid var(--line-strong);
    box-shadow: var(--shadow);
    outline: none;
  }
  .head {
    display: flex;
    align-items: center;
  }
  h2 {
    flex: 1;
    margin: 0;
    font-family: var(--font-display);
    font-size: 26px;
  }
  .close {
    width: 36px;
    height: 36px;
    border: 0;
    border-radius: 10px;
    background: var(--surface-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .hint {
    margin: 0;
    color: var(--muted);
    font-size: 14px;
  }
  .search {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 44px;
    padding: 0 6px 0 12px;
    border-radius: 10px;
    background: var(--surface-2);
    border: 1px solid var(--line);
    color: var(--muted);
  }
  .search:focus-within {
    border-color: var(--accent);
  }
  .search input {
    flex: 1;
    min-width: 0;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--text);
    font-size: 15px;
  }
  .go {
    height: 32px;
    padding: 0 12px;
    border: 0;
    border-radius: 8px;
    background: var(--accent);
    color: var(--accent-ink);
    font-weight: 700;
    font-size: 14px;
  }
  .results {
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-height: 120px;
  }
  .hit {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px 10px 10px 12px;
    border-radius: 10px;
    background: var(--surface-2);
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }
  .name {
    font-weight: 700;
    overflow-wrap: anywhere;
  }
  .kind {
    margin-left: 8px;
    padding: 1px 8px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 12px;
    text-transform: uppercase;
  }
  .times {
    font-size: 13px;
    color: var(--muted);
  }
  .use,
  .auto {
    height: 34px;
    padding: 0 12px;
    border-radius: var(--radius-s);
    border: 1px solid var(--line-strong);
    background: transparent;
    font-size: 13.5px;
    font-weight: 700;
  }
  .use:hover:not(:disabled),
  .auto:hover:not(:disabled) {
    background: var(--surface-3);
  }
  .auto {
    align-self: flex-start;
  }
  .msg {
    margin: 12px 0;
    text-align: center;
    color: var(--muted);
  }
  .msg.error {
    color: var(--danger);
  }
</style>
