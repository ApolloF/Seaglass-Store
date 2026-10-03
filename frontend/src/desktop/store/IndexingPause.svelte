<script lang="ts">
  // Pauses or resumes background indexing on this PC. Game pages and
  // searches keep working while it is paused.
  import Icon from "../../components/Icon.svelte";
  import { api } from "../../lib/api";
  import { lib } from "../../lib/store.svelte";
  import { storefront } from "../../lib/storefront.svelte";

  let { variant = "btn" }: { variant?: string } = $props();

  const paused = $derived(!!storefront.status?.paused);
  let busy = $state(false);

  // Not disabled while busy, so the button keeps keyboard focus.
  async function toggle() {
    if (busy) return;
    busy = true;
    const st = await lib.run(() => api.store.discovery.pauseIndexing(!paused));
    busy = false;
    if (!st) return;
    storefront.setStatus(st);
    // The setting is saved by the backend; keep the copy here in step so a
    // later settings change doesn't undo it.
    if (lib.settings) lib.settings = { ...lib.settings, store: { ...lib.settings.store, indexingPaused: st.paused } };
  }
</script>

<button type="button" class={variant} aria-disabled={busy} onclick={toggle}>
  <Icon name={paused ? "play" : "stop"} size={15} stroke={2.2} />{paused ? "Resume indexing" : "Pause indexing"}
</button>

<style>
  /* The Settings look; in the Store it takes the shared sf-btn. */
  .btn {
    width: fit-content;
    height: 36px;
    padding: 0 14px;
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: transparent;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
    font-weight: 700;
  }
  .btn:hover:not(:disabled) {
    background: var(--surface-3);
  }
  .btn[aria-disabled="true"] {
    opacity: 0.6;
  }
</style>
