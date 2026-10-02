<script lang="ts">
  // What discovery is doing, in one line that opens to a line per source.
  import Icon from "../../components/Icon.svelte";
  import { sourceLine, statusSummary } from "../../lib/storefront";
  import { storefront } from "../../lib/storefront.svelte";

  const st = $derived(storefront.status);
  const sum = $derived(statusSummary(st));
  const sources = $derived((st?.sources ?? []).filter((s) => s.enabled));
  let open = $state(false);
</script>

{#if st?.enabled && sources.length}
  <div class="bar">
    <button type="button" class="line" aria-expanded={open} aria-controls="sf-status" onclick={() => (open = !open)}>
      <span class="dot {sum.tone}" aria-hidden="true"></span>
      <span class="text">{sum.text}</span>
      <span class="count">{st.games.toLocaleString("en")} {st.games === 1 ? "game" : "games"} indexed</span>
      <span class="chev" class:open aria-hidden="true"><Icon name="chevronDown" size={14} stroke={2.2} /></span>
    </button>
    {#if open}
      <ul id="sf-status" class="list">
        {#each sources as s (s.id)}
          <li>
            <span class="name">{s.name}</span>
            <span class="sf-muted">{sourceLine(s)}</span>
            {#if s.error}<span class="err">{s.error}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .bar {
    padding: 0 28px 4px;
  }
  .line {
    display: flex;
    align-items: center;
    gap: 8px;
    max-width: 100%;
    min-height: 30px;
    padding: 0 8px;
    margin-left: -8px;
    border: 0;
    border-radius: 8px;
    background: none;
    color: var(--muted);
    font-size: 13.5px;
  }
  .line:hover {
    background: var(--surface-2);
  }
  .text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .count::before {
    content: "·";
    margin-right: 8px;
  }
  .dot {
    width: 8px;
    height: 8px;
    flex-shrink: 0;
    border-radius: 50%;
    background: var(--muted);
  }
  .dot.ok {
    background: var(--accent);
  }
  .dot.busy {
    background: var(--accent);
    animation: pulse 1.4s ease-in-out infinite;
  }
  .dot.warn {
    background: var(--warn);
  }
  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  .chev {
    display: flex;
    transition: transform 0.15s var(--ease);
  }
  .chev.open {
    transform: rotate(180deg);
  }
  .list {
    list-style: none;
    margin: 4px 0 6px;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .list li {
    display: flex;
    flex-wrap: wrap;
    gap: 2px 12px;
    padding: 8px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
    font-size: 13.5px;
  }
  .name {
    min-width: 70px;
    font-weight: 700;
  }
  .err {
    flex-basis: 100%;
    color: var(--warn);
    overflow-wrap: anywhere;
  }
  @media (max-width: 700px) {
    .bar {
      padding: 0 16px 4px;
    }
  }
</style>
