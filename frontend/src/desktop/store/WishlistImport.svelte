<script lang="ts">
  // Imports a public Steam wishlist. Games already saved stay as they are,
  // and nothing is removed; games without a known release are searched for
  // while Seaglass is idle.
  import Icon from "../../components/Icon.svelte";
  import { api } from "../../lib/api";
  import { importLine } from "../../lib/indexing";
  import { storefront } from "../../lib/storefront.svelte";

  let steamId = $state("");
  let hint = $state("");
  let busy = $state(false);
  let result = $state("");
  let error = $state("");
  let imported = $state(false);

  $effect(() => {
    api.store.wishlist
      .steamAccount()
      .then((a) => {
        if (a.steamId && !steamId) steamId = a.steamId;
        hint = a.detected ? "The Steam account on this PC." : a.error ? `${a.error} Enter a SteamID64 instead.` : "";
      })
      .catch(() => {});
  });

  async function run(e: SubmitEvent) {
    e.preventDefault();
    if (busy) return;
    busy = true;
    error = "";
    try {
      const r = await api.store.wishlist.importSteam(steamId.trim());
      storefront.setWishlist(r.items);
      result = importLine(r);
      imported = true;
    } catch (err) {
      result = "";
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }
</script>

<form class="import" onsubmit={run} aria-busy={busy}>
  <label class="label" for="wi-id">Import from Steam</label>
  <p class="sf-muted">Your Steam profile and its game details must be public.</p>
  <div class="inputs">
    <input id="wi-id" type="text" inputmode="numeric" autocomplete="off" spellcheck="false" placeholder="SteamID64, e.g. 7656119…" aria-describedby={hint ? "wi-hint wi-result" : "wi-result"} bind:value={steamId} />
    <button type="submit" class="sf-btn" disabled={!steamId.trim()} aria-disabled={busy}>
      <Icon name={busy ? "refresh" : "download"} size={15} stroke={2.2} />{busy ? "Importing…" : imported ? "Refresh" : "Import"}
    </button>
  </div>
  {#if hint}<p id="wi-hint" class="sf-muted">{hint}</p>{/if}
  <p id="wi-result" class="out" class:err={!!error} role="status" aria-live="polite">{error || result}</p>
</form>

<style>
  .import {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 12px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .label {
    font-weight: 700;
  }
  .inputs {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 6px;
  }
  input {
    flex: 1 1 220px;
    min-width: 0;
    min-height: 38px;
    padding: 0 12px;
    border: 1px solid var(--line-strong);
    border-radius: 10px;
    background: var(--surface);
    color: var(--text);
    font: inherit;
  }
  input:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }
  p {
    margin: 0;
  }
  .out {
    font-size: 13.5px;
  }
  .out:empty {
    display: none;
  }
  .out.err {
    color: var(--warn);
    overflow-wrap: anywhere;
  }
</style>
