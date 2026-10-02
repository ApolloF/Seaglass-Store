<script lang="ts">
  // One catalog game: its art and description, and every version the
  // feeds offer.
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import { offerLine } from "../lib/catalog";
  import { shop } from "../lib/shop.svelte";
  import type { CatalogEntry } from "../lib/types";
  import InstallDialog from "./InstallDialog.svelte";

  let { entry, onback }: { entry: CatalogEntry; onback: () => void } = $props();

  $effect(() => shop.requestArt([entry.key]));
  const m = $derived(shop.art[entry.key]);
  const art = $derived({ key: entry.key, meta: m });
  const facts = $derived(
    [
      m?.developers?.length && ["Developer", m.developers.join(", ")],
      m?.genres?.length && ["Genre", m.genres.join(", ")],
      (m?.releaseDate || m?.releaseYear) && ["Released", m.releaseDate || String(m.releaseYear)],
      entry.languages.length && ["Languages", entry.languages.join(", ")],
    ].filter((f): f is [string, string] => !!f),
  );
  const onWay = $derived(shop.downloads.find((d) => d.gameKey === entry.key && d.state !== "failed"));
  let getting = $state<number | null>(null);
</script>

<div class="page">
  <div class="hero">
    <GameArt game={art} kind="hero" />
    <div class="shade"></div>
    <button type="button" class="back" onclick={onback}><Icon name="chevronDown" size={16} stroke={2.4} />Store</button>
  </div>

  <div class="body">
    <div class="cover"><GameArt game={art} /></div>
    <div class="main">
      <h1>{entry.title}</h1>
      <div class="cta">
        {#if onWay}
          <span class="on-way"><Icon name="download" size={18} />{onWay.state === "downloaded" ? "Downloaded" : "In Downloads"}</span>
        {:else}
          <button type="button" class="get" onclick={() => (getting = 0)}><Icon name="download" size={18} />Get {entry.version}</button>
        {/if}
        <span class="sub">{entry.offers.length} {entry.offers.length === 1 ? "version" : "versions"}{entry.updated ? ` · updated ${entry.updated}` : ""}</span>
      </div>
      {#if m?.description}<p class="desc">{m.description}</p>{/if}
      {#if facts.length}
        <dl class="facts">
          {#each facts as [k, v] (k)}<dt>{k}</dt><dd>{v}</dd>{/each}
        </dl>
      {/if}

      <h2>Versions</h2>
      <ul class="offers">
        {#each entry.offers as o, i (i)}
          <li>
            <div class="text">
              <span class="v">{o.version || "Version not given"}{#if i === 0 && entry.offers.length > 1}<span class="newest">Newest</span>{/if}</span>
              <span class="line">{offerLine(o)}</span>
              {#if o.languages?.length}<span class="line">{o.languages.join(", ")}</span>{/if}
              {#if o.notes}<span class="line">{o.notes}</span>{/if}
            </div>
            <button type="button" class="btn" disabled={!!onWay} onclick={() => (getting = i)}>Download</button>
          </li>
        {/each}
      </ul>
    </div>
  </div>
</div>

{#if getting !== null}
  <InstallDialog {entry} offer={getting} onclose={() => (getting = null)} />
{/if}

<style>
  .page {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }
  .hero {
    position: relative;
    height: 260px;
    overflow: hidden;
  }
  .hero :global(img),
  .hero :global(.art) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
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
  .cover :global(img),
  .cover :global(.art) {
    width: 100%;
    height: 100%;
    object-fit: cover;
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
    gap: 14px;
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
  .on-way {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--accent-text);
    font-weight: 700;
  }
  .sub,
  .line {
    font-size: 13.5px;
    color: var(--muted);
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
  .offers {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .offers li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 12px 12px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .v {
    display: flex;
    align-items: center;
    gap: 8px;
    font-weight: 700;
  }
  .newest {
    padding: 1px 7px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--accent-text);
    font-size: 11.5px;
  }
  .btn {
    flex-shrink: 0;
    height: 38px;
    padding: 0 14px;
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: transparent;
    font-size: 14.5px;
    font-weight: 700;
  }
  .btn:hover:not(:disabled) {
    background: var(--surface-3);
  }
  .btn:disabled {
    opacity: 0.5;
  }
  @media (max-width: 700px) {
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
  }
</style>
