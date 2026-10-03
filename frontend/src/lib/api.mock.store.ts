// The experimental store for `npm run dev:mock`: a pretend engine whose
// downloads move along on their own.
import type { Api } from "./api";
import { mockDiscovery, mockDiscoveryArt } from "./api.mock.discovery";
import type { CatalogEntry, CatalogOffer, Download, EngineStatus, FeedInfo, Settings, SourceRelease, StoreArt, StoreSettings } from "./types";

export const mockStoreSettings: StoreSettings = {
  qbittorrent: "",
  downloads: "",
  games: "",
  keepDownloads: false,
  pauseWhilePlaying: true,
  disablePayloadScanning: false,
  privateSources: true,
  sources: ["fitgirl", "dodi"],
  sourceSetup: "done",
  indexingPaused: false,
  blockDetections: true,
  language: "English",
  network: {
    interface: "",
    address: "",
    port: 0,
    upnp: true,
    proxy: "none",
    proxyHost: "",
    proxyPort: 1080,
    proxyUser: "",
    proxyPeers: false,
    encryption: "prefer",
    dht: true,
    pex: true,
    lsd: true,
    anonymous: false,
    downLimit: 0,
    upLimit: 0,
    maxActive: 2,
    seedRatio: 1,
  },
  feeds: [
    { url: "https://feeds.example/indie.json", enabled: true, trust: 0 },
    { url: "https://feeds.example/freeware.json", enabled: true, trust: 0 },
    { url: "https://old.example/feed.json", enabled: false, trust: 0 },
  ],
};

const now = () => Math.floor(Date.now() / 1000);
const gb = 1 << 30;
const dl = (p: Partial<Download> & { id: string; title: string }): Download => ({
  source: "magnet:?xt=urn:btih:0000",
  savePath: "C:\\Users\\you\\Downloads\\Seaglass",
  state: "downloading",
  size: 0,
  done: 0,
  downSpeed: 0,
  upSpeed: 0,
  seeds: 0,
  peers: 0,
  eta: 0,
  seeding: false,
  created: now() - 600,
  ...p,
});

let downloads: Download[] = [
  dl({ id: "sg-1", title: "Big Buck Bunny", name: "Big Buck Bunny", size: 0.9 * gb, done: 0.35 * gb, downSpeed: 6.2e6, seeds: 24, peers: 5, eta: 95, engine: "downloading" }),
  dl({ id: "sg-2", title: "Sintel", state: "paused", size: 1.4 * gb, done: 0.2 * gb, engine: "paused" }),
  dl({ id: "sg-3", title: "Tears of Steel", state: "downloaded", size: 3.1 * gb, done: 3.1 * gb, seeding: true, upSpeed: 4e5, peers: 3, engine: "seeding", finished: now() - 300 }),
  dl({ id: "sg-4", title: "Elephants Dream", state: "failed", size: 0.8 * gb, done: 0.1 * gb, error: "Not enough free space: 700 MB more is needed, 120 MB is free." }),
  dl({
    id: "sg-5", title: "Brass Orchard Build 15302", gameKey: "title:brassorchard", state: "downloaded", size: 2.1 * gb, done: 2.1 * gb, installer: "nsis",
    installDir: "C:\\Users\\you\\Games\\Brass Orchard", finished: now() - 900,
    safety: { verdict: "warn", checked: now() - 800, main: "setup.exe", findings: [
      { check: "integrity", level: "info", text: "The feed gives no checksum, so the installer can't be compared with what it lists. The download itself was verified piece by piece." },
      { check: "signature", level: "info", text: "setup.exe isn't digitally signed, so its publisher can't be confirmed." },
      { check: "files", level: "ok", text: "412 files, nothing unusual among them." },
      { check: "defender", level: "ok", text: "Microsoft Defender found nothing." },
      { check: "virustotal", level: "warn", text: "1 of 72 VirusTotal engines flag setup.exe (often a false alarm with few engines)." },
    ] },
  }),
  dl({
    id: "sg-6", title: "Glass Meridian v0.9 beta", gameKey: "title:glassmeridian", state: "blocked", size: 3.4 * gb, done: 3.4 * gb, finished: now() - 4000,
    safety: { verdict: "block", checked: now() - 3900, main: "setup.exe", findings: [
      { check: "integrity", level: "block", text: "setup.exe isn't the file the feed lists: its SHA-256 differs. It was changed or replaced." },
      { check: "defender", level: "ok", text: "Microsoft Defender found nothing." },
    ] },
  }),
  dl({ id: "sg-7", title: "Cinder Drift 0.9", version: "0.9", gameKey: "title:cinderdrift", state: "installed", size: 0.4 * gb, done: 0.4 * gb, installer: "archive", installDir: "C:\\Users\\you\\Games\\Cinder Drift", installedAt: now() - 86400, safety: { verdict: "clean", checked: now() - 86500, findings: [] } }),
];
let engine: EngineStatus = { installed: true, exe: "C:\\Program Files\\qBittorrent\\qbittorrent.exe", running: true, version: "v5.1.4", interfaceMissing: false, gameRunning: false, held: false };
let proxyPassword = false;
let vtKey = false;
const listeners = new Set<(d: Download[]) => void>();
const engineListeners = new Set<(s: EngineStatus) => void>();
const copy = <T>(v: T): T => structuredClone(v);
const changed = () => listeners.forEach((cb) => cb(copy(downloads)));
const sourcePreviews = new Map<string, SourceRelease>();

