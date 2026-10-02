<script lang="ts">
  // Before a catalog game downloads: which version, which language, where
  // it goes, and whether to install it straight away. Opens for a feed's
  // CatalogEntry, or for a source release that was prepared (validated
  // torrent metadata) by discovery.
  import { untrack } from "svelte";
  import Icon from "../components/Icon.svelte";
  import Toggle from "../components/Toggle.svelte";
  import { api } from "../lib/api";
  import { offerLine } from "../lib/catalog";
  import { bytes } from "../lib/format";
  import { lib } from "../lib/store.svelte";
  import { languagesLine, publishedText } from "../lib/storefront";
  import type { CatalogEntry, PreparedRelease } from "../lib/types";

  let {
    entry,
    prepared,
    offer = 0,
    update = false,
    onclose,
    ondone,
  }: { entry?: CatalogEntry; prepared?: PreparedRelease; offer?: number; update?: boolean; onclose: () => void; ondone?: () => void } = $props();

  // One thing to choose from, whichever way the dialog was opened.
  interface Choice {
    label: string;
    languages: string[];
    sizeBytes: number;
    installedSizeBytes?: number;
    line: string;
  }
  const gameTitle = $derived(entry?.title ?? prepared?.release.title ?? "");
  const choices = $derived<Choice[]>(
    entry
      ? entry.offers.map((x, i) => ({
          label: `${x.version || "Version not given"} · ${x.feedName}${i === (entry.recommended?.offer ?? 0) ? " (recommended)" : ""}`,
          languages: x.languages ?? [],
          sizeBytes: x.sizeBytes ?? 0,
          installedSizeBytes: x.installedSizeBytes,
          line: offerLine(x),
        }))
      : (prepared?.offers ?? []).map((x) => ({
          label: `${x.version || prepared?.release.version || "Version not given"} · ${x.sourceName}${x.torrentName ? ` · ${x.torrentName}` : ""}`,
          languages: x.languages.length ? x.languages : (prepared?.release.languages ?? []),
          sizeBytes: x.sizeBytes,
          installedSizeBytes: x.installedSizeBytes,
          line: [x.sourceName, x.version, bytes(x.sizeBytes)].filter(Boolean).join(" · "),
        })),
  );

  // Starts on the version picked on the page; changed here after that.
  let pick = $state(untrack(() => offer));
  const o = $derived(choices[pick] ?? choices[0]);
  let language = $state("English");
  let dir = $state("");
  let install = $state(true);
  let busy = $state(false);
  // A prepared release doesn't say where an installed copy is: the catalog does.
  let installedDir = $state("");
  let looked = $state(false);

  $effect(() => {
    if (entry) {
      if (update && entry.installed) dir = entry.installed.dir;
      else api.store.installFolder(entry.title).then((d) => (dir = d));
    } else if (update) {
      api.store
        .catalogEntry(untrack(() => prepared!.gameKey))
        .then((e) => (installedDir = e.installed?.dir ?? ""))
        .catch(() => (installedDir = ""))
        .finally(async () => {
          looked = true;
          dir = installedDir || (await api.store.installFolder(untrack(() => gameTitle)));
        });
    } else {
      api.store.installFolder(gameTitle).then((d) => (dir = d));
    }
  });
  // Without a known folder an update can only be a new install.
  const updating = $derived(update && (entry ? !!entry.installed : !!installedDir));
  const installedVersion = $derived(entry?.installed?.version ?? "the installed version");
  // A language the newly picked version doesn't have goes back to the default.
  $effect(() => {
    if (o?.languages?.length && !o.languages.some((l) => l.toLowerCase() === language.toLowerCase())) language = o.languages.find((l) => l.toLowerCase() === "english") ?? o.languages[0];
  });

  async function choose() {
    const d = await lib.run(() => api.store.chooseInstallFolder(dir));
    if (d) dir = d;
  }
  async function start() {
    busy = true;
    const opts = { dir, language, install, update: updating };
    const d = await lib.run(() => (entry ? api.store.downloadOffer(entry.key, pick, opts) : api.store.discovery.downloadRelease(prepared!.gameKey, prepared!.release.id, prepared!.offers[pick].transport, opts)));
    busy = false;
    if (d) {
      lib.toast(`${d.title} is downloading. Follow it in Downloads.`);
      ondone?.();
      onclose();
    }
  }

  let box: HTMLDivElement | undefined = $state();
  $effect(() => {
    const before = document.activeElement as HTMLElement | null;
    box?.focus();
    return () => before?.isConnected && before.focus();
  });

  const notes = $derived(prepared ? [...new Set([...(prepared.release.unresolved ?? []), ...prepared.warnings])] : []);
</script>

