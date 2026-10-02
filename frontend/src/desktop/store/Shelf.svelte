<script lang="ts">
  // A titled row of cards that scrolls sideways inside itself.
  import type { Snippet } from "svelte";

  let { title, note = "", empty = "", count, children }: { title: string; note?: string; empty?: string; count: number; children: Snippet } = $props();
  const id = `sf-shelf-${Math.random().toString(36).slice(2, 8)}`;
</script>

<section class="shelf" aria-labelledby={id}>
  <div class="head">
    <h2 {id}>{title}</h2>
    {#if note}<span class="sf-chip">{note}</span>{/if}
  </div>
  {#if count > 0}
    <ul class="row">{@render children()}</ul>
  {:else}
    <p class="empty sf-muted">{empty}</p>
  {/if}
</section>

<style>
  .shelf {
    display: flex;
    flex-direction: column;
    gap: 10px;
    min-width: 0;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 24px;
  }
  .row {
    list-style: none;
    margin: 0;
    padding: 4px 4px 12px;
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: 160px;
    gap: 16px;
    overflow-x: auto;
    scroll-padding: 0 4px;
  }
  .empty {
    margin: 0;
    padding: 14px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  @media (max-width: 700px) {
    .row {
      grid-auto-columns: 132px;
      gap: 12px;
    }
  }
</style>
