<script lang="ts">
  // One download: progress, what the safety checks found, and what can be
  // done with it now.
  import DownloadLanguages from "./DownloadLanguages.svelte";
  import Icon, { type IconName } from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { progress, statusLine } from "../lib/downloads";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import type { Download, DownloadAction, EngineStatus, SafetyLevel } from "../lib/types";

  let { d, eng, sandbox }: { d: Download; eng: EngineStatus | null; sandbox: boolean } = $props();

  // An open confirmation: removing, uninstalling or installing despite a block.
  let confirm = $state<"remove" | "uninstall" | "allow" | null>(null);
  let typed = $state("");
  let details = $state(false);
  let languagesOpen = $state(false);

  async function act(action: DownloadAction, deleteFiles = false) {
    confirm = null;
    await lib.run(() => api.store.action(d.id, action, deleteFiles));
    if (action === "remove") shop.downloads = shop.downloads.filter((x) => x.id !== d.id);
  }
  async function allow() {
    const ok = await lib.run(() => api.store.allowDownload(d.id, typed).then(() => true));
    if (ok) {
      confirm = null;
      typed = "";
    }
  }

  const moving = $derived(d.state === "queued" || d.state === "downloading" || d.state === "paused");
  const working = $derived(d.state === "scanning" || d.state === "installing");
  const verdict = $derived(d.safety ? (d.safety.overridden ? "overridden" : d.safety.verdict) : null);
  const badge: Record<string, [string, IconName]> = {
    clean: ["Checked", "check"],
    warn: ["Warnings", "warn"],
    block: ["Blocked", "warn"],
    overridden: ["Installed anyway", "warn"],
  };
  const levelIcon: Record<SafetyLevel, IconName> = { ok: "check", info: "info", warn: "warn", block: "warn" };
</script>