{#if o}
<div class="scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dialog" role="dialog" aria-modal="true" aria-label={`${updating ? "Update" : "Get"} ${gameTitle}`} tabindex="-1" bind:this={box} onkeydown={(e) => e.key === "Escape" && (e.stopPropagation(), onclose())}>
    <div class="top">
      <h2>{updating ? "Update" : "Get"} {gameTitle}</h2>
      <button type="button" class="close" aria-label="Close" onclick={onclose}><Icon name="close" size={18} stroke={2.2} /></button>
    </div>

    {#if choices.length > 1}
      <label class="field">
        <span class="label">Version</span>
        <select bind:value={pick}>
          {#each choices as x, i (i)}<option value={i}>{x.label}</option>{/each}
        </select>
      </label>
    {/if}
    <p class="sub">{o.line}</p>

    {#if prepared}
      <dl class="facts">
        <dt>Source</dt><dd>{prepared.release.sourceName}</dd>
        <dt>Version</dt><dd>{prepared.release.version || "Not stated"}</dd>
        <dt>Languages</dt><dd>{languagesLine(prepared.release)}</dd>
        {#if prepared.release.publishedAt}<dt>Published</dt><dd>{publishedText(prepared.release.publishedAt).replace("Published ", "")}</dd>{/if}
      </dl>
      {#if notes.length}
        <ul class="notes">
          {#each notes as n, i (i)}<li><Icon name="warn" size={14} stroke={2.2} /><span>{n}</span></li>{/each}
        </ul>
      {/if}
    {/if}

    <label class="field">
      <span class="label">Language</span>
      <select bind:value={language} disabled={!o.languages?.length}>
        {#if !o.languages?.length}<option value="English">English (availability checked after metadata arrives)</option>{/if}
        {#each o.languages ?? [] as l (l)}<option value={l}>{l}</option>{/each}
      </select>
    </label>

    <div class="field">
      <span class="label">Folder</span>
      <span class="path" title={dir}>{dir}</span>
      {#if !updating}<button type="button" class="btn" onclick={choose}>Change</button>{/if}
    </div>
    {#if updating}
      <p class="sub">Installs over {installedVersion} once downloaded and checked. Your saves usually stay; back them up first if the game keeps them in its folder.</p>
    {:else}
    {#if update && prepared && looked}
      <p class="sub">Seaglass can't tell where the installed copy is, so this is installed as a separate copy.</p>
    {/if}
    <Toggle
      checked={install}
      title="Install when it's downloaded"
      detail="Installs after checks pass. Off: choose torrent or installer languages in Downloads before installing."
      onchange={(v) => (install = v)}
    />
    {/if}

    <p class="sub">
      {[o.sizeBytes && `Download ${bytes(o.sizeBytes)}`, o.installedSizeBytes && `${bytes(o.installedSizeBytes)} once installed`].filter(Boolean).join(" · ")}
    </p>

    <div class="actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="button" class="btn primary" disabled={busy || !dir} onclick={start}><Icon name="download" size={16} />{busy ? "Starting…" : "Download"}</button>
    </div>
  </div>
</div>
{/if}

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
  .field {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 44px;
    padding: 4px 6px 4px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
    font-size: 14px;
  }
  .label {
    width: 90px;
    flex-shrink: 0;
    font-weight: 600;
  }
  select {
    flex: 1;
    min-width: 0;
    height: 36px;
    padding: 0 10px;
    border-radius: 8px;
    border: 1px solid var(--line-strong);
    background: var(--surface);
    color: var(--text);
    font-size: 14px;
  }
  .path {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-2);
  }
  .sub {
    margin: -4px 0 0;
    font-size: 13.5px;
    color: var(--muted);
  }
  .facts {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 4px 16px;
    margin: 0;
    font-size: 14px;
  }
  .facts dt {
    color: var(--muted);
  }
  .facts dd {
    margin: 0;
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  .notes {
    list-style: none;
    margin: 0;
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    border-radius: var(--radius);
    background: var(--surface-2);
    font-size: 13.5px;
    color: var(--text-2);
  }
  .notes li {
    display: flex;
    gap: 8px;
    align-items: flex-start;
  }
  .notes :global(svg) {
    flex-shrink: 0;
    margin-top: 3px;
    color: var(--warn);
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    margin-top: 4px;
  }
  .btn {
    height: 38px;
    padding: 0 14px;
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: transparent;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14.5px;
    font-weight: 700;
  }
  .btn:hover:not(:disabled) {
    background: var(--surface-3);
  }
  .btn.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--accent-ink);
  }
  .btn.primary:hover:not(:disabled) {
    background: var(--accent);
    filter: brightness(1.08);
  }
  .btn:disabled {
    opacity: 0.6;
  }
  @media (max-width: 520px) {
    .dialog {
      padding: 18px 16px;
    }
    .label {
      width: 70px;
    }
  }
</style>
