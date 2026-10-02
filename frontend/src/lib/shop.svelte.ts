// The experimental store's state: downloads and the download engine.
// Loaded the first time the store is opened, kept up to date by events.
import { api } from "./api";
import { activeCount } from "./downloads";
import type { Download, EngineStatus } from "./types";

class ShopStore {
  downloads = $state<Download[]>([]);
  engine = $state<EngineStatus | null>(null);
  active = $derived(activeCount(this.downloads));
  private started = false;

  /** Starts following downloads; safe to call again. */
  start() {
    if (this.started) return;
    this.started = true;
    api.store.onDownloads((d) => (this.downloads = d));
    api.store.onEngine((s) => (this.engine = s));
    void this.refresh();
  }

  async refresh() {
    const [downloads, engine] = await Promise.all([api.store.downloads(), api.store.engine()]);
    this.downloads = downloads;
    this.engine = engine;
  }
}

export const shop = new ShopStore();
