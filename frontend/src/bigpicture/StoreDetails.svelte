<script lang="ts">
  // A Store game from the couch: its art, reviews and completion times on
  // the left, its releases on the right. Left / right moves between the
  // buttons and the releases, up / down along them; A on a release opens the
  // install confirmation (or its page when it can't be checked).
  import { untrack } from "svelte";
  import GameArt from "../components/GameArt.svelte";
  import Icon, { type IconName } from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { completionTimes, scoreLine, sourcesText } from "../lib/bpstore";
  import { feedback, useInput } from "../lib/input.svelte";
  import { shop } from "../lib/shop.svelte";
  import { errText, lib } from "../lib/store.svelte";
  import { availabilityLabel, canPrepare, freshness, languagesLine, publishedText, sizeLine } from "../lib/storefront";
  import { storefront } from "../lib/storefront.svelte";
  import type { Enrichment, GameDetails, GameSummary, Release } from "../lib/types";
  import Hints from "./Hints.svelte";
  import StoreInstall from "./StoreInstall.svelte";

  let { game, onclose, ondownloads }: { game: GameSummary; onclose: () => void; ondownloads: () => void } = $props();

  const gameKey = untrack(() => game.key);
  let details = $state<GameDetails | null>(null);
  let enrichment = $state<Enrichment | null>(null);
  let failed = $state("");
  let flow = $state<{ release: Release; update: boolean } | null>(null);
  let seq = 0;

  // Steam games with no release have no source page: the card's own summary is the page.
  const bare = (): GameDetails => ({
    summary: game,
    releases: [],
    recommended: -1,
    why: [],
    identity: { steamAppId: game.steamAppId ?? 0, name: game.steamAppId ? game.title : undefined, how: "", corrected: false },
    loading: false,
  });

  async function loadDetails() {
    const mine = ++seq;
    try {
      const d = await api.store.discovery.game(gameKey);
      if (mine === seq) [details, failed] = [d, ""];
    } catch (e) {
      if (mine !== seq) return;
      if (!game.sourceBacked) [details, failed] = [bare(), ""];
      else failed = errText(e);
    }
  }
  async function loadEnrichment() {
    try {
      enrichment = await api.store.enrich.game(gameKey);
    } catch {
      /* the boxes say "unavailable"; nothing else to do */
    }
  }
  $effect(() => {
    untrack(() => {
      void loadDetails();
      void loadEnrichment();
    });
    shop.requestArt([gameKey]);
  });
  $effect(() =>
    api.store.discovery.onGames((c) => {
      if (c.all || c.keys.includes(gameKey)) void loadDetails();
    }),
  );
  $effect(() =>
    api.store.enrich.onEnrichment((e) => {
      if (e.key === gameKey) enrichment = e;
    }),
  );

  const s = $derived(details?.summary ?? game);
  const m = $derived(shop.art[gameKey]);
  const art = $derived({ key: gameKey, meta: m });
  const wished = $derived(storefront.wished(gameKey, s.wishlisted));
  const onWay = $derived(shop.downloads.find((d) => d.gameKey === gameKey && d.state !== "failed" && d.state !== "installed"));
  const releases = $derived(details?.releases ?? []);
  const rec = $derived(details && details.recommended >= 0 ? details.releases[details.recommended] : undefined);
  // The release to offer first: the recommended one, else the newest that can be checked.
  const top = $derived(rec && canPrepare(rec) ? rec : releases.find(canPrepare));
  const ready = $derived(top?.availability === "installable");
  const times = $derived(completionTimes(enrichment));
  const facts = $derived(
    [m?.developers?.[0], m?.releaseYear ? String(m.releaseYear) : "", (m?.genres?.length ? m.genres : s.genres).slice(0, 3).join(" · ")].filter(Boolean).join("  ·  "),
  );

  type Action = { id: "get" | "downloads" | "wish" | "retry"; label: string; icon: IconName };
  const actions = $derived.by<Action[]>(() => {
    const out: Action[] = [];
    if (failed) out.push({ id: "retry", label: "Try again", icon: "refresh" });
    if (onWay) out.push({ id: "downloads", label: onWay.state === "downloaded" ? "Downloaded" : "In Downloads", icon: "download" });
    else if (top && s.installed?.update) out.push({ id: "get", label: `${ready ? "Update to" : "Check"} ${top.version || "the newest"}`, icon: "sparkle" });
    else if (top && !s.installed) out.push({ id: "get", label: `${ready ? "Get" : "Check"} ${top.version || "this release"}`, icon: "download" });
    out.push({ id: "wish", label: wished ? "On your wishlist" : "Add to wishlist", icon: "star" });
    return out;
  });

  // Focus: the buttons, or one release. Kept while the install confirmation is open.
  let zone = $state<"actions" | "releases">("actions");
  let a = $state(0);
  let r = $state(0);
  $effect(() => {
    a = Math.min(a, actions.length - 1);
    r = Math.min(r, Math.max(0, releases.length - 1));
    if (zone === "releases" && !releases.length) zone = "actions";
  });

  let list: HTMLDivElement | undefined = $state();
  $effect(() => {
    if (zone === "releases") list?.querySelector<HTMLElement>(`[data-row="${r}"]`)?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  });

  function get(rel: Release) {
    flow = { release: rel, update: !!s.installed };
  }
  const pageOf = (rel: Release) => void lib.run(() => api.store.discovery.openRelease(gameKey, rel.id));
  // What A does on a release: nothing is checked twice while it's on its way.
  const releaseAction = (rel: Release | undefined): "get" | "page" | "" =>
    !rel ? "" : canPrepare(rel) && !onWay ? "get" : rel.pageUrl ? "page" : "";
  const releaseLabel = (rel: Release) =>
    releaseAction(rel) === "page" ? "Open release page" : rel.availability === "installable" ? (s.installed ? "Install over it" : "Download") : "Check release";

  function press(act: Action | undefined) {
    if (!act) return feedback.edge();
    feedback.confirm();
    if (act.id === "get" && top) get(top);
    else if (act.id === "downloads") ondownloads();
    else if (act.id === "wish") storefront.toggleWish({ ...s, wishlisted: wished });
    else if (act.id === "retry") void loadDetails();
  }
  function pressRelease(rel: Release | undefined) {
    const what = releaseAction(rel);
    if (!rel || !what) return feedback.edge();
    feedback.confirm();
    if (what === "get") get(rel);
    else pageOf(rel);
  }

  $effect(() =>
    useInput((intent) => {
      switch (intent) {
        case "left":
        case "right": {
          const d = intent === "left" ? -1 : 1;
          if (zone === "releases") {
            if (d < 0) ((zone = "actions"), feedback.move());
            else feedback.edge();
          } else if (a + d >= 0 && a + d < actions.length) ((a += d), feedback.move());
          else if (d > 0 && releases.length) ((zone = "releases"), feedback.move());
          else feedback.edge();
          return;
        }
        case "up":
        case "down": {
          const d = intent === "up" ? -1 : 1;
          if (zone === "actions") {
            if (d > 0 && releases.length) ((zone = "releases"), feedback.move());
            else feedback.edge();
          } else if (r + d >= 0 && r + d < releases.length) ((r += d), feedback.move());
          else if (d < 0) ((zone = "actions"), feedback.move());
          else feedback.edge();
          return;
        }
        case "confirm":
          if (zone === "actions") press(actions[a]);
          else pressRelease(releases[r]);
          return;
        case "action":
          press(actions.find((x) => x.id === "wish"));
          return;
        case "info":
          if (zone === "releases" && releases[r]?.pageUrl) (feedback.confirm(), pageOf(releases[r]));
          else feedback.edge();
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

  const focused = $derived(zone === "releases" ? releases[r] : undefined);
  const wishLabel = $derived(wished ? "Remove from wishlist" : "Add to wishlist");
  const onWish = $derived(zone === "actions" && actions[a]?.id === "wish");
  const confirmLabel = $derived(onWish ? wishLabel : zone === "actions" ? (actions[a]?.label ?? "") : focused && releaseAction(focused) ? releaseLabel(focused) : "");
</script>

<div class="sheet">
  <div class="art"><GameArt game={art} kind="backdrop" /></div>
  <div class="shade"></div>

  <div class="left">
    {#if s.sources.length}<span class="src">{sourcesText(s)}</span>{/if}
    <h1>{s.title}</h1>
    {#if facts}<div class="line">{facts}</div>{/if}
    <div class="buttons">
      {#each actions as act, k (act.id)}
        <button
          type="button"
          class="btn"
          class:primary={act.id === "get"}
          class:on={zone === "actions" && k === a && !flow}
          tabindex="-1"
          onclick={() => ((zone = "actions"), (a = k), press(act))}
        >
          <Icon name={act.icon} size={22} />{act.label}
        </button>
      {/each}
    </div>
    {#if s.installed && !onWay}
      <div class="note">{s.installed.update ? `${s.installed.version} is installed` : `Installed ${s.installed.version}`}</div>
    {/if}
    {#if details && details.recommended >= 0 && details.releases.length > 1 && !onWay}
      <div class="note">Recommended: {details.why.join(" · ")}</div>
    {/if}
    {#if failed}<div class="note warn" role="alert">{failed}</div>{/if}
    {#if m?.description}<p class="desc">{m.description}</p>{/if}

    <div class="boxes">
      <section class="box">
        <h3>Steam reviews</h3>
        {#if !enrichment || enrichment.reviews.state === "loading"}
          <p class="muted">Loading…</p>
        {:else if enrichment.reviews.state === "unavailable" || enrichment.reviews.state === "error" || !enrichment.reviews.appId}
          <p class="muted">{enrichment.reviews.error || "No Steam reviews are available for this game."}</p>
        {:else}
          <dl>
            <dt>Overall</dt><dd>{scoreLine(enrichment.reviews.overall) || "No user reviews"}</dd>
            <dt>Recent</dt><dd>{scoreLine(enrichment.reviews.recent) || "No recent reviews"}</dd>
          </dl>
          {#if freshness(enrichment.reviews.state, enrichment.reviews.fetchedAt)}<p class="muted">{freshness(enrichment.reviews.state, enrichment.reviews.fetchedAt)}</p>{/if}
        {/if}
        {#if enrichment && enrichment.critic.score > 0}
          <p class="muted">Metacritic <strong>{enrichment.critic.score}</strong></p>
        {/if}
      </section>
      <section class="box">
        <h3>How long to beat</h3>
        {#if !enrichment || enrichment.completion.state === "loading"}
          <p class="muted">Loading…</p>
        {:else if times.length}
          <dl>
            {#each times as t (t.label)}<dt>{t.label}</dt><dd>{t.time}</dd>{/each}
          </dl>
          {#if freshness(enrichment.completion.state, enrichment.completion.fetchedAt)}<p class="muted">{freshness(enrichment.completion.state, enrichment.completion.fetchedAt)}</p>{/if}
        {:else}
          <p class="muted">Times unavailable{enrichment.completion.error ? `: ${enrichment.completion.error}` : ""}.</p>
        {/if}
      </section>
    </div>
  </div>

  <div class="right">
    <h2>Releases{#if releases.length}<span class="count">{releases.length}</span>{/if}</h2>
    {#if details?.loading}<p class="muted">Loading release details…</p>{/if}
    {#if !details && !failed}
      <p class="muted">Loading…</p>
    {:else if details && !releases.length}
      <p class="muted">No source has a release of this game yet. Add it to your wishlist and it shows up here when one appears.</p>
    {/if}
    <div class="list" bind:this={list}>
      {#each releases as rel, k (rel.id)}
        <button
          type="button"
          class="rel"
          class:on={zone === "releases" && k === r && !flow}
          class:dim={rel.availability === "unavailable"}
          data-row={k}
          tabindex="-1"
          onclick={() => ((zone = "releases"), (r = k), pressRelease(rel))}
        >
          <span class="v">
            <span class="chip">{rel.sourceName}</span>
            {rel.version || "Version not given"}
            {#if details && k === details.recommended && releases.length > 1}<span class="chip accent">Recommended</span>{/if}
            {#if rel.newer}<span class="chip accent">Newer than installed</span>{/if}
            {#if rel.kind === "update"}<span class="chip">Update</span>{/if}
          </span>
          <span class="rl">{[sizeLine(rel), languagesLine(rel), publishedText(rel.publishedAt)].filter(Boolean).join(" · ")}</span>
          <span class="rl">
            <span class="chip" class:accent={rel.availability === "installable"} class:warn={rel.availability === "unavailable" || rel.availability === "manual"}>{availabilityLabel(rel.availability)}</span>
            {#if rel.availability === "summary"}Release details are loading.{/if}
            {#if rel.kind === "update"}A patch for an installed game. It can't be installed on its own.{/if}
          </span>
          {#each rel.unresolved.slice(0, 2) as u, j (j)}<span class="rl why">{u}</span>{/each}
        </button>
      {/each}
    </div>
  </div>

  {#if flow}
    <StoreInstall {gameKey} release={flow.release} update={flow.update} onclose={() => (flow = null)} onqueued={() => ((flow = null), ondownloads())} />
  {:else}
    <div class="hints">
      <Hints
        hints={[
          ...(confirmLabel ? [{ button: "confirm" as const, label: confirmLabel }] : []),
          ...(onWish ? [] : [{ button: "action" as const, label: wishLabel }]),
          ...(focused?.pageUrl ? [{ button: "info" as const, label: "Release page" }] : []),
          { button: "back", label: "Back" },
        ]}
      />
    </div>
  {/if}
</div>

<style>
  .sheet {
    position: absolute;
    inset: 0;
    z-index: 25;
    background: #06080b;
    color: #f3f5f7;
    animation: in 0.3s ease both;
  }
  @keyframes in {
    from {
      opacity: 0;
      transform: scale(1.01);
    }
  }
  .art {
    position: absolute;
    inset: 0;
  }
  .shade {
    position: absolute;
    inset: 0;
    background:
      linear-gradient(90deg, rgba(6, 8, 11, 0.95) 0%, rgba(6, 8, 11, 0.82) 45%, rgba(6, 8, 11, 0.9) 60%, rgba(6, 8, 11, 0.96) 100%),
      linear-gradient(0deg, rgba(6, 8, 11, 0.9) 0%, rgba(6, 8, 11, 0) 50%);
  }
  .left {
    position: absolute;
    left: 110px;
    top: 80px;
    bottom: 130px;
    width: 900px;
    display: flex;
    flex-direction: column;
    gap: 18px;
    overflow: hidden;
  }
  .src {
    width: fit-content;
    padding: 7px 14px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.24);
    font-size: 15px;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  h1 {
    flex-shrink: 0;
    margin: 0;
    font-family: var(--font-display);
    font-size: 76px;
    line-height: 1;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .line {
    font-size: 22px;
    color: rgba(243, 245, 247, 0.82);
  }
  .buttons {
    display: flex;
    flex-wrap: wrap;
    gap: 14px;
    margin-top: 4px;
  }
  .btn {
    height: 68px;
    padding: 0 26px;
    border-radius: 34px;
    border: 1px solid rgba(255, 255, 255, 0.2);
    background: rgba(14, 18, 24, 0.6);
    color: #f3f5f7;
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 22px;
    font-weight: 700;
  }
  .btn.primary {
    background: #f3f5f7;
    color: #06080b;
    border-color: transparent;
    padding: 0 36px;
  }
  .btn.on {
    box-shadow:
      0 0 0 4px #06080b,
      0 0 0 7px #f3f5f7;
  }
  .note {
    font-size: 19px;
    color: rgba(243, 245, 247, 0.65);
  }
  .note.warn {
    color: #f3b35a;
  }
  .desc {
    flex-shrink: 0;
    margin: 0;
    font-size: 20px;
    line-height: 1.5;
    color: rgba(243, 245, 247, 0.75);
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .boxes {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 14px;
  }
  .box {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 18px 22px;
    border-radius: 18px;
    background: rgba(18, 26, 35, 0.85);
    min-width: 0;
  }
  h3 {
    margin: 0;
    font-size: 17px;
    font-weight: 800;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: #7f8c99;
  }
  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 6px 18px;
    margin: 0;
    font-size: 19px;
  }
  dt {
    color: #9ba8b5;
  }
  dd {
    margin: 0;
    font-weight: 700;
    overflow-wrap: anywhere;
  }
  .muted {
    margin: 0;
    font-size: 18px;
    color: #9ba8b5;
  }
  .muted strong {
    color: #f3f5f7;
  }
  .right {
    position: absolute;
    left: 1080px;
    right: 110px;
    top: 80px;
    bottom: 130px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  h2 {
    display: flex;
    align-items: baseline;
    gap: 14px;
    margin: 0;
    font-family: var(--font-display);
    font-size: 40px;
  }
  .count {
    font-size: 20px;
    color: #9ba8b5;
  }
  .list {
    flex: 1;
    min-height: 0;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 6px;
    mask-image: linear-gradient(180deg, transparent 0, #000 12px, #000 calc(100% - 40px), transparent 100%);
    scroll-padding-block: 18px 48px;
  }
  .rel {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 18px 22px;
    border: 0;
    border-radius: 18px;
    background: #121a23;
    color: #e8edf2;
    text-align: left;
  }
  .rel.on {
    background: #1b2632;
    box-shadow: 0 0 0 3px #fff;
  }
  .rel.dim {
    opacity: 0.7;
  }
  .v {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    font-size: 22px;
    font-weight: 700;
  }
  .rl {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    font-size: 17px;
    color: #9ba8b5;
  }
  .rl.why {
    color: #f3b35a;
    overflow-wrap: anywhere;
  }
  .chip {
    padding: 3px 10px;
    border-radius: 8px;
    background: rgba(255, 255, 255, 0.1);
    color: #e8edf2;
    font-size: 15px;
    font-weight: 700;
  }
  .chip.accent {
    background: rgba(79, 209, 232, 0.16);
    color: oklch(0.88 0.09 205);
  }
  .chip.warn {
    background: rgba(243, 179, 90, 0.16);
    color: #f3b35a;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 48px;
    left: 110px;
    z-index: 3;
  }
</style>
