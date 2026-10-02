<script lang="ts">
  import { untrack } from "svelte";
  import { api } from "../lib/api";
  import { lib } from "../lib/store.svelte";
  import { shop } from "../lib/shop.svelte";
  import Toggle from "../components/Toggle.svelte";
  import type { Download, DownloadLanguageOptions } from "../lib/types";

  let { download, onclose }: { download: Download; onclose: () => void } = $props();
  let options = $state<DownloadLanguageOptions | null>(null);
  let language = $state(untrack(() => download.language || "English"));
  let setupLanguage = $state(untrack(() => download.setupLanguage || ""));
  let ask = $state(untrack(() => download.askInstaller ?? false));
  let busy = $state(false);
  let error = $state("");
  let box: HTMLDivElement | undefined = $state();
  $effect(() => { box?.focus(); });
  $effect(() => {
    let active = true;
    api.store.downloadLanguages(download.id).then((o) => {
      if (!active) return;
      options = o;
      if (o.game.length && !o.game.includes(language) && language !== "*") language = o.game.find((l) => l.toLowerCase() === "english") ?? "*";
      if (!setupLanguage) setupLanguage = o.installer.find((l) => l.id === "english")?.id ?? "";
    }).catch((e) => { if (active) error = String(e); });
    return () => { active = false; };
  });
  async function save() {
    busy = true;
    const d = await lib.run(() => api.store.setDownloadLanguages(download.id, language, setupLanguage, ask));
    busy = false;
    if (!d) return;
    shop.downloads = shop.downloads.map((old) => old.id === d.id ? d : old);
    lib.toast(d.state === "paused" && !d.safety ? "Resume the download to fetch the selected language packs, then install." : "Languages saved. Install when you are ready.");
    onclose();
  }
</script>

<div class="scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dialog" role="dialog" aria-modal="true" aria-label={`Languages for ${download.title}`} tabindex="-1" bind:this={box} onkeydown={(e) => e.key === "Escape" && onclose()}>
    <h2>Languages for {download.title}</h2>
    {#if error}<p role="alert">{error}</p>
    {:else if !options}<p>Reading available languages…</p>
    {:else}
      <label>Game language
        <select bind:value={language}>
          {#if !options.game.length}<option value="English">English (availability not listed)</option>{/if}
          {#each options.game as l (l)}<option value={l}>{l}</option>{/each}
          {#if options.torrent}<option value="*">All language packs</option>{/if}
        </select>
      </label>
      <p>{options.torrent ? "Selects optional language packs in the torrent. Required and unrecognized files stay selected." : "Game language claims come from the feed. Use the installer to choose game components when needed."}</p>
      {#if options.installer.length}
        <label>Installer language
          <select bind:value={setupLanguage}>
            <option value="">Installer default</option>
            {#each options.installer as l (l)}<option value={l.id}>{l.name}</option>{/each}
          </select>
        </label>
      {/if}
      {#if options.note}<p>{options.note}</p>{/if}
      <Toggle checked={ask} title="Ask in the installer" detail="Shows the installer's language and component choices. Automatic installation is turned off when you save these options." onchange={(v) => ask = v} />
    {/if}
    <div class="actions">
      <button type="button" onclick={onclose}>Cancel</button>
      <button type="button" class="primary" disabled={!options || busy} onclick={save}>{busy ? "Saving…" : "Save languages"}</button>
    </div>
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; z-index: 65; background: var(--scrim); display: flex; align-items: center; justify-content: center; padding: 16px; }
  .dialog { width: min(540px, 100%); max-height: 100%; overflow-y: auto; padding: 22px; border-radius: var(--radius-l); background: var(--surface); box-shadow: var(--shadow); display: flex; flex-direction: column; gap: 12px; }
  h2 { margin: 0; font-family: var(--font-display); font-size: 24px; }
  p { margin: 0; color: var(--muted); font-size: 14px; }
  label { display: flex; flex-direction: column; gap: 6px; font-weight: 600; }
  select, button { min-height: 38px; border-radius: var(--radius); border: 1px solid var(--line-strong); background: var(--surface-2); color: var(--text); padding: 8px 12px; }
  .actions { display: flex; justify-content: flex-end; gap: 10px; }
  .primary { background: var(--accent); color: var(--accent-ink); }
  button:disabled { opacity: .5; }
</style>
