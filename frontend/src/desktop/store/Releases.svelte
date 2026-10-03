<script lang="ts">
  // A game's release choices, each labelled with the source it comes from.
  import Icon from "../../components/Icon.svelte";
  import { availabilityLabel, canPrepare, languagesLine, publishedText, sizeLine } from "../../lib/storefront";
  import type { GameDetails, Release } from "../../lib/types";

  let {
    details,
    installed,
    locked,
    onget,
    onopenpage,
  }: { details: GameDetails; installed: boolean; locked: boolean; onget: (r: Release) => void; onopenpage: (r: Release) => void } = $props();
</script>

<ul class="releases">
  {#each details.releases as r, i (r.id)}
    <li class:dim={r.availability === "unavailable"}>
      <div class="text">
        <span class="v">
          <span class="sf-chip">{r.sourceName}</span>
          {r.version || "Version not given"}
          {#if i === details.recommended && details.releases.length > 1}<span class="sf-chip accent" title={details.why.join("\n")}>Recommended</span>{/if}
          {#if r.newer}<span class="sf-chip accent">Newer than installed</span>{/if}
          {#if r.kind === "update"}<span class="sf-chip">Update</span>{/if}
        </span>
        <span class="line">{[sizeLine(r), languagesLine(r), publishedText(r.publishedAt)].filter(Boolean).join(" · ")}</span>
        <span class="line">
          <span class="sf-chip" class:accent={r.availability === "installable"} class:warn={r.availability === "unavailable" || r.availability === "manual"}>{availabilityLabel(r.availability)}</span>
          {#if r.availability === "summary"}Release details are loading.{/if}
          {#if r.kind === "update"}A patch for an installed game. It can't be installed on its own.{/if}
          {#if r.kind === "preview"}The source only announces this release. There is nothing to download yet.{/if}
          {#if r.browserOnly && r.kind !== "preview"}This source's files open in your browser. Seaglass can't download or install them.{/if}
        </span>
        {#each r.unresolved as u, j (j)}<span class="line why"><Icon name="info" size={13} stroke={2.2} />{u}</span>{/each}
        {#if r.warnings.length}
          <details>
            <summary>{r.warnings.length} {r.warnings.length === 1 ? "warning" : "warnings"}</summary>
            <ul>{#each r.warnings as w, j (j)}<li>{w}</li>{/each}</ul>
          </details>
        {/if}
      </div>
      <div class="btns">
        {#if canPrepare(r)}
          <button type="button" class={r.availability === "installable" ? "sf-btn" : "sf-btn"} disabled={locked} onclick={() => onget(r)}>
            {r.availability === "installable" ? (installed ? "Install over it" : "Download") : "Check release"}
          </button>
        {/if}
        {#if r.pageUrl}<button type="button" class="sf-btn" aria-label={`Open the ${r.sourceName} page for ${r.version || r.title}`} onclick={() => onopenpage(r)}><Icon name="link" size={15} />Release page</button>{/if}
      </div>
    </li>
  {/each}
</ul>

<style>
  .releases {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .releases > li {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 12px 12px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .releases > li.dim .text {
    opacity: 0.7;
  }
  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .v {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    font-weight: 700;
  }
  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    font-size: 13.5px;
    color: var(--muted);
  }
  .line.why {
    color: var(--warn);
    align-items: flex-start;
    overflow-wrap: anywhere;
  }
  details {
    font-size: 13.5px;
    color: var(--muted);
  }
  summary {
    cursor: pointer;
    width: fit-content;
    border-radius: 6px;
  }
  details ul {
    margin: 4px 0 0;
    padding-left: 20px;
  }
  .btns {
    display: flex;
    flex-direction: column;
    gap: 6px;
    flex-shrink: 0;
  }
  @media (max-width: 700px) {
    .releases > li {
      flex-direction: column;
      align-items: stretch;
    }
    .btns {
      flex-direction: row;
      flex-wrap: wrap;
    }
  }
</style>
