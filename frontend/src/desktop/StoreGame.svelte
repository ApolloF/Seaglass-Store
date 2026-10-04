<script lang="ts">
  // One game: its art and description, where its releases come from, Steam
  // reviews, completion times, and the way into the install confirmation.
  import { untrack } from "svelte";
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { shop } from "../lib/shop.svelte";
  import { errText, lib } from "../lib/store.svelte";
  import { canPrepare, dateText, publishedText } from "../lib/storefront";
  import { storefront } from "../lib/storefront.svelte";
  import type { Enrichment, GameDetails, GameSummary, Release } from "../lib/types";
  import CompletionPicker from "./store/CompletionPicker.svelte";
  import Ratings from "./store/Ratings.svelte";
  import ReleaseFlow from "./store/ReleaseFlow.svelte";
  import Releases from "./store/Releases.svelte";
  import Reviews from "./store/Reviews.svelte";
  import SteamMatch from "./store/SteamMatch.svelte";

  let { gameKey, initial, onback, onkey }: { gameKey: string; initial: GameSummary; onback: () => void; onkey: (key: string) => void } = $props();

  let details = $state<GameDetails | null>(null);
  let enrichment = $state<Enrichment | null>(null);
  let failed = $state("");
  let flow = $state<{ release: Release; update: boolean } | null>(null);
  let matching = $state(false);
  let picking = $state(false);
  let seq = 0;
  let heading: HTMLHeadingElement | undefined = $state();

  // Steam games with no release have no source page: the card's own summary is the page.
  const bare = (): GameDetails => ({
    summary: initial,
    releases: [],
    recommended: -1,
    why: [],
    identity: { steamAppId: initial.steamAppId ?? 0, name: initial.steamAppId ? initial.title : undefined, how: "", corrected: false },
    loading: false,
  });

  async function loadDetails(key: string) {
    const mine = ++seq;
    try {
      const d = await api.store.discovery.game(key);
      if (mine === seq) [details, failed] = [d, ""];
    } catch (e) {
      if (mine !== seq) return;
      if (!initial.sourceBacked) [details, failed] = [bare(), ""];
      else failed = errText(e);
    }
  }
  async function loadEnrichment(key: string) {
    try {
      const e = await api.store.enrich.game(key);
      if (key === gameKey) enrichment = e;
    } catch {
      /* the cards say "unavailable"; nothing else to do */
    }
  }

  // Loads when opened and when the key changes (a corrected Steam match).
  $effect(() => {
    const key = gameKey;
    untrack(() => {
      [details, enrichment, failed] = [null, null, ""];
      void loadDetails(key);
      void loadEnrichment(key);
    });
    shop.requestArt([key]);
    heading?.focus();
  });
  // Release details arrive later (and a wishlist change touches the page).
  $effect(() =>
    api.store.discovery.onGames((c) => {
      if (c.all || c.keys.includes(gameKey)) void loadDetails(gameKey);
    }),
  );
  $effect(() =>
    api.store.enrich.onEnrichment((e) => {
      if (e.key === gameKey) enrichment = e;
    }),
  );

  const s = $derived(details?.summary ?? initial);
  const m = $derived(shop.art[gameKey]);
  const art = $derived({ key: gameKey, meta: m });
  const wished = $derived(storefront.wished(gameKey, s.wishlisted));
  const onWay = $derived(shop.downloads.find((d) => d.gameKey === gameKey && d.state !== "failed" && d.state !== "installed"));
  const rec = $derived(details && details.recommended >= 0 ? details.releases[details.recommended] : undefined);
  const identity = $derived(details?.identity);
  const how = $derived(identity?.how === "correction" ? "chosen by you" : identity?.how === "feed" ? "from a feed" : identity?.how === "game-database" ? "matched by Seaglass" : "");
  const facts = $derived(
    [
      m?.developers?.length && ["Developer", m.developers.join(", ")],
      (m?.genres?.length || s.genres.length) && ["Genre", (m?.genres?.length ? m.genres : s.genres).join(", ")],
      (m?.releaseDate || m?.releaseYear) && ["Game released", m.releaseDate || String(m.releaseYear)],
      s.publishedAt > 0 && ["Newest release", publishedText(s.publishedAt).replace("Published ", "")],
    ].filter((f): f is [string, string] => !!f),
  );

  function get(r: Release) {
    flow = { release: r, update: !!s.installed };
  }
  function pageOf(r: Release) {
    void lib.run(() => api.store.discovery.openRelease(gameKey, r.id));
  }
  function matched(d: GameDetails) {
    details = d;
    if (d.summary.key !== gameKey) onkey(d.summary.key);
  }
  // The release to offer first: the recommended one, else the newest that can be checked.
  const top = $derived(rec && canPrepare(rec) ? rec : details?.releases.find(canPrepare));
  const ready = $derived(top?.availability === "installable");
