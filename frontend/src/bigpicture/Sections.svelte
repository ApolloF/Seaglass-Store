<script lang="ts" module>
  export type { Section } from "../lib/bpstore";
</script>

<script lang="ts">
  // Big picture's sections, switched with L1 / R1 from anywhere (or clicked).
  // The Store is one only while the experimental Store is on.
  import { sectionsFor, type Section } from "../lib/bpstore";
  import { lib } from "../lib/store.svelte";
  import Glyph from "./Glyph.svelte";

  let { current, onpick }: { current: Section | null; onpick: (s: Section) => void } = $props();
</script>

<nav class="sections" aria-label="Sections">
  <Glyph button="lb" size={30} />
  {#each sectionsFor(!!lib.settings?.experimentalStore) as s (s.id)}
    <button type="button" class="tab" class:on={s.id === current} aria-current={s.id === current ? "page" : undefined} tabindex="-1" onclick={() => onpick(s.id)}>{s.label}</button>
  {/each}
  <Glyph button="rb" size={30} />
</nav>

<style>
  .sections {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
  }
  .sections > :global(:first-child) {
    margin-right: 8px;
  }
  .sections > :global(:last-child) {
    margin-left: 8px;
  }
  .tab {
    height: 46px;
    padding: 0 22px;
    border: 0;
    border-radius: 23px;
    background: transparent;
    color: rgba(243, 245, 247, 0.62);
    font-size: 21px;
    font-weight: 700;
    transition:
      background 0.2s,
      color 0.2s;
  }
  .tab.on {
    background: rgba(243, 245, 247, 0.16);
    color: #f3f5f7;
  }
</style>
