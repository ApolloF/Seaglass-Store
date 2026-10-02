<script lang="ts">
  // A small dialog: Escape closes it, focus moves in and returns where it was.
  import type { Snippet } from "svelte";
  import Icon from "../../components/Icon.svelte";

  let { label, onclose, children, wide = false }: { label: string; onclose: () => void; children: Snippet; wide?: boolean } = $props();

  let box: HTMLDivElement | undefined = $state();
  $effect(() => {
    const before = document.activeElement as HTMLElement | null;
    if (box && !box.contains(document.activeElement)) box.focus();
    return () => before?.isConnected && before.focus();
  });

  // Tab stays inside the dialog.
  function keys(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      onclose();
    } else if (e.key === "Tab" && box) {
      const items = [...box.querySelectorAll<HTMLElement>('button:not(:disabled), input, select, [href], [tabindex]:not([tabindex="-1"])')].filter((el) => el.offsetParent !== null);
      if (!items.length) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (e.shiftKey && (document.activeElement === first || document.activeElement === box)) (e.preventDefault(), last.focus());
      else if (!e.shiftKey && document.activeElement === last) (e.preventDefault(), first.focus());
    }
  }
</script>

<div class="scrim" role="presentation" onmousedown={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dialog" class:wide role="dialog" aria-modal="true" aria-label={label} tabindex="-1" bind:this={box} onkeydown={keys}>
    <div class="top">
      <h2>{label}</h2>
      <button type="button" class="close" aria-label="Close" onclick={onclose}><Icon name="close" size={18} stroke={2.2} /></button>
    </div>
    {@render children()}
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
    width: min(560px, 100%);
    max-height: 100%;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 22px 24px;
    border-radius: var(--radius-l);
    background: var(--surface);
    box-shadow: var(--shadow);
    outline: none;
  }
  .dialog.wide {
    width: min(680px, 100%);
  }
  .top {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  h2 {
    flex: 1;
    min-width: 0;
    margin: 0;
    font-family: var(--font-display);
    font-size: 24px;
    overflow-wrap: anywhere;
  }
  .close {
    width: 36px;
    height: 36px;
    border: 0;
    border-radius: 10px;
    background: transparent;
    color: var(--muted);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .close:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  @media (max-width: 520px) {
    .dialog {
      padding: 18px 16px;
    }
  }
</style>
