<script lang="ts">
  // Settings → Experimental, with the store on: where downloads go, the
  // download engine and how it connects.
  import Icon from "../components/Icon.svelte";
  import Toggle from "../components/Toggle.svelte";
  import { api } from "../lib/api";
  import { feedLine } from "../lib/catalog";
  import { shop } from "../lib/shop.svelte";
  import { lib } from "../lib/store.svelte";
  import type { FeedInfo, Settings, StoreSettings, TorrentInterface, TorrentNetwork } from "../lib/types";

  const s = $derived(lib.settings);
  const st = $derived(s?.store);
  const net = $derived(s?.store.network);
  const setStore = (patch: Partial<StoreSettings>) => s && lib.saveSettings({ ...s, store: { ...s.store, ...patch } });
  const setNet = (patch: Partial<TorrentNetwork>) => s && lib.saveSettings({ ...s, store: { ...s.store, network: { ...s.store.network, ...patch } } });

  let folder = $state("");
  let gamesFolder = $state("");
  let hasPassword = $state(false);
  let hasVT = $state(false);
  let vtDraft = $state("");
  $effect(() => {
    api.store.hasVirusTotalKey().then((v) => (hasVT = v));
  });
  async function saveVT(k: string) {
    if ((await lib.run(() => api.store.setVirusTotalKey(k).then(() => true))) === true) {
      hasVT = !!k;
      vtDraft = "";
    }
  }
  $effect(() => {
    shop.start();
    void st?.downloads;
    void st?.games;
    api.store.downloadsFolder().then((f) => (folder = f));
    api.store.gamesFolder().then((f) => (gamesFolder = f));
  });
  $effect(() => {
    api.store.hasProxyPassword().then((v) => (hasPassword = v));
  });

  async function apply(fn: () => Promise<Settings>) {
    const next = await lib.run(fn);
    if (next) lib.settings = next;
    return !!next;
  }

  let feeds = $state<FeedInfo[]>([]);
  const loadFeeds = () => api.store.feeds().then((f) => (feeds = f));
  $effect(() => {
    void loadFeeds();
  });
  let feedURL = $state("");
  let feedBusy = $state(false);
  async function addFeed(e: SubmitEvent) {
    e.preventDefault();
    if (!feedURL.trim() || feedBusy) return;
    feedBusy = true;
    if (await apply(() => api.store.addFeed(feedURL.trim()))) feedURL = "";
    feedBusy = false;
    await loadFeeds();
  }
  async function feedChange(fn: () => Promise<Settings>) {
    await apply(fn);
    await loadFeeds();
  }
  let refreshing = $state(false);
  async function refreshFeeds() {
    refreshing = true;
    const f = await lib.run(() => api.store.refreshFeeds());
    if (f) feeds = f;
    refreshing = false;
  }

  // Interfaces come from qBittorrent, so listing them starts it.
  let interfaces = $state<TorrentInterface[] | null>(null);
  let addresses = $state<string[]>([]);
  let loading = $state(false);
  async function loadInterfaces() {
    loading = true;
    interfaces = (await lib.run(() => api.store.interfaces())) ?? null;
    loading = false;
  }
  $effect(() => {
    const iface = net?.interface;
    if (!iface || !interfaces) {
      addresses = [];
      return;
    }
    api.store.addresses(iface).then((a) => (addresses = a)).catch(() => (addresses = []));
  });
  // The bound interface's name, also before the list was loaded.
  const ifaceLabel = $derived(interfaces?.find((i) => i.id === net?.interface)?.name ?? net?.interface ?? "");

  let password = $state("");
  async function savePassword(p: string) {
    if ((await lib.run(() => api.store.setProxyPassword(p).then(() => true))) === true) {
      hasPassword = !!p;
      password = "";
    }
  }

  // A number field saves when it's left, kept in range.
  function num(e: Event, lo: number, hi: number, fn: (n: number) => void) {
    const el = e.currentTarget as HTMLInputElement;
    const n = Math.round(Number(el.value));
    fn(Number.isFinite(n) ? Math.min(hi, Math.max(lo, n)) : lo);
  }

  const seedChoices: [number, string][] = [
    [0, "Stop when done"],
    [1, "Until 1×"],
    [2, "Until 2×"],
    [-1, "Keep sharing"],
  ];
