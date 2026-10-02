// The experimental store for `npm run dev:mock`: a pretend engine whose
// downloads move along on their own.
import type { Api } from "./api";
import type { Download, EngineStatus, Settings, StoreSettings } from "./types";

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
  };
}
