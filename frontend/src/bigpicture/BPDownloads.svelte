<script lang="ts">
  // The store's downloads, to look at from the couch: what's coming, how
  // far it got, and what's ready. Changing them is for desktop mode.
  import { progress, statusLine } from "../lib/downloads";
  import { feedback, useInput } from "../lib/input.svelte";
  import { shop } from "../lib/shop.svelte";
  import Hints from "./Hints.svelte";

  let { onback }: { onback: () => void } = $props();

  $effect(() => shop.start());

  // Still going first, newest first.
  const list = $derived(
    [...shop.downloads].sort((a, b) => Number(a.state === "installed") - Number(b.state === "installed") || b.created - a.created),
  );
  let scroller: HTMLDivElement | undefined = $state();

  $effect(() =>
    useInput((intent) => {
      switch (intent) {
        case "up":
        case "down":
          scroller?.scrollBy({ top: intent === "up" ? -240 : 240, behavior: "smooth" });
          feedback.move();
          return;
        case "back":
          onback();
          return;
      }
    }),
  );
</script>

<div class="screen">
  <div class="head">
    <h1>Downloads</h1>
    <span class="sub">{shop.active ? `${shop.active} on their way` : "Nothing on its way"}</span>
  </div>
  <div class="list" bind:this={scroller}>
    {#each list as d (d.id)}
      <div class="row" class:warn={d.state === "failed" || d.state === "blocked"}>
        <div class="title">{d.title}</div>
        <div class="status">{statusLine(d, shop.engine)}</div>
        {#if d.state === "downloading" || d.state === "queued" || d.state === "paused"}
          <div class="bar"><span style:width={`${progress(d) * 100}%`} class:paused={d.state !== "downloading"}></span></div>
        {/if}
      </div>
    {:else}
      <p class="none">Games you get from the store in desktop mode download here, also while you play (unless that's turned off).</p>
    {/each}
  </div>
  <div class="hints"><Hints left="Install, pause and remove downloads in desktop mode" hints={[{ button: "back", label: "Back" }]} /></div>
</div>

<style>
  .screen {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    background: radial-gradient(ellipse at 40% 45%, #111925, #06080b 70%);
    color: #e8edf2;
    padding: 56px 110px 40px;
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 28px;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 52px;
  }
  .sub {
    font-size: 22px;
    color: rgba(232, 237, 242, 0.7);
  }
  .list {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
    margin-top: 32px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .row {
    padding: 22px 28px;
    border-radius: 18px;
    background: rgba(255, 255, 255, 0.06);
  }
  .title {
    font-size: 26px;
    font-weight: 700;
  }
  .status {
    margin-top: 6px;
    font-size: 19px;
    color: rgba(232, 237, 242, 0.7);
  }
  .row.warn .status {
    color: #f3b35a;
  }
  .bar {
    height: 8px;
    margin-top: 14px;
    border-radius: 99px;
    background: rgba(255, 255, 255, 0.1);
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--accent-game, var(--accent));
    transition: width 0.6s ease;
  }
  .bar span.paused {
    background: rgba(232, 237, 242, 0.4);
  }
  .none {
    font-size: 22px;
    color: rgba(232, 237, 242, 0.7);
    max-width: 900px;
  }
  .hints {
    margin-top: 20px;
  }
</style>
