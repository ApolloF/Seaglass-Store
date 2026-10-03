<script lang="ts">
  // The one-time choice of release sources for someone who used the Store
  // before sources were found automatically.
  import Icon from "../../components/Icon.svelte";
  import { api } from "../../lib/api";
  import { providerTraits } from "../../lib/indexing";
  import { lib } from "../../lib/store.svelte";
  import { storefront } from "../../lib/storefront.svelte";

  const providers = $derived(storefront.status?.sources ?? []);
  // Only the providers meant for new Store users start checked.
  let picked = $state<Record<string, boolean>>({});
  const checked = (id: string) => picked[id] ?? !!providers.find((p) => p.id === id)?.defaultOn;
  const chosen = $derived(providers.filter((p) => checked(p.id)).map((p) => p.id));
  const names = $derived(providers.map((p) => p.name));
  const listed = $derived(names.length > 1 ? `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}` : (names[0] ?? "release sources"));
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
  <p>Seaglass can read the public release lists of {listed} to show new games here. It only reads release details such as titles, versions, sizes and languages. It never downloads games by itself.</p>
  <div class="choices" role="group" aria-label="Sources to index">
    {#each providers as p (p.id)}
      {@const traits = providerTraits(p)}
      <label class="choice">
        <input type="checkbox" checked={checked(p.id)} onchange={(e) => (picked[p.id] = e.currentTarget.checked)} />
        <span class="what">
          <span class="name">{p.name}</span>
          {#if traits.length}<span class="traits">{traits.join(" · ")}</span>{/if}
        </span>
      </label>
    {/each}
  </div>
  <div class="actions">
    <button type="button" class="sf-primary" disabled={busy || !chosen.length} onclick={() => start(chosen)}>
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
    max-width: 100%;
    padding: 8px 18px;
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
  .what {
    display: flex;
    flex-direction: column;
    gap: 2px;
    text-align: left;
  }
  .traits {
    color: var(--muted);
    font-size: 13px;
    font-weight: 500;
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