if (typeof window !== "undefined") {
  setInterval(() => {
    downloads = downloads.map((d) => {
      if (d.state !== "downloading") return d;
      const done = Math.min(d.size, d.done + d.downSpeed);
      return done >= d.size
        ? { ...d, done, state: "downloaded", seeding: true, downSpeed: 0, upSpeed: 2e5, eta: 0, engine: "seeding", finished: now() }
        : { ...d, done, eta: Math.round((d.size - done) / d.downSpeed) };
    });
    changed();
  }, 1000);
}

const feedNames: Record<string, string> = {
  "https://feeds.example/indie.json": "Indie Showcase",
  "https://feeds.example/freeware.json": "Freeware Classics",
  "https://old.example/feed.json": "Old Mirror",
};
const feedErrors: Record<string, string> = { "https://old.example/feed.json": "couldn't reach the feed: no such host" };
const INDIE = "https://feeds.example/indie.json";
const FREE = "https://feeds.example/freeware.json";

const offer = (feedUrl: string, p: Partial<CatalogOffer> & { title: string }): CatalogOffer => ({
  magnet: "magnet:?xt=urn:btih:dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c",
  platform: "windows",
  feedUrl,
  feedName: feedNames[feedUrl],
  ...p,
});
const entry = (title: string, offers: CatalogOffer[], steamAppId?: number): CatalogEntry => ({
  key: steamAppId ? `steam:${steamAppId}` : `title:${title.toLowerCase().replace(/[^a-z0-9]/g, "")}`,
  title,
  steamAppId,
  offers,
  version: offers[0].version ?? "",
  updated: offers.map((o) => o.buildDate ?? "").sort().at(-1) ?? "",
  size: offers[0].sizeBytes ?? 0,
  languages: [...new Set(offers.flatMap((o) => o.languages ?? []))],
});
const catalogEntries: CatalogEntry[] = [
  entry("Ashen Lanterns", [
    offer(INDIE, { title: "Ashen Lanterns", version: "v1.4.2", buildDate: "2026-09-12", sizeBytes: 6.2 * gb, installerType: "inno", languages: ["English", "German", "French", "Japanese"] }),
    offer(FREE, { title: "Ashen Lanterns", version: "v1.3", buildDate: "2026-06-01", sizeBytes: 5.9 * gb, languages: ["English"] }),
  ]),
  entry("Brass Orchard", [offer(INDIE, { title: "Brass Orchard", version: "Build 15302", buildDate: "2026-08-30", sizeBytes: 2.1 * gb, installerType: "nsis", languages: ["English", "Spanish"] })]),
  entry("Cinder Drift", [offer(FREE, { title: "Cinder Drift", version: "1.0", buildDate: "2025-12-24", sizeBytes: 0.4 * gb, installerType: "archive", languages: ["English"] })]),
  entry("Dune Lark", [
    offer(INDIE, { title: "Dune Lark", version: "v2.0.1", buildDate: "2026-09-28", sizeBytes: 11.8 * gb, installerType: "inno", languages: ["English", "German", "Polish", "Russian", "Simplified Chinese"], notes: "Includes the Tidewater expansion." }),
  ]),
  entry("Ember Crown", [offer(INDIE, { title: "Ember Crown", version: "v1.0.6", buildDate: "2026-07-03", sizeBytes: 38 * gb, installerType: "inno", languages: ["English", "French", "German", "Italian", "Spanish"] })], 1245620),
  entry("Fable of the Fen", [offer(FREE, { title: "Fable of the Fen", buildDate: "2024-03-10", sizeBytes: 0.15 * gb, installerType: "portable", languages: ["English"] })]),
  entry("Glass Meridian", [offer(INDIE, { title: "Glass Meridian", version: "v0.9 beta", buildDate: "2026-09-30", sizeBytes: 3.4 * gb, installerType: "msi", languages: ["English", "Japanese"] })]),
  entry("Harbor Nine", [offer(FREE, { title: "Harbor Nine", version: "v3.1", buildDate: "2026-02-14", sizeBytes: 1.2 * gb, installerType: "nsis", languages: ["English", "German"] })]),
];
// Descriptions only: the mock has no pictures, so covers show the generated art.
const blurbs: Record<string, [string, string, string[]]> = {
  "Ashen Lanterns": ["Carry the last lit lantern through a city of ash, where every light you kindle wakes something else.", "Kiln & Co.", ["Adventure", "Puzzle"]],
  "Brass Orchard": ["Build clockwork trees and harvest gears in a cosy factory sim.", "Gearwright", ["Simulation", "Strategy"]],
  "Cinder Drift": ["A one-button racer down an erupting mountain.", "Small Fire", ["Racing", "Arcade"]],
  "Dune Lark": ["Glide over shifting dunes as a messenger bird, delivering letters between desert towns.", "Lark Studio", ["Adventure", "Exploration"]],
  "Ember Crown": ["A fallen knight climbs a burning mountain to take back a crown that was never theirs.", "Ashgrove", ["Action", "RPG"]],
  "Fable of the Fen": ["A short point-and-click tale about a frog who wants to see the sea.", "Fen Folk", ["Adventure"]],
  "Glass Meridian": ["Bend light through glass towers to restore a broken skyline.", "Prism Works", ["Puzzle"]],
  "Harbor Nine": ["Run a harbour at the edge of the world, one storm at a time.", "Ninefold", ["Management"]],
};

