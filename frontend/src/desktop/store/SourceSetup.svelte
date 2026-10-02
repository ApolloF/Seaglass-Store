<script lang="ts">
  // The one-time choice of release sources for someone who used the Store
  // before sources were found automatically.
  import Icon from "../../components/Icon.svelte";
  import { api } from "../../lib/api";
  import { lib } from "../../lib/store.svelte";
  import { storefront } from "../../lib/storefront.svelte";

  let fitgirl = $state(true);
  let dodi = $state(true);
  let busy = $state(false);

  async function start(chosen: string[]) {
    busy = true;
    const s = await lib.run(() => api.store.discovery.setupSources(chosen));
    busy = false;
    if (s) {
      lib.settings = s;
      void storefront.load();
    }
  }
</script>

<div class="sf-empty setup">
  <Icon name="cloudDown" size={40} stroke={1.6} />
  <h2>Find games automatically</h2>
  <p>Seaglass can read the public release lists of FitGirl and DODI to show new games here. It only reads release details such as titles, versions, sizes and languages. It never downloads games by itself.</p>
  <div class="choices" role="group" aria-label="Sources to index">
    <label class="choice"><input type="checkbox" bind:checked={fitgirl} /><span>FitGirl</span></label>
    <label class="choice"><input type="checkbox" bind:checked={dodi} /><span>DODI</span></label>
  </div>
  <div class="actions">
    <button type="button" class="sf-primary" disabled={busy || (!fitgirl && !dodi)} onclick={() => start([fitgirl && "fitgirl", dodi && "dodi"].filter((s): s is string => !!s))}>
      {busy ? "Starting…" : "Start"}
    </button>
    <button type="button" class="sf-btn" disabled={busy} onclick={() => start([])}>Don't use sources</button>
  </div>
  <p class="sf-muted">You can change this later in Settings → Experimental.</p>
</div>

<style>
  .setup {
    flex: 1;
  }
  .choices {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 10px;
    margin: 8px 0 4px;
  }
  .choice {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 44px;
    padding: 0 18px;
    border-radius: var(--radius);
    background: var(--surface-2);
    color: var(--text);
    font-weight: 700;
    cursor: pointer;
  }
  .choice input {
    width: 18px;
    height: 18px;
    accent-color: var(--accent);
  }
  .choice:focus-within {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }
  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 10px;
    margin-top: 6px;
  }
</style>