</script>

{#if s && st && net}
  <div class="group">
    <span class="glabel">Catalog feeds</span>
    <p class="hint">The store shows the games in feeds you add: web addresses of catalogs in Seaglass's feed format. Seaglass comes with none. Only add feeds you trust, for games you're allowed to download.</p>
    {#if feeds.length}
      <ul class="feeds">
        {#each feeds as f (f.url)}
          <li class:off={!f.enabled}>
            <div class="text">
              <span class="t">{f.name || f.url}</span>
              {#if f.name}<span class="url">{f.url}</span>{/if}
              <span class="d" class:err={!!f.error}>{feedLine(f)}</span>
            </div>
            <button type="button" class="btn" aria-pressed={f.enabled} onclick={() => feedChange(() => api.store.setFeedEnabled(f.url, !f.enabled))}>{f.enabled ? "Turn off" : "Turn on"}</button>
            <button type="button" class="icon" aria-label={`Remove ${f.name || f.url}`} title="Remove" onclick={() => feedChange(() => api.store.removeFeed(f.url))}><Icon name="trash" size={16} /></button>
          </li>
        {/each}
      </ul>
    {/if}
    <form class="row" onsubmit={addFeed}>
      <label class="sr-only" for="st-feed">Feed address</label>
      <input id="st-feed" class="text-in" type="url" autocomplete="off" spellcheck="false" placeholder="https://…/feed.json" bind:value={feedURL} />
      <button type="submit" class="btn" disabled={!feedURL.trim() || feedBusy}><Icon name="plus" size={16} />{feedBusy ? "Checking the feed…" : "Add feed"}</button>
      {#if feeds.length}
        <button type="button" class="btn" disabled={refreshing} onclick={refreshFeeds}><Icon name="refresh" size={16} />{refreshing ? "Fetching…" : "Fetch now"}</button>
      {/if}
    </form>
  </div>

  <div class="group">
    <span class="glabel">Downloads</span>
    <div class="pathrow">
      <Icon name="folder" size={16} /><span class="path">{folder}</span>
      <button type="button" class="btn" onclick={() => apply(() => api.store.chooseDownloadsFolder())}>Change</button>
    </div>
    <div class="pathrow">
      <Icon name="pad" size={16} /><span class="path" title="Where games are installed">{gamesFolder}</span>
      <button type="button" class="btn" onclick={() => apply(() => api.store.chooseGamesFolder())}>Change</button>
    </div>
    <Toggle checked={st.keepDownloads} title="Keep downloads after installing" detail="Keeps sharing them, and lets you install again without downloading. Off: a download is deleted once its game is installed." onchange={(v) => setStore({ keepDownloads: v })} />
    <Toggle checked={st.pauseWhilePlaying} title="Pause downloads while playing" detail="Games get the whole connection and disk. Downloads go on when the game closes." onchange={(v) => setStore({ pauseWhilePlaying: v })} />
  </div>

  <div class="group">
    <span class="glabel">Safety checks</span>
    <p class="hint">Before anything in a download runs, Seaglass compares the installer with the feed's checksum, looks at what's inside, and has Microsoft Defender scan it. These checks catch known problems; they can't prove a download is safe.</p>
    <Toggle checked={st.blockDetections} title="Block what Defender or VirusTotal flags" detail="Blocked downloads aren't installed unless you type their name to insist. Off: detections are warnings you decide on." onchange={(v) => setStore({ blockDetections: v })} />
    {#if hasVT}
      <div class="pathrow">
        <Icon name="check" size={16} /><span class="path">VirusTotal key saved: installers are looked up by their SHA-256</span>
        <button type="button" class="btn" onclick={() => saveVT("")}>Remove</button>
      </div>
    {:else}
      <form
        class="field"
        onsubmit={(e) => {
          e.preventDefault();
          if (vtDraft.trim()) saveVT(vtDraft.trim());
        }}
      >
        <label class="label" for="st-vt">VirusTotal key</label>
        <input id="st-vt" class="text-in" type="password" autocomplete="off" spellcheck="false" placeholder="Optional: your own API key" bind:value={vtDraft} />
        <button type="submit" class="btn" disabled={!vtDraft.trim()}>Save</button>
      </form>
      <p class="hint">With your free VirusTotal key, installers are looked up by their SHA-256 too. Files are never uploaded. The key is stored encrypted for your Windows account.</p>
    {/if}
  </div>

  <div class="group">
    <span class="glabel">Download engine</span>
    <div class="card" class:ok={shop.engine?.running} class:warn={shop.engine && !shop.engine.installed}>
      <span class="dot" aria-hidden="true"></span>
      <div class="text">
        <span class="t">
          {#if !shop.engine}Checking…{:else if !shop.engine.installed}qBittorrent isn't installed{:else if shop.engine.running}qBittorrent {shop.engine.version ?? ""} is running{:else}qBittorrent starts when there's something to download{/if}
        </span>
        <span class="d">{shop.engine?.error || shop.engine?.exe || "Seaglass runs qBittorrent hidden, with settings and torrents of its own."}</span>
      </div>
    </div>
    <div class="row">
      {#if shop.engine && !shop.engine.installed}
        <button type="button" class="btn" onclick={() => lib.run(() => api.store.getQBittorrent())}><Icon name="link" size={16} />Get qBittorrent</button>
      {/if}
      <button type="button" class="btn" onclick={() => apply(() => api.store.chooseQBittorrent())}>Choose qbittorrent.exe</button>
      {#if st.qbittorrent}
        <button type="button" class="btn" onclick={() => setStore({ qbittorrent: "" })}>Use the installed one</button>
      {/if}
    </div>
    <p class="hint">qBittorrent is a file sharing program: what you download is shared with others while it runs. What you share is your responsibility.</p>
  </div>

  <div class="group">
    <span class="glabel">Network</span>
    <div class="field">
      <span class="label">Network interface</span>
      {#if interfaces}
        <select aria-label="Network interface" value={net.interface} onchange={(e) => setNet({ interface: e.currentTarget.value, address: "" })}>
          <option value="">Any interface</option>
          {#each interfaces as i (i.id)}<option value={i.id}>{i.name}</option>{/each}
          {#if net.interface && !interfaces.some((i) => i.id === net.interface)}<option value={net.interface}>{net.interface} (not connected)</option>{/if}
        </select>
      {:else}
        <span class="value">{net.interface ? ifaceLabel : "Any interface"}</span>
        <button type="button" class="btn" disabled={loading} onclick={loadInterfaces}>{loading ? "Starting qBittorrent…" : "Choose"}</button>
      {/if}
    </div>
    {#if net.interface && addresses.length}
      <div class="field">
        <span class="label">Address</span>
        <select aria-label="Address" value={net.address} onchange={(e) => setNet({ address: e.currentTarget.value })}>
          <option value="">All of its addresses</option>
          {#each addresses as a (a)}<option value={a}>{a}</option>{/each}
        </select>
      </div>
    {/if}
    <p class="hint">Bound to an interface (a VPN, say), nothing is downloaded or shared while it's disconnected.</p>
    <div class="field">
      <label class="label" for="st-port">Incoming port</label>
      <input id="st-port" class="num" type="number" min="0" max="65535" value={net.port} onchange={(e) => num(e, 0, 65535, (n) => setNet({ port: n }))} />
      <span class="unit">{net.port ? "" : "0 picks a random port"}</span>
    </div>
    <Toggle checked={net.upnp} title="Forward the port on the router" detail="With UPnP or NAT-PMP, so other people can connect to you. Faster downloads; off if your router or VPN forwards it." onchange={(v) => setNet({ upnp: v })} />
  </div>

  <div class="group">
    <span class="glabel">Proxy</span>
    <div class="seg" role="group" aria-label="Proxy">
      {#each [["none", "None"], ["socks5", "SOCKS5"], ["http", "HTTP"]] as [id, label] (id)}
        <button type="button" class:on={net.proxy === id} aria-pressed={net.proxy === id} onclick={() => setNet({ proxy: id as TorrentNetwork["proxy"] })}>{label}</button>
      {/each}
    </div>
    {#if net.proxy !== "none"}
      <div class="field">
        <label class="label" for="st-ph">Host</label>
        <input id="st-ph" class="text-in" type="text" spellcheck="false" value={net.proxyHost} onchange={(e) => setNet({ proxyHost: e.currentTarget.value.trim() })} />
        <label class="label short" for="st-pp">Port</label>
        <input id="st-pp" class="num" type="number" min="1" max="65535" value={net.proxyPort} onchange={(e) => num(e, 1, 65535, (n) => setNet({ proxyPort: n }))} />
      </div>
      <div class="field">
        <label class="label" for="st-pu">User name</label>
        <input id="st-pu" class="text-in" type="text" autocomplete="off" spellcheck="false" placeholder="None" value={net.proxyUser} onchange={(e) => setNet({ proxyUser: e.currentTarget.value.trim() })} />
      </div>
      {#if net.proxyUser}
        <form
          class="field"
          onsubmit={(e) => {
            e.preventDefault();
            if (password) savePassword(password);
          }}
        >
          <label class="label" for="st-pw">Password</label>
          {#if hasPassword}
            <span class="value">Saved, encrypted for your Windows account</span>
            <button type="button" class="btn" onclick={() => savePassword("")}>Remove</button>
          {:else}
            <input id="st-pw" class="text-in" type="password" autocomplete="off" bind:value={password} />
            <button type="submit" class="btn" disabled={!password}>Save</button>
          {/if}
        </form>
      {/if}
      <Toggle checked={net.proxyPeers} title="Connect to peers through the proxy too" detail="Off: only trackers go through it." onchange={(v) => setNet({ proxyPeers: v })} />
    {/if}
  </div>

  <div class="group">
    <span class="glabel">Privacy and peers</span>
    <div class="seg" role="group" aria-label="Encryption">
      {#each [["prefer", "Encrypt when possible"], ["require", "Always encrypt"], ["off", "Don't encrypt"]] as [id, label] (id)}
        <button type="button" class:on={net.encryption === id} aria-pressed={net.encryption === id} onclick={() => setNet({ encryption: id as TorrentNetwork["encryption"] })}>{label}</button>
      {/each}
    </div>
    <Toggle checked={net.dht} title="DHT" detail="Find peers without a tracker. Needed for most magnet links." onchange={(v) => setNet({ dht: v })} />
    <Toggle checked={net.pex} title="Peer exchange" detail="Peers tell each other about more peers." onchange={(v) => setNet({ pex: v })} />
    <Toggle checked={net.lsd} title="Local peer discovery" detail="Find peers on your own network." onchange={(v) => setNet({ lsd: v })} />
    <Toggle checked={net.anonymous} title="Anonymous mode" detail="Don't tell peers and trackers which program this is, or your port. Some trackers refuse it." onchange={(v) => setNet({ anonymous: v })} />
  </div>

  <div class="group">
    <span class="glabel">Speed</span>
    <div class="field">
      <label class="label" for="st-dl">Download limit</label>
      <input id="st-dl" class="num" type="number" min="0" value={net.downLimit} onchange={(e) => num(e, 0, 10_000_000, (n) => setNet({ downLimit: n }))} />
      <span class="unit">KiB/s, 0 for none</span>
    </div>
    <div class="field">
      <label class="label" for="st-ul">Upload limit</label>
      <input id="st-ul" class="num" type="number" min="0" value={net.upLimit} onchange={(e) => num(e, 0, 10_000_000, (n) => setNet({ upLimit: n }))} />
      <span class="unit">KiB/s, 0 for none</span>
    </div>
    <div class="field">
      <label class="label" for="st-ma">Downloads at once</label>
      <input id="st-ma" class="num" type="number" min="1" max="10" value={net.maxActive} onchange={(e) => num(e, 1, 10, (n) => setNet({ maxActive: n }))} />
    </div>
    <span class="label">After a download</span>
    <div class="seg" role="group" aria-label="Sharing after a download">
      {#each seedChoices as [ratio, label] (ratio)}
        <button type="button" class:on={net.seedRatio === ratio} aria-pressed={net.seedRatio === ratio} onclick={() => setNet({ seedRatio: ratio })}>{label}</button>
      {/each}
    </div>
  </div>
{/if}

<style>
  .group {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .group + .group {
    margin-top: 6px;
  }
  .glabel {
    font-size: 14px;
    font-weight: 700;
    color: var(--text-2);
  }
  .hint {
    margin: -2px 0 2px;
    font-size: 13.5px;
    color: var(--muted);
  }
  .pathrow,
  .field {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    min-height: 44px;
    padding: 4px 6px 4px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 14px;
  }
  .path,
  .value {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .label {
    min-width: 150px;
    font-size: 14px;
    font-weight: 600;
    color: var(--text);
  }
  .group > .label {
    min-width: 0;
  }
  .label.short {
    min-width: 0;
  }
  .unit {
    color: var(--muted);
    font-size: 13px;
  }
  select,
  .num,
  .text-in {
    height: 36px;
    padding: 0 10px;
    border-radius: 8px;
    border: 1px solid var(--line-strong);
    background: var(--surface);
    color: var(--text);
    font-size: 14px;
    outline: none;
  }
  select {
    flex: 1;
    min-width: 0;
  }
  .num {
    width: 110px;
  }
  .text-in {
    flex: 1;
    min-width: 120px;
  }
  select:focus,
  .num:focus,
  .text-in:focus {
    border-color: var(--accent);
  }
  .feeds {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .feeds li {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 8px 10px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .feeds li.off .text {
    opacity: 0.55;
  }
  .feeds .text {
    flex: 1;
  }
  .url {
    font-size: 12.5px;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .d.err {
    color: var(--warn);
  }
  .icon {
    width: 34px;
    height: 34px;
    flex-shrink: 0;
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
  .card {
    display: flex;
    gap: 14px;
    align-items: center;
    padding: 14px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .dot {
    width: 10px;
    height: 10px;
    flex-shrink: 0;
    border-radius: 50%;
    background: var(--muted);
  }
  .card.ok .dot {
    background: var(--accent);
  }
  .card.warn .dot {
    background: var(--warn);
  }
  .text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .t {
    font-weight: 700;
  }
  .d {
    font-size: 13.5px;
    color: var(--muted);
    overflow-wrap: anywhere;
  }
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
  }
  .btn {
    width: fit-content;
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
  .btn:disabled {
    opacity: 0.6;
  }
  .seg {
    display: flex;
    flex-wrap: wrap;
    gap: 2px;
    padding: 3px;
    width: fit-content;
    border-radius: 10px;
    background: var(--surface-2);
  }
  .seg button {
    height: 34px;
    padding: 0 14px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    font-size: 14px;
    font-weight: 700;
  }
  .seg button.on {
    background: var(--surface-3);
    color: var(--text);
  }
</style>