const squash = (t: string) => t.toLowerCase().replace(/[^a-z0-9]/g, "");

// What the backend adds to an entry: the version to get, and what's installed.
function annotate(e: CatalogEntry): CatalogEntry {
  const lang = mockStoreSettingsRef?.().store.language ?? "";
  let pick = 0;
  if (lang) {
    const i = e.offers.findIndex((o) => o.languages?.some((l) => l.toLowerCase() === lang.toLowerCase()));
    if (i > 0 && e.offers[0].languages?.length && !e.offers[0].languages?.some((l) => l.toLowerCase() === lang.toLowerCase())) pick = i;
  }
  const why = pick === 0 ? (e.offers.length > 1 ? ["The newest version", "Installs without the installer's questions"] : ["The only version offered"]) : [`Has ${lang}`, `Not the newest (${e.offers[0].version}): it doesn't have ${lang}`];
  const inst = downloads.find((d) => d.gameKey === e.key && d.state === "installed");
  const installed = inst ? { download: inst.id, version: inst.version ?? inst.title.split(" ").pop() ?? "", dir: inst.installDir ?? "", update: e.key === "title:cinderdrift" } : undefined;
  return { ...e, recommended: { offer: pick, why }, installed };
}
let mockStoreSettingsRef: (() => Settings) | undefined;

