// The experimental store's state: downloads and the download engine.
// Loaded the first time the store is opened, kept up to date by events.
import { api } from "./api";
import { activeCount } from "./downloads";
import type { Download, EngineStatus, Meta } from "./types";

class ShopStore {
  downloads = $state<Download[]>([]);
  engine = $state<EngineStatus | null>(null);
  active = $derived(activeCount(this.downloads));
  /** Catalog games' art and descriptions, by entry key. */
  art = $state<Record<string, Meta>>({});
  private asked = new Set<string>();
  private started = false;

  /** Starts following downloads; safe to call again. */
  start() {
    if (this.started) return;
    this.started = true;
    api.store.onDownloads((d) => (this.downloads = d));
    api.store.onEngine((s) => (this.engine = s));
    api.store.onArt((a) => (this.art[a.key] = a.meta));
    void this.refresh();
  }

  /** Asks for art the interface is about to show (the first key first). */
  requestArt(keys: string[]) {
    const missing = keys.filter((k) => !this.asked.has(k));
    if (!missing.length) return;
    missing.forEach((k) => this.asked.add(k));
    api.store.art(missing).then((list) => list.forEach((a) => (this.art[a.key] = a.meta))).catch(() => {});
  }

  async refresh() {
    const [downloads, engine] = await Promise.all([api.store.downloads(), api.store.engine()]);
    this.downloads = downloads;
    this.engine = engine;
  }
}

export const shop = new ShopStore();
