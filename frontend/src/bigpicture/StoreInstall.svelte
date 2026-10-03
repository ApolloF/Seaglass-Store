<script lang="ts">
  // The install confirmation from the couch, the same flow as the desktop's:
  // the release is prepared first (Go fetches its page and checks the torrent
  // metadata; nothing downloads), then version, language, folder and
  // whether to install are confirmed. Only a validated release offers a
  // download; one Seaglass couldn't check offers its page instead.
  import { onMount, untrack } from "svelte";
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { blockedText, cycle, installChoices, installStep, pickLanguage, sizesText } from "../lib/bpstore";
  import { feedback, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { languagesLine, publishedText } from "../lib/storefront";
  import type { CatalogEntry, Download, PreparedRelease, Release } from "../lib/types";
  import Hints from "./Hints.svelte";

  let {
    gameKey,
    release,
    update,
    onclose,
    onqueued,
  }: { gameKey: string; release: Release; update: boolean; onclose: () => void; onqueued: (d: Download) => void } = $props();

  let prepared = $state<PreparedRelease | null>(null);
  let entry = $state<CatalogEntry | null>(null);
  let closed = false;

  onMount(() => {
    void (async () => {
      if (release.origin === "feed" && release.feedKey) {
        // A feed's offer is already validated.
        const e = await lib.run(() => api.store.catalogEntry(release.feedKey!));
        if (closed) return;
        if (e) entry = e;
        else onclose();
        return;
      }
      const p = await lib.run(() => api.store.discovery.prepareRelease(gameKey, release.id));
      if (closed) return;
      if (p) prepared = p;
      else onclose();
    })();
    return () => (closed = true);
  });

  const step = $derived(installStep(prepared, entry));
  const choices = $derived(installChoices(prepared, entry));
  let pick = $state(untrack(() => (release.origin === "feed" ? Math.max(0, release.feedOffer) : 0)));
  const o = $derived(choices[pick] ?? choices[0]);
  let language = $state("English");
  let dir = $state("");
  let install = $state(true);
  let busy = $state(false);

  const gameTitle = $derived(entry?.title ?? prepared?.release.title ?? release.title);
  const installedDir = $derived(entry ? (entry.installed?.dir ?? "") : (prepared?.installed?.dir ?? ""));
  // Without a known folder an update can only be a new install.
  const updating = $derived(update && !!installedDir);
  const installedVersion = $derived(entry?.installed?.version ?? prepared?.installed?.version ?? "the installed version");
  $effect(() => {
    if (step !== "confirm") return;
    if (updating) dir = installedDir;
    else void api.store.installFolder(gameTitle).then((d) => (dir = d));
  });
  // A language the picked version doesn't have goes back to the default.
  $effect(() => {
    const langs = o?.languages ?? [];
    language = pickLanguage(langs, untrack(() => language));
  });
  const notes = $derived(prepared ? [...new Set([...(prepared.release.unresolved ?? []), ...prepared.warnings])] : []);

  async function choose() {
    const d = await lib.run(() => api.store.chooseInstallFolder(dir));
    if (d) dir = d;
  }
  async function start() {
    if (busy || !dir) return feedback.edge();
    busy = true;
    const opts = { dir, language, install, update: updating };
    const d = await lib.run(() =>
      entry ? api.store.downloadOffer(entry.key, pick, opts) : api.store.discovery.downloadRelease(prepared!.gameKey, prepared!.release.id, prepared!.offers[pick].transport, opts),
    );
    busy = false;
    if (closed || !d) return;
    lib.toast(`${d.title} is in Downloads.`);
    onqueued(d);
  }
  const openPage = () => void lib.run(() => api.store.discovery.openRelease(gameKey, release.id));

  // The rows to move through: settings first, then the buttons.
  type Row = "version" | "language" | "folder" | "install" | "download" | "page" | "close";
  const rows = $derived.by<Row[]>(() => {
    if (step === "confirm")
      return [
        ...(choices.length > 1 ? (["version"] as const) : []),
        ...((o?.languages.length ?? 0) > 1 ? (["language"] as const) : []),
        ...(updating ? [] : (["folder", "install"] as const)),
        "download",
        "close",
      ];
    if (step === "browser") return [...(release.pageUrl ? (["page"] as const) : []), "close"];
    return ["close"];
  });
  let i = $state(0);
  $effect(() => {
    i = Math.min(i, rows.length - 1);
  });
  // The confirmation opens on Download, the usual choice.
  $effect(() => {
    if (step === "confirm") i = untrack(() => rows.indexOf("download"));
  });

  function change(row: Row, d: -1 | 1) {
    if (row === "version") pick = cycle(choices.length, pick, d);
    else if (row === "language") {
      const langs = o.languages;
      language = langs[cycle(langs.length, Math.max(0, langs.indexOf(language)), d)];
    } else return feedback.edge();
    feedback.move();
  }
  function press(row: Row | undefined) {
    if (!row) return;
    if (row === "version" || row === "language") return change(row, 1);
    feedback.confirm();
    if (row === "folder") void choose();
    else if (row === "install") install = !install;
    else if (row === "download") void start();
    else if (row === "page") openPage();
    else onclose();
  }

  $effect(() =>
    useInput((intent) => {
      switch (intent) {
        case "up":
        case "down": {
          const j = i + (intent === "up" ? -1 : 1);
          if (j >= 0 && j < rows.length) ((i = j), feedback.move());
          else feedback.edge();
          return;
        }
        case "left":
        case "right":
          change(rows[i], intent === "left" ? -1 : 1);
          return;
        case "confirm":
          press(rows[i]);
          return;
        case "back":
          onclose();
          return;
        case "menu":
        case "home":
          return false;
      }
    }),
  );

  const label: Record<Row, string> = {
    version: "Version",
    language: "Language",
    folder: "Folder",
    install: "Install when it's downloaded",
    download: "Download",
    page: "Open in browser",
    close: "Cancel",
  };
  // Nothing is cancelled once Seaglass has said why it can't go on.
  const rowLabel = (row: Row) => (row === "close" && (step === "browser" || step === "blocked") ? "Close" : label[row]);
  const confirmLabel = (row: Row | undefined) => (row === "version" || row === "language" || row === "folder" ? "Change" : row === "install" ? "Turn on or off" : row ? rowLabel(row) : "");
</script>

<div class="scrim">
  <div class="panel" role="dialog" aria-modal="true" aria-label={`${updating ? "Update" : "Get"} ${gameTitle}`}>
    <h2>{updating ? "Update" : "Get"} {gameTitle}</h2>

    {#if step === "checking"}
      <p class="sub" aria-live="polite">Seaglass is reading the release page. This can take a few seconds.</p>
    {:else if step === "browser"}
      <p class="why">{prepared?.reason || "Seaglass couldn't get this release's torrent file yet."}</p>
      {#each prepared?.warnings ?? [] as w, k (k)}<p class="sub">{w}</p>{/each}
      <p class="sub">Get the .torrent file from the release page and attach it in desktop mode. Seaglass checks it before anything downloads.</p>
    {:else if step === "blocked"}
      <p class="why">{blockedText(prepared)}</p>
    {:else if o}
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
            {#each notes.slice(0, 3) as n, k (k)}<li><Icon name="warn" size={20} stroke={2.2} /><span>{n}</span></li>{/each}
          </ul>
        {/if}
      {/if}
      {#if !o.languages.length}
        <p class="sub">Language: English. Seaglass checks it once the torrent's details arrive.</p>
      {:else if o.languages.length === 1}
        <p class="sub">Language: {language}</p>
      {/if}
      {#if updating}
        <p class="sub">Installs over {installedVersion} once downloaded and checked. Your saves usually stay; back them up first if the game keeps them in its folder.</p>
      {:else if update}
        <p class="sub">Seaglass can't tell where the installed copy is, so this is installed as a separate copy.</p>
      {/if}
      {#if sizesText(o)}<p class="sub">{sizesText(o)}</p>{/if}
    {/if}

    <div class="rows">
      {#each rows as row, k (row)}
        <button
          type="button"
          class="row"
          class:on={k === i}
          class:primary={row === "download" || row === "page"}
          tabindex="-1"
          disabled={row === "download" && (busy || !dir)}
          onclick={() => ((i = k), press(row))}
        >
          <span class="text">
            <span class="t">{row === "download" && busy ? "Starting…" : rowLabel(row)}</span>
            {#if row === "install"}<span class="d">Installs after checks pass. Off: finish it in desktop mode's Downloads.</span>{/if}
          </span>
          {#if row === "version"}
            <span class="choice"><span class="arrow">‹</span><span class="val">{o.label}</span><span class="arrow">›</span></span>
          {:else if row === "language"}
            <span class="choice"><span class="arrow">‹</span><span class="val">{language}</span><span class="arrow">›</span></span>
          {:else if row === "folder"}
            <span class="path" title={dir}>{dir}</span><span class="act">Change</span>
          {:else if row === "install"}
            <span class="track" class:yes={install}><span class="knob"></span></span>
          {:else if row === "download"}
            <Icon name="download" size={24} />
          {:else if row === "page"}
            <Icon name="link" size={24} />
          {/if}
        </button>
      {/each}
    </div>
  </div>
  <div class="hints">
    <Hints
      hints={[
        ...(confirmLabel(rows[i]) ? [{ button: "confirm" as const, label: confirmLabel(rows[i]) }] : []),
        { button: "back", label: "Back" },
      ]}
    />
  </div>
</div>

<style>
  .scrim {
    position: absolute;
    inset: 0;
    z-index: 30;
    background: rgba(6, 8, 11, 0.82);
    display: flex;
    align-items: center;
    justify-content: center;
    animation: in 0.2s ease both;
  }
  @keyframes in {
    from {
      opacity: 0;
    }
  }
  .panel {
    width: 1100px;
    max-height: 860px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    gap: 14px;
    padding: 44px 50px;
    border-radius: 28px;
    background: #0f151c;
    border: 1px solid rgba(255, 255, 255, 0.08);
    box-shadow: 0 30px 90px rgba(0, 0, 0, 0.6);
    color: #e8edf2;
  }
  h2 {
    margin: 0 0 6px;
    font-family: var(--font-display);
    font-size: 48px;
    line-height: 1.05;
    overflow-wrap: anywhere;
  }
  .why {
    margin: 0;
    font-size: 22px;
    line-height: 1.45;
    overflow-wrap: anywhere;
  }
  .sub {
    margin: 0;
    font-size: 19px;
    line-height: 1.45;
    color: #9ba8b5;
    overflow-wrap: anywhere;
  }
  .facts {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 6px 22px;
    margin: 0;
    font-size: 19px;
  }
  .facts dt {
    color: #9ba8b5;
  }
  .facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .notes {
    list-style: none;
    margin: 0;
    padding: 14px 18px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    border-radius: 14px;
    background: #121a23;
    font-size: 18px;
    color: #c9d2db;
  }
  .notes li {
    display: flex;
    gap: 10px;
    align-items: flex-start;
  }
  .notes :global(svg) {
    flex-shrink: 0;
    margin-top: 2px;
    color: #f3b35a;
  }
  .rows {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-top: 8px;
    padding: 4px;
  }
  .row {
    display: flex;
    align-items: center;
    gap: 24px;
    min-height: 72px;
    padding: 14px 26px;
    border: 0;
    border-radius: 18px;
    background: #121a23;
    color: #e8edf2;
    text-align: left;
  }
  .row.on {
    background: #1b2632;
    box-shadow: 0 0 0 3px #fff;
  }
  .row.primary .t {
    color: oklch(0.88 0.09 205);
  }
  .row:disabled {
    opacity: 0.6;
  }
  .text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .d {
    font-size: 18px;
    color: #9ba8b5;
  }
  .t {
    font-size: 23px;
    font-weight: 700;
    white-space: nowrap;
  }
  .choice {
    display: flex;
    align-items: center;
    gap: 14px;
    min-width: 0;
    max-width: 700px;
    font-size: 21px;
    font-weight: 700;
  }
  .val {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .arrow {
    color: #9ba8b5;
    font-size: 30px;
  }
  .path {
    min-width: 0;
    max-width: 640px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 19px;
    color: #c9d2db;
  }
  .act {
    font-size: 20px;
    font-weight: 700;
    color: oklch(0.88 0.09 205);
    white-space: nowrap;
  }
  .track {
    position: relative;
    width: 62px;
    height: 34px;
    border-radius: 17px;
    background: rgba(255, 255, 255, 0.18);
    flex-shrink: 0;
    transition: background 0.2s;
  }
  .track.yes {
    background: oklch(0.8 0.12 205);
  }
  .knob {
    position: absolute;
    top: 4px;
    left: 4px;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    background: #fff;
    transition: transform 0.2s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .track.yes .knob {
    transform: translateX(28px);
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 48px;
    left: 110px;
  }
</style>
