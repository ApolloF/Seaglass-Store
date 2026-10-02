// The experimental store for `npm run dev:mock`: a pretend engine whose
// downloads move along on their own.
import type { Api } from "./api";
import type { CatalogEntry, CatalogOffer, Download, EngineStatus, FeedInfo, Settings, StoreSettings } from "./types";

export const mockStoreSettings: StoreSettings = {
  qbittorrent: "",
  downloads: "",
  pauseWhilePlaying: true,
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
    { url: "https://feeds.example/indie.json", enabled: true },
    { url: "https://feeds.example/freeware.json", enabled: true },
    { url: "https://old.example/feed.json", enabled: false },
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
];
let engine: EngineStatus = { installed: true, exe: "C:\\Program Files\\qBittorrent\\qbittorrent.exe", running: true, version: "v5.1.4", interfaceMissing: false, gameRunning: false };
let proxyPassword = false;
const listeners = new Set<(d: Download[]) => void>();
const engineListeners = new Set<(s: EngineStatus) => void>();
const copy = <T>(v: T): T => structuredClone(v);
const changed = () => listeners.forEach((cb) => cb(copy(downloads)));

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
const squash = (t: string) => t.toLowerCase().replace(/[^a-z0-9]/g, "");

function feedInfo(url: string, enabled: boolean): FeedInfo {
  const items = catalogEntries.reduce((n, e) => n + e.offers.filter((o) => o.feedUrl === url).length, 0);
  return { url, enabled, name: feedNames[url] ?? "", items, skipped: url === FREE ? 2 : 0, fetched: feedErrors[url] ? 0 : now() - 1800, error: feedErrors[url] };
}

/** The store namespace; settings come and go through the mock's own settings. */
export function mockStore(getSettings: () => Settings, setSettings: (s: Settings) => void): Api["store"] {
  return {
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
      downloads = downloads
        .filter((d) => !(d.id === id && action === "remove"))
        .map((d) => (d.id !== id ? d : action === "pause" ? { ...d, state: "paused", downSpeed: 0, engine: "paused" } : { ...d, state: "downloading", error: undefined, downSpeed: 5e6, engine: "downloading" }));
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
      setSettings({ ...s, store: { ...s.store, feeds: [...s.store.feeds, { url, enabled: true }] } });
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
      let hits = catalogEntries
        .map((e) => ({ ...e, offers: e.offers.filter((o) => on.has(o.feedUrl)) }))
        .filter((e) => e.offers.length && squash(e.title).includes(squash(q.text)))
        .filter((e) => !q.language || e.languages.some((l) => l.toLowerCase() === q.language.toLowerCase()));
      if (q.sort === "updated") hits = [...hits].sort((a, b) => b.updated.localeCompare(a.updated));
      if (q.sort === "size") hits = [...hits].sort((a, b) => a.size - b.size);
      return copy({ entries: hits.slice(q.offset, q.offset + (q.limit || 200)), total: hits.length });
    },
    async catalogLanguages() {
      return [...new Set(catalogEntries.flatMap((e) => e.languages))];
    },
    async catalogEntry(key) {
      const e = catalogEntries.find((x) => x.key === key);
      if (!e) throw new Error("that game isn't in the catalog anymore");
      return copy(e);
    },
    async downloadOffer(key, i) {
      const e = catalogEntries.find((x) => x.key === key);
      const o = e?.offers[i];
      if (!e || !o) throw new Error("that version isn't offered anymore");
      const title = o.version ? `${e.title} ${o.version}` : e.title;
      const d = dl({ id: `sg-${Date.now()}`, title, name: e.title, source: o.magnet ?? "", size: o.sizeBytes ?? 0, downSpeed: 3e7, seeds: 40, peers: 8, engine: "downloading", created: now(), gameKey: key, version: o.version, feedName: o.feedName });
      downloads = [...downloads, d];
      changed();
      return copy(d);
    },
    onCatalog() {
      return () => {};
    },
  };
}