</script>

<div class="page" onkeydown={(e) => e.key === "Escape" && !flow && !matching && !picking && onback()} role="presentation">
  <div class="hero">
    <GameArt game={art} kind="hero" />
    <div class="shade"></div>
    <button type="button" class="back" onclick={onback}><Icon name="chevronDown" size={16} stroke={2.4} />Store</button>
  </div>

  <div class="body">
    <div class="cover"><GameArt game={art} /></div>
    <div class="main">
      <h1 tabindex="-1" bind:this={heading}>{s.title}</h1>
      <div class="cta">
        {#if onWay}
          <span class="on-way"><Icon name="download" size={18} />{onWay.state === "downloaded" ? "Downloaded" : "In Downloads"}</span>
        {:else if top && s.installed?.update}
          <button type="button" class="get" onclick={() => get(top)}><Icon name="sparkle" size={18} />{ready ? "Update to" : "Check"} {top.version || "the newest"}</button>
          <span class="sub">{s.installed.version} is installed</span>
        {:else if s.installed}
          <span class="on-way"><Icon name="check" size={18} />Installed {s.installed.version}</span>
        {:else if top}
          <button type="button" class="get" onclick={() => get(top)}><Icon name="download" size={18} />{ready ? "Get" : "Check"} {top.version || "this release"}</button>
        {/if}
        <button type="button" class="sf-btn wish" aria-pressed={wished} onclick={() => storefront.toggleWish({ ...s, wishlisted: wished })}>
          <Icon name="star" size={16} stroke={2.2} />{wished ? "On your wishlist" : "Add to wishlist"}
        </button>
        {#if details}
          <span class="sub">{details.releases.length ? `${details.releases.length} ${details.releases.length === 1 ? "release" : "releases"}` : "No known source release"}</span>
        {/if}
      </div>
      {#if details && details.recommended >= 0 && details.releases.length > 1 && !onWay}
        <p class="why">Recommended: {details.why.join(" · ")}</p>
      {/if}
      {#if details?.loading}<p class="sub" aria-live="polite">Loading release details…</p>{/if}
      {#if failed}<p class="err" role="alert">{failed} <button type="button" class="sf-link" onclick={() => loadDetails(gameKey)}>Try again</button></p>{/if}

      {#if m?.description}<p class="desc">{m.description}</p>{/if}
      {#if facts.length}
        <dl class="facts">
          {#each facts as [k, v] (k)}<dt>{k}</dt><dd>{v}</dd>{/each}
        </dl>
      {/if}

      <div class="steam">
        {#if identity?.steamAppId}
          <span>Steam: {identity.name || s.title} ({identity.steamAppId}){how ? `, ${how}` : ""}</span>
        {:else if identity}
          <span>{identity.corrected ? "Marked as not on Steam" : "No Steam match"}</span>
        {/if}
        {#if details}<button type="button" class="sf-link" onclick={() => (matching = true)}>Change Steam match</button>{/if}
      </div>

      <h2>Ratings</h2>
      <Ratings e={enrichment} onpick={() => (picking = true)} />

      <h2>Releases</h2>
      {#if !details && !failed}
        <p class="sub" aria-busy="true">Loading…</p>
      {:else if details && details.releases.length}
        <Releases {details} installed={!!s.installed} locked={!!onWay} onget={get} onopenpage={pageOf} />
      {:else if details}
        <p class="sub">No source has a release of this game yet. Add it to your wishlist and it will show up here when one appears.</p>
      {/if}

      {#if enrichment?.steamAppId}
        {#key enrichment.steamAppId}<Reviews appId={enrichment.steamAppId} />{/key}
      {/if}
    </div>
  </div>
</div>

{#if flow}
  <ReleaseFlow gameKey={gameKey} release={flow.release} update={flow.update} onclose={() => (flow = null)} />
{/if}
{#if matching}
  <SteamMatch {gameKey} title={s.title} ondone={matched} onclose={() => (matching = false)} />
{/if}
{#if picking}
  <CompletionPicker {gameKey} title={s.title} corrected={!!enrichment?.completion.corrected} ondone={(e) => (enrichment = e)} onclose={() => (picking = false)} />
{/if}

<style>
  .page {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    overflow-x: hidden;
  }
  .hero {
    position: relative;
    height: 260px;
    overflow: hidden;
  }
  .shade {
    position: absolute;
    inset: 0;
    background: linear-gradient(to bottom, transparent 40%, var(--bg));
  }
  .back {
    position: absolute;
    top: 16px;
    left: 20px;
    height: 36px;
    padding: 0 14px 0 10px;
    border: 0;
    border-radius: 10px;
    background: var(--scrim);
    color: #fff;
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: 700;
  }
  .back :global(svg) {
    transform: rotate(90deg);
  }
  .body {
    display: flex;
    gap: 28px;
    padding: 0 28px 32px;
    margin-top: -110px;
    position: relative;
  }
  .cover {
    position: relative;
    align-self: flex-start;
    width: 200px;
    flex-shrink: 0;
    aspect-ratio: 2 / 3;
    border-radius: var(--radius);
    overflow: hidden;
    box-shadow: var(--shadow);
    background: var(--surface-2);
  }
  .main {
    flex: 1;
    min-width: 0;
    padding-top: 70px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 36px;
    line-height: 1.1;
    outline: none;
    overflow-wrap: anywhere;
  }
  h2 {
    margin: 10px 0 0;
    font-family: var(--font-display);
    font-size: 22px;
  }
  .cta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px 14px;
  }
  .get {
    height: 44px;
    padding: 0 20px;
    border: 0;
    border-radius: 10px;
    background: var(--accent);
    color: var(--accent-ink);
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 15.5px;
    font-weight: 700;
  }
  .get:hover {
    filter: brightness(1.08);
  }
  .wish {
    min-height: 44px;
  }
  .wish[aria-pressed="true"] {
    color: var(--accent-text);
  }
  .on-way {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--accent-text);
    font-weight: 700;
  }
  .sub {
    margin: 0;
    font-size: 13.5px;
    color: var(--muted);
  }
  .why {
    margin: -4px 0 0;
    font-size: 13.5px;
    color: var(--muted);
  }
  .err {
    margin: 0;
    color: var(--warn);
  }
  .desc {
    margin: 0;
    max-width: 70ch;
    color: var(--text-2);
    line-height: 1.5;
  }
  .facts {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 6px 18px;
    margin: 0;
    font-size: 14px;
  }
  .facts dt {
    color: var(--muted);
  }
  .facts dd {
    margin: 0;
    color: var(--text-2);
  }
  .steam {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    font-size: 14px;
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  @media (max-width: 700px) {
    .hero {
      height: 200px;
    }
    .body {
      flex-direction: column;
      gap: 16px;
      padding: 0 16px 24px;
    }
    .cover {
      width: 130px;
    }
    .main {
      padding-top: 0;
    }
    h1 {
      font-size: 30px;
    }
    .back {
      left: 12px;
    }
  }
</style>
