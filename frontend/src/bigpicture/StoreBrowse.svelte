<script lang="ts">
  // Browse and search the Store from the couch: the on-screen keyboard on
  // the left, two filters and the results on the right. Matches from the
  // index show at once, the sources and Steam fill in after typing stops,
  // and an answer to an older query is dropped. A real keyboard types too.
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { browseQuery, browseResults, cycle, SHOW_CHOICES, sourceChoices } from "../lib/bpstore";
  import { isWide, keyLabel, keyMove, KEYS, typable, typeKey, wideColumn } from "../lib/bpstore-keyboard";
  import { feedback, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { createSearchController, providerText, remoteShown, type SearchView } from "../lib/storefront";
  import { storefront } from "../lib/storefront.svelte";
  import { shop } from "../lib/shop.svelte";
  import type { GameSummary } from "../lib/types";
  import Hints from "./Hints.svelte";
  import { gridStep } from "./nav";
  import StoreCard from "./StoreCard.svelte";

  let { width, height, active, onopen, onback }: { width: number; height: number; active: boolean; onopen: (g: GameSummary) => void; onback: () => void } = $props();

  let text = $state("");
  let zone = $state<"keys" | "filters" | "results">("keys");
  let k = $state(11);
  let r = $state(0);
  let rKey = "";
  let chip = $state(0);
  let si = $state(0);
  let shi = $state(0);
  // Typing on a real keyboard: Enter then goes to the results instead of
  // pressing the on-screen key under the highlight.
  let typing = $state(false);
  let view = $state<SearchView | null>(null);

  const sources = $derived(sourceChoices(storefront.status));
  const sourceId = $derived(sources[si]?.id ?? "");
  const show = $derived(SHOW_CHOICES[shi]);

  const search = createSearchController({
    browse: (q) => api.store.discovery.browse(q),
    search: (q) => api.store.discovery.search(q),
    onChange: (v) => (view = v),
    onError: (m) => lib.toast(m, "error"),
  });
  $effect(() => () => search.dispose());
  $effect(() => api.store.discovery.onSearch((p) => search.progress(p)));
  $effect(() => api.store.discovery.onGames(() => search.refreshLocal()));
  $effect(() => search.update(browseQuery(text, sourceId, show.id)));

  const results = $derived(view ? browseResults(view) : []);
  // The results change under the focus (remote answers, a wishlist change): it stays on its game.
  $effect(() => {
    const i = results.findIndex((x) => x.game.key === rKey);
    if (i >= 0) r = i;
    else if (r >= results.length) setR(Math.max(0, results.length - 1));
  });
  function setR(i: number) {
    r = i;
    rKey = results[i]?.game.key ?? "";
  }
  $effect(() => {
    const keys = results.map((x) => x.game.key);
    shop.requestArt(keys);
    storefront.ask(keys);
  });

  const remote = $derived(view ? remoteShown(view.query.text, view.remote) : []);
  const progressLine = $derived(
    [view?.remoteError ? `Search failed: ${view.remoteError}` : "", ...remote.map((p) => `${p.name} ${providerText(p)}`)].filter(Boolean).join(" · "),
  );
  const countLine = $derived.by(() => {
    if (!view?.page) return "";
    const other = results.filter((x) => x.other).length;
    return [`${view.page.total} ${view.page.total === 1 ? "game" : "games"}`, other ? `${other} more on Steam without a source release` : ""].filter(Boolean).join(" · ");
  });

  // Results: landscape cards in a grid that scrolls by rows.
  const LEFT = 830;
  const SIDE = 110;
  const GAP = 26;
  const avail = $derived(width - LEFT - SIDE);
  const COLS = $derived(Math.max(2, Math.floor((avail + GAP) / (300 + GAP))));
  const W = $derived((avail - GAP * (COLS - 1)) / COLS);
  const H = $derived(Math.round(W * 0.47));
  const ROW = $derived(H + 120);
  const TOP = 290;
  const BOTTOM = 110;
  const visibleRows = $derived(Math.max(1, Math.floor((height - TOP - BOTTOM) / ROW)));
  const firstRow = $derived(Math.max(0, Math.min(Math.floor(r / COLS) - (visibleRows > 1 ? 1 : 0), Math.ceil(results.length / COLS) - visibleRows)));

  function press(id: string) {
    feedback.move();
    if (id === "done") {
      if (results.length) ((zone = "results"), setR(0));
      return;
    }
    text = typeKey(text, id);
    setR(0);
  }
  function toResults() {
    if (results.length) ((zone = "results"), setR(Math.min(r, results.length - 1)));
    else zone = "filters";
  }
  function changeFilter(d: -1 | 1) {
    if (chip === 0) si = cycle(sources.length, si, d);
    else shi = cycle(SHOW_CHOICES.length, shi, d);
    setR(0);
    feedback.confirm();
  }
  const dirs = ["up", "down", "left", "right"];

  $effect(() =>
    useInput((intent) => {
      if (zone === "keys") {
        if (intent === "back") return onback();
        if (intent === "confirm" && typing) {
          if (results.length) ((zone = "results"), setR(0), feedback.move());
          else feedback.edge();
        } else if (intent === "confirm") press(KEYS[k]);
        else if (intent === "action") press("del");
        else if (intent === "info") press("space");
        else if (dirs.includes(intent)) {
          typing = false;
          const next = keyMove(k, intent);
          if (next === "out") toResults();
          else if (next === null) return feedback.edge();
          else k = next;
          feedback.move();
        } else return false;
        return;
      }
      if (zone === "filters") {
        if (intent === "back" || intent === "info") zone = "keys";
        else if (intent === "confirm") return changeFilter(1);
        else if (intent === "left" && chip === 0) zone = "keys";
        else if (intent === "left" || intent === "right") {
          const next = chip + (intent === "left" ? -1 : 1);
          if (next > 1) return feedback.edge();
          chip = next;
        } else if (intent === "down") {
          if (!results.length) return feedback.edge();
          zone = "results";
        } else if (intent === "up") return feedback.edge();
        else return false;
        feedback.move();
        return;
      }
      const x = results[r];
      if (intent === "back" || intent === "info") zone = "keys";
      else if (intent === "confirm" && x) return (feedback.confirm(), onopen(x.game));
      else if (intent === "action" && x) return (feedback.confirm(), void storefront.toggleWish(storefront.card(x.game)));
      else if (dirs.includes(intent)) {
        if (intent === "left" && r % COLS === 0) zone = "keys";
        else if (intent === "up" && r < COLS) zone = "filters";
        else {
          const j = gridStep(r, results.length, COLS, intent);
          if (j === null) return feedback.edge();
          setR(j);
        }
      } else return false;
      feedback.move();
    }),
  );

  function onkeydown(e: KeyboardEvent) {
    // A game's page, quick access or the launch sequence on top gets its keys.
    // On the filters and results the keys are shortcuts again (X wishlists).
    if (!active || zone !== "keys" || e.ctrlKey || e.altKey || e.metaKey) return;
    const t = e.target as HTMLElement | null;
    if (t?.tagName === "INPUT" || t?.tagName === "TEXTAREA") return;
    // Typed characters go into the search, not to the shortcuts (X, I, Q, …);
    // Backspace deletes while typing instead of going back.
    const space = e.key === " " && typing && text !== "";
    const del = e.key === "Backspace" && text !== "";
    if (!space && !del && (e.key === " " || !typable(e.key))) return;
    text = del ? text.slice(0, -1) : text + e.key;
    typing = true;
    setR(0);
    e.stopPropagation();
    e.preventDefault();
  }
  const wished = $derived(results[r] ? storefront.wished(results[r].game.key, results[r].game.wishlisted) : false);
</script>

<svelte:window onkeydowncapture={onkeydown} />

<div class="browse">
  <div class="left">
    <div class="field" class:focus={zone === "keys"}>
      <Icon name="search" size={28} stroke={2} />
      <span class="q">{text}<span class="caret"></span></span>
    </div>
    <div class="keys">
      {#each KEYS as id, idx (id)}
        <button type="button" class="key" class:wide={isWide(idx)} class:on={zone === "keys" && idx === k} tabindex="-1" style:grid-column={isWide(idx) ? wideColumn(idx) : undefined} onclick={() => ((k = idx), press(id))}>
          {keyLabel(id)}
        </button>
      {/each}
    </div>
    <p class="tip">Matches from the index show at once. Your sources and Steam are searched when you stop typing.</p>
  </div>

  <div class="right" style:left="{LEFT}px">
    <div class="filters">
      <button type="button" class="chip" class:on={zone === "filters" && chip === 0} tabindex="-1" onclick={() => ((chip = 0), changeFilter(1))}>
        <span class="cl">Source</span>{sources[si]?.label ?? "All sources"}<span class="arrow">›</span>
      </button>
      <button type="button" class="chip" class:on={zone === "filters" && chip === 1} tabindex="-1" onclick={() => ((chip = 1), changeFilter(1))}>
        <span class="cl">Show</span>{show.label}<span class="arrow">›</span>
      </button>
    </div>
    <p class="count">{countLine}{#if view?.searching}{" · "}<span class="busy">Searching…</span>{/if}</p>
    {#if progressLine}<p class="progress">{progressLine}</p>{/if}
  </div>

  {#if !view?.page}
    <p class="hint" style:left="{LEFT}px">Loading…</p>
  {:else if results.length === 0}
    <p class="hint" style:left="{LEFT}px">{text.trim() ? `Nothing matches “${text.trim()}”${view.searching ? " yet" : ""}.` : "No games match these filters."}</p>
  {/if}
  <div class="view" style:top="{TOP - 24}px" style:bottom="{BOTTOM}px" style:left="{LEFT - 30}px">
    <div class="grid" style:transform="translateY({-firstRow * ROW}px)">
      {#each results as x, idx (x.game.key)}
        <div class="cell" style:left="{30 + (idx % COLS) * (W + GAP)}px" style:top="{Math.floor(idx / COLS) * ROW}px">
          <StoreCard game={x.game} on={zone === "results" && idx === r} width={W} height={H} onclick={() => (zone === "results" && idx === r ? onopen(x.game) : ((zone = "results"), setR(idx)))} />
        </div>
      {/each}
    </div>
  </div>

  <div class="hints">
    <Hints
      hints={zone === "keys"
        ? [
            { button: "confirm", label: "Type" },
            { button: "action", label: "Delete" },
            { button: "info", label: "Space" },
            { button: "lt", also: "rt", label: "Tabs" },
            { button: "back", label: "Back" },
          ]
        : zone === "filters"
          ? [
              { button: "confirm", label: "Change" },
              { button: "back", label: "Keyboard" },
            ]
          : [
              { button: "confirm", label: "Open" },
              { button: "action", label: wished ? "Remove from wishlist" : "Add to wishlist" },
              { button: "back", label: "Keyboard" },
            ]}
    />
  </div>
</div>

<style>
  .browse {
    position: absolute;
    inset: 0;
  }
  .left {
    position: absolute;
    left: 110px;
    top: 140px;
    width: 660px;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }
  .field {
    height: 72px;
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 0 22px;
    border-radius: 18px;
    background: #121a23;
    border: 2px solid transparent;
    font-size: 28px;
    font-weight: 600;
    color: #9ba8b5;
    overflow: hidden;
  }
  .field.focus {
    border-color: oklch(0.8 0.12 205);
  }
  .q {
    color: #fff;
    white-space: nowrap;
  }
  .caret {
    display: inline-block;
    width: 3px;
    height: 32px;
    margin-left: 3px;
    vertical-align: middle;
    background: oklch(0.8 0.12 205);
    animation: blink 1s steps(1) infinite;
  }
  @keyframes blink {
    50% {
      opacity: 0;
    }
  }
  .keys {
    display: grid;
    grid-template-columns: repeat(10, minmax(0, 1fr));
    gap: 9px;
  }
  .key {
    height: 62px;
    border: 0;
    border-radius: 13px;
    background: #121a23;
    color: #e8edf2;
    font-size: 25px;
    font-weight: 600;
    text-transform: uppercase;
  }
  .key.wide {
    font-size: 20px;
    text-transform: none;
  }
  .key.on {
    background: #e8edf2;
    color: #0a0e13;
  }
  .tip {
    margin: 0;
    font-size: 17px;
    line-height: 1.45;
    color: #7f8c99;
  }
  .right {
    position: absolute;
    right: 110px;
    top: 140px;
  }
  .filters {
    display: flex;
    gap: 14px;
  }
  .chip {
    height: 56px;
    display: inline-flex;
    align-items: center;
    gap: 12px;
    padding: 0 22px;
    border: 0;
    border-radius: 28px;
    background: #121a23;
    color: #e8edf2;
    font-size: 20px;
    font-weight: 700;
  }
  .chip.on {
    background: #1b2632;
    box-shadow: 0 0 0 3px #fff;
  }
  .cl {
    color: #9ba8b5;
    font-weight: 600;
  }
  .arrow {
    color: #9ba8b5;
    font-size: 26px;
  }
  .count,
  .progress {
    margin: 14px 0 0;
    font-size: 18px;
    color: #9ba8b5;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .progress {
    margin-top: 4px;
    font-size: 16px;
    color: #7f8c99;
  }
  .busy {
    color: oklch(0.88 0.09 205);
  }
  .hint {
    position: absolute;
    top: 300px;
    right: 110px;
    font-size: 22px;
    color: #9ba8b5;
  }
  .view {
    position: absolute;
    right: 0;
    overflow: hidden;
    mask-image: linear-gradient(180deg, transparent 0, #000 24px, #000 calc(100% - 50px), transparent 100%);
  }
  .grid {
    position: absolute;
    inset: 24px 0 auto 0;
    transition: transform 0.4s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .cell {
    position: absolute;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 40px;
    left: 110px;
  }
</style>