function feedInfo(url: string, enabled: boolean): FeedInfo {
  const items = catalogEntries.reduce((n, e) => n + e.offers.filter((o) => o.feedUrl === url).length, 0);
  return { url, enabled, name: feedNames[url] ?? "", items, skipped: url === FREE ? 2 : 0, fetched: feedErrors[url] ? 0 : now() - 1800, error: feedErrors[url] };
}

/** The store namespace; settings come and go through the mock's own settings. */
export function mockStore(getSettings: () => Settings, setSettings: (s: Settings) => void): Api["store"] {
  mockStoreSettingsRef = getSettings;
  const queue = (p: Partial<Download> & { title: string; gameKey: string }): Download => {
    const d = dl({ id: `sg-${Date.now()}`, name: p.title, size: 2 * gb, downSpeed: 3e7, seeds: 40, peers: 8, engine: "downloading", created: now(), ...p });
    downloads = [...downloads, d];
    changed();
    return copy(d);
  };
  return {
    ...mockDiscovery(getSettings, setSettings, queue),
    async engine() {
      return copy(engine);
    },
    async startEngine() {
      engine = { ...engine, running: true };
      engineListeners.forEach((cb) => cb(copy(engine)));
      return copy(engine);
    },
    async interfaces() {
      return [
        { id: "ethernet_32769", name: "Ethernet" },
        { id: "wireless_32768", name: "Wi-Fi" },
        { id: "iftype53_32768", name: "WireGuard Tunnel" },
      ];
    },
    async addresses(iface) {
      return iface === "iftype53_32768" ? ["10.8.0.2", "fd00::2"] : ["192.168.1.20"];
    },
    async hasProxyPassword() {
      return proxyPassword;
    },
    async setProxyPassword(p) {
      proxyPassword = !!p;
    },
    async chooseQBittorrent() {
      const s = getSettings();
      setSettings({ ...s, store: { ...s.store, qbittorrent: "D:\\Apps\\qBittorrent\\qbittorrent.exe" } });
      return copy(getSettings());
    },
    async getQBittorrent() {},
    async downloadsFolder() {
      return getSettings().store.downloads || "C:\\Users\\you\\Downloads\\Seaglass";
    },
    async chooseDownloadsFolder() {
      const s = getSettings();
      setSettings({ ...s, store: { ...s.store, downloads: "D:\\Downloads\\Games" } });
      return copy(getSettings());
    },
    async downloadLanguages() { return { game: ["English", "German", "French"], installer: [{ id: "english", name: "English" }, { id: "german", name: "German" }], torrent: true, note: "" }; },
    async setDownloadLanguages(id, language, setupLanguage, ask) {
      const index = downloads.findIndex((d) => d.id === id);
      if (index < 0) throw new Error("download not found");
      downloads[index] = { ...downloads[index], language, setupLanguage, askInstaller: ask, autoInstall: false };
      changed(); return copy(downloads[index]);
    },
    async discoverReleases(source, query) {
      if (!getSettings().store.privateSources) throw new Error("private catalog sources are disabled");
      const release: SourceRelease = { id: "mock-release", sourceId: source, title: query || "Ember Crown", rawTitle: `${query || "Ember Crown"} v1.2`, version: "v1.2", pageUrl: "https://example.com/release", releaseKind: "release", warnings: ["Confirm game identity before adding."], transports: [{ infoHash: "a".repeat(40), uri: "magnet:?xt=urn:btih:" + "a".repeat(40) }], references: [] };
      sourcePreviews.set(source,release);
      return { entries: [copy(release)] };
    },
    async attachReleaseTorrent() { throw new Error("Manual torrent selection is available in the desktop app."); },
    async openReleasePage() {},
    async reviewRelease(source, id, transport) {
      if (!getSettings().store.privateSources) throw new Error("private catalog sources are disabled");
      const release = sourcePreviews.get(source);
      if (!release || release.id !== id || !release.transports[transport]) throw new Error("preview no longer available");
      const url = source === "fitgirl" ? "https://fitgirl-repacks.site/feed/" : "https://dodi-repacks.site/";
      feedNames[url] = source === "fitgirl" ? "FitGirl" : "DODI";
      const added = offer(url, {title: release.title, version:release.version, magnet:release.transports[transport].uri});
      const existing = catalogEntries.find((e) => e.title === release.title);
      if (existing) { if (!existing.offers.some((o) => o.magnet === added.magnet)) existing.offers.unshift(added); return existing.key; }
      const game = entry(release.title,[added]); catalogEntries.push(game); return game.key;
    },
    async downloads() {
      return copy(downloads);
    },
    async addDownload(source, title) {
      if (!/^(magnet:\?|https?:\/\/)/.test(source.trim())) throw new Error("that isn't a magnet link or a link to a .torrent file");
      const name = title || new URLSearchParams(source.split("?")[1] ?? "").get("dn") || "Download";
      const d = dl({ id: `sg-${Date.now()}`, title: name, name, source, size: 2 * gb, downSpeed: 8e7, seeds: 12, peers: 2, engine: "downloading", created: now() });
      downloads = [...downloads, d];
      changed();
      return copy(d);
    },
    async action(id, action) {
      const step = (d: Download): Download => {
        switch (action) {
          case "pause":
            return { ...d, state: "paused", downSpeed: 0, engine: "paused" };
          case "resume":
            return { ...d, state: "downloading", error: undefined, downSpeed: 5e6, engine: "downloading" };
          case "install":
            setTimeout(() => {
              downloads = downloads.map((x) => (x.id === id ? { ...x, state: "installed", installedAt: now(), installDone: undefined } : x));
              changed();
            }, 3000);
            return { ...d, state: "installing", installDone: 0.6 * d.size };
          case "uninstall":
            return { ...d, state: "downloaded", installedAt: undefined };
          case "recheck":
            return { ...d, state: "scanning", safety: undefined };
        }
        return d;
      };
      downloads = downloads.filter((d) => !(d.id === id && action === "remove")).map((d) => (d.id === id ? step(d) : d));
      changed();
    },
    async showDownload() {},
    onDownloads(cb) {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    onEngine(cb) {
      engineListeners.add(cb);
      return () => engineListeners.delete(cb);
    },
    async feeds() {
      return getSettings().store.feeds.map((f) => feedInfo(f.url, f.enabled));
    },
    async addFeed(url) {
      if (!url.startsWith("https://")) throw new Error("only https:// addresses (or http:// on this PC)");
      const s = getSettings();
      if (s.store.feeds.some((f) => f.url === url)) throw new Error("that feed is added already");
      feedNames[url] = new URL(url).hostname;
      setSettings({ ...s, store: { ...s.store, feeds: [...s.store.feeds, { url, enabled: true, trust: 0 }] } });
      return copy(getSettings());
    },
    async removeFeed(url) {
      const s = getSettings();
      setSettings({ ...s, store: { ...s.store, feeds: s.store.feeds.filter((f) => f.url !== url) } });
      return copy(getSettings());
    },
    async setFeedEnabled(url, on) {
      const s = getSettings();
      setSettings({ ...s, store: { ...s.store, feeds: s.store.feeds.map((f) => (f.url === url ? { ...f, enabled: on } : f)) } });
      return copy(getSettings());
    },
    async refreshFeeds() {
      return getSettings().store.feeds.map((f) => feedInfo(f.url, f.enabled));
    },
    async catalog(q) {
      const on = new Set(getSettings().store.feeds.filter((f) => f.enabled).map((f) => f.url));
      if (getSettings().store.privateSources) { on.add("https://fitgirl-repacks.site/feed/"); on.add("https://dodi-repacks.site/"); }
      let hits = catalogEntries
        .map((e) => ({ ...e, offers: e.offers.filter((o) => on.has(o.feedUrl)) }))
        .filter((e) => e.offers.length && squash(e.title).includes(squash(q.text)))
        .filter((e) => !q.language || e.languages.some((l) => l.toLowerCase() === q.language.toLowerCase()));
      if (q.sort === "updated") hits = [...hits].sort((a, b) => b.updated.localeCompare(a.updated));
      if (q.sort === "size") hits = [...hits].sort((a, b) => a.size - b.size);
      return copy({ entries: hits.slice(q.offset, q.offset + (q.limit || 200)).map(annotate), total: hits.length });
    },
    async catalogLanguages() {
      return [...new Set(catalogEntries.flatMap((e) => e.languages))];
    },
    async catalogEntry(key) {
      const e = catalogEntries.find((x) => x.key === key);
      if (!e) throw new Error("that game isn't in the catalog anymore");
      return copy(annotate(e));
    },
    async downloadOffer(key, i, opts) {
      const e = catalogEntries.find((x) => x.key === key);
      const o = e?.offers[i];
      if (!e || !o) throw new Error("that version isn't offered anymore");
      const title = o.version ? `${e.title} ${o.version}` : e.title;
      const d = dl({ id: `sg-${Date.now()}`, title, name: e.title, source: o.magnet ?? "", size: o.sizeBytes ?? 0, downSpeed: 3e7, seeds: 40, peers: 8, engine: "downloading", created: now(), gameKey: key, version: o.version, feedName: o.feedName, installDir: opts.dir, language: opts.language, autoInstall: opts.install });
      downloads = [...downloads, d];
      changed();
      return copy(d);
    },
    onCatalog() {
      return () => {};
    },
    async installFolder(title) {
      return `${getSettings().store.games || "C:\\Users\\you\\Games"}\\${title.replace(/[<>:"/\\|?*]/g, "")}`;
    },
    async chooseInstallFolder(current) {
      return current.replace(/^C:\\Users\\you\\Games/, "D:\\Games");
    },
    async gamesFolder() {
      return getSettings().store.games || "C:\\Users\\you\\Games";
    },
    async chooseGamesFolder() {
      const s = getSettings();
      setSettings({ ...s, store: { ...s.store, games: "D:\\Games" } });
      return copy(getSettings());
    },
    async art(keys) {
      const out: StoreArt[] = [];
      for (const k of keys) {
        const e = catalogEntries.find((x) => x.key === k);
        const b = e && blurbs[e.title];
        if (b) out.push({ key: k, meta: { description: b[0], developers: [b[1]], genres: b[2], releaseYear: Number(e.updated.slice(0, 4)) || undefined } });
        else if (mockDiscoveryArt(k)) out.push({ key: k, meta: mockDiscoveryArt(k)! });
      }
      return out;
    },
    onArt() {
      return () => {};
    },
    async allowDownload(id, confirm) {
      const d = downloads.find((x) => x.id === id);
      if (!d || confirm.trim().toLowerCase() !== d.title.toLowerCase()) throw new Error("type the download's name exactly to install it anyway");
      downloads = downloads.map((x) => (x.id === id && x.safety ? { ...x, state: "downloaded", safety: { ...x.safety, overridden: true } } : x));
      changed();
    },
    async hasVirusTotalKey() {
      return vtKey;
    },
    async setVirusTotalKey(k) {
      vtKey = !!k;
    },
    async sandboxAvailable() {
      return true;
    },
    async updates() {
      return copy(catalogEntries.map(annotate).filter((e) => e.installed?.update));
    },
    async holdDownloads(on) {
      engine = { ...engine, held: on };
      engineListeners.forEach((cb) => cb(copy(engine)));
      changed();
    },
    async openInSandbox() {},
  };
}