<li class="item" class:failed={d.state === "failed" || d.state === "blocked"}>
  <div class="row">
    <div class="info">
      <span class="title">{d.title}</span>
      <span class="status">{statusLine(d, eng)}</span>
      {#if moving}
        <div class="bar" role="progressbar" aria-label={`${d.title} progress`} aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(progress(d) * 100)}>
          <span style:width={`${progress(d) * 100}%`} class:paused={d.state !== "downloading"}></span>
        </div>
      {:else if working}
        <div class="bar busy" role="progressbar" aria-label={d.state === "scanning" ? "Checking" : "Installing"}><span></span></div>
      {/if}
    </div>

    {#if verdict}
      <button type="button" class="verdict {verdict}" aria-expanded={details} onclick={() => (details = !details)}>
        <Icon name={badge[verdict][1]} size={14} stroke={2.4} />{d.safety?.payloadSkipped ? "Scan skipped" : badge[verdict][0]}
      </button>
    {/if}

    {#if confirm === "remove"}
      <div class="confirm">
        <span>{d.state === "installed" ? "Forget this download? The game stays." : "Remove it?"}</span>
        <button type="button" class="btn" onclick={() => act("remove", false)}>Keep files</button>
        <button type="button" class="btn danger" onclick={() => act("remove", true)}>Delete files</button>
        <button type="button" class="icon" aria-label="Don't remove" onclick={() => (confirm = null)}><Icon name="close" size={16} /></button>
      </div>
    {:else if confirm === "uninstall"}
      <div class="confirm">
        <span>Uninstall {d.title}? Its saves may go with it.</span>
        <button type="button" class="btn danger" onclick={() => act("uninstall")}>Uninstall</button>
        <button type="button" class="icon" aria-label="Don't uninstall" onclick={() => (confirm = null)}><Icon name="close" size={16} /></button>
      </div>
    {:else}
      <div class="actions">
        {#if !working && d.state !== "installed"}
          <button type="button" class="btn" onclick={() => languagesOpen = true}>Languages</button>
        {/if}
        {#if d.state === "downloaded" && d.safety}
          <button type="button" class="btn primary" onclick={() => act("install")}><Icon name="download" size={16} />Install</button>
        {:else if d.state === "blocked"}
          <button type="button" class="btn" onclick={() => ((confirm = "allow"), (details = true))}>Install anyway…</button>
        {:else if d.state === "installed"}
          <button type="button" class="btn" onclick={() => (confirm = "uninstall")}>Uninstall</button>
        {/if}
        {#if d.state === "queued" || d.state === "downloading"}
          <button type="button" class="icon" aria-label={`Pause ${d.title}`} title="Pause" onclick={() => act("pause")}><Icon name="stop" size={14} /></button>
        {:else if d.state === "paused" || d.state === "failed"}
          <button type="button" class="icon" aria-label={`${d.state === "failed" ? "Try again" : "Resume"}: ${d.title}`} title={d.state === "failed" ? "Try again" : "Resume"} onclick={() => act("resume")}
            ><Icon name={d.state === "failed" ? "refresh" : "play"} size={16} /></button
          >
        {/if}
        {#if d.state === "downloaded" || d.state === "blocked"}
          <button type="button" class="icon" aria-label={`Check ${d.title} again`} title="Run the safety checks again" onclick={() => act("recheck")}><Icon name="refresh" size={16} /></button>
          {#if sandbox}
            <button type="button" class="icon" aria-label={`Open ${d.title} in Windows Sandbox`} title="Try it in Windows Sandbox first" onclick={() => lib.run(() => api.store.openInSandbox(d.id))}><Icon name="lock" size={16} /></button>
          {/if}
        {/if}
        <button type="button" class="icon" aria-label={`Show ${d.title} in Explorer`} title="Show in Explorer" onclick={() => lib.run(() => api.store.showDownload(d.id))}><Icon name="folder" size={16} /></button>
        {#if !working}
          <button type="button" class="icon" aria-label={`Remove ${d.title}`} title="Remove" onclick={() => (confirm = "remove")}><Icon name="trash" size={16} /></button>
        {/if}
      </div>
    {/if}
  </div>

  {#if details && d.safety}
    <ul class="findings">
      {#each d.safety.findings as f, i (i)}
        <li class={f.level}><Icon name={levelIcon[f.level]} size={14} stroke={2.2} /><span>{f.text}</span></li>
      {/each}
      {#if d.safety.sha256}<li class="info hash"><span>SHA-256 of {d.safety.main}: {d.safety.sha256}</span></li>{/if}
    </ul>
    {#if confirm === "allow"}
      <form
        class="allow"
        onsubmit={(e) => {
          e.preventDefault();
          allow();
        }}
      >
        <label for={`allow-${d.id}`}>To install it anyway, type <strong>{d.title}</strong>:</label>
        <input id={`allow-${d.id}`} type="text" autocomplete="off" spellcheck="false" bind:value={typed} />
        <button type="submit" class="btn danger" disabled={typed.trim().toLowerCase() !== d.title.trim().toLowerCase()}>Allow installing</button>
        <button type="button" class="btn" onclick={() => (confirm = null)}>Cancel</button>
      </form>
    {/if}
  {/if}
</li>
{#if languagesOpen}<DownloadLanguages download={d} onclose={() => languagesOpen = false} />{/if}

<style>
  .item {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 14px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .title {
    font-weight: 700;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .status {
    font-size: 13.5px;
    color: var(--muted);
  }
  .item.failed .status {
    color: var(--warn);
  }
  .bar {
    height: 6px;
    margin-top: 4px;
    border-radius: 99px;
    background: var(--surface-3);
    overflow: hidden;
  }
  .bar span {
    display: block;
    height: 100%;
    border-radius: inherit;
    background: var(--accent);
    transition: width 0.6s var(--ease);
  }
  .bar span.paused {
    background: var(--muted);
  }
  .bar.busy span {
    width: 30%;
    animation: slide 1.4s ease-in-out infinite;
  }
  @keyframes slide {
    from {
      transform: translateX(-100%);
    }
    to {
      transform: translateX(340%);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .bar.busy span {
      animation: none;
      width: 100%;
      opacity: 0.5;
    }
  }
  .verdict {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 5px;
    height: 28px;
    padding: 0 10px;
    border: 0;
    border-radius: 999px;
    font-size: 12.5px;
    font-weight: 700;
    background: var(--surface-3);
    color: var(--text-2);
  }
  .verdict.clean {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .verdict.warn,
  .verdict.overridden {
    color: var(--warn);
  }
  .verdict.block {
    color: var(--danger);
  }
  .actions,
  .confirm {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .confirm {
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }
  .confirm span {
    font-size: 14px;
    color: var(--text-2);
  }
  .btn {
    flex-shrink: 0;
    height: 36px;
    padding: 0 14px;
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: transparent;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
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
  .btn.primary:hover {
    background: var(--accent);
    filter: brightness(1.08);
  }
  .btn.danger {
    color: var(--danger);
  }
  .btn:disabled {
    opacity: 0.5;
  }
  .icon {
    width: 34px;
    height: 34px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .icon:hover {
    background: var(--surface-3);
    color: var(--text);
  }
  .findings {
    list-style: none;
    margin: 0;
    padding: 10px 12px;
    border-radius: 10px;
    background: var(--surface-3);
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 13.5px;
    color: var(--text-2);
  }
  .findings li {
    display: flex;
    gap: 8px;
    align-items: flex-start;
  }
  .findings li :global(svg) {
    flex-shrink: 0;
    margin-top: 2px;
  }
  .findings .ok :global(svg) {
    color: var(--accent-text);
  }
  .findings .warn :global(svg) {
    color: var(--warn);
  }
  .findings .block {
    color: var(--danger);
  }
  .findings .info :global(svg) {
    color: var(--muted);
  }
  .hash {
    font-size: 12px;
    color: var(--muted);
    overflow-wrap: anywhere;
  }
  .allow {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
    font-size: 14px;
  }
  .allow label {
    width: 100%;
    color: var(--text-2);
  }
  .allow input {
    flex: 1;
    min-width: 160px;
    height: 36px;
    padding: 0 10px;
    border-radius: 8px;
    border: 1px solid var(--line-strong);
    background: var(--surface);
    color: var(--text);
  }
</style>
