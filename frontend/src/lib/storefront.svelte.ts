// The desktop Store's shared state: what discovery is doing, the wishlist and
// the enrichment of cards on screen. Started when the Store opens, kept up to
// date by events.
import { api } from "./api";
import { lib } from "./store.svelte";
import { learnSources, unreadCount, withEnrichment } from "./storefront";
import type { DiscoveryStatus, Enrichment, GameSummary, WishlistItem } from "./types";

class Storefront {
  status = $state<DiscoveryStatus | null>(null);
  wishlist = $state<WishlistItem[]>([]);
  /** The wishlist was read once. Until then a summary's own flag is all there is. */
  wishlistReady = $state(false);
  /** Cached or fetched enrichment by game key. */
  enriched = $state<Record<string, Enrichment>>({});
  unread = $derived(unreadCount(this.wishlist));
  private asked = new Set<string>();
  private started = false;

  start() {
    if (this.started) return;
    this.started = true;
    api.store.discovery.onStatus((s) => this.setStatus(s));
    api.store.wishlist.onChange((w) => this.setWishlist(w));
    api.store.enrich.onEnrichment((e) => (this.enriched[e.key] = e));
    void this.load();
  }

  setStatus(status: DiscoveryStatus) {
    learnSources(status.sources);
    this.status = status;
  }

  setWishlist(list: WishlistItem[]) {
    this.wishlist = list;
    this.wishlistReady = true;
  }

  async load() {
    const [status, wishlist] = await Promise.allSettled([api.store.discovery.status(), api.store.wishlist.list()]);
    if (status.status === "fulfilled") this.setStatus(status.value);
    if (wishlist.status === "fulfilled") this.setWishlist(wishlist.value);
  }

  /** Asks for the enrichment of cards about to show; answers arrive by event. */
  ask(keys: string[]) {
    const fresh = keys.filter((k) => !this.asked.has(k));
    if (!fresh.length) return;
    fresh.forEach((k) => this.asked.add(k));
    api.store.enrich
      .games(fresh)
      .then((list) => list.forEach((e) => (this.enriched[e.key] = e)))
      .catch(() => fresh.forEach((k) => this.asked.delete(k)));
  }

  /** A card's game with whatever enrichment and wishlist state is known. */
  card(g: GameSummary): GameSummary {
    const item = this.wishlist.find((w) => w.key === g.key);
    return { ...withEnrichment(g, this.enriched[g.key]), wishlisted: this.wished(g.key, g.wishlisted), activity: this.wishlistReady ? !!item?.unread : g.activity };
  }

  /** Adds a game to the wishlist, or takes it off. */
  async toggleWish(g: Pick<GameSummary, "key" | "title" | "steamAppId" | "wishlisted">) {
    const on = this.wished(g.key, g.wishlisted);
    const list = await lib.run(() => (on ? api.store.wishlist.remove(g.key) : api.store.wishlist.add(g.key, g.title, g.steamAppId ?? 0)));
    if (list) {
      this.setWishlist(list);
      lib.toast(on ? `${g.title} is off your wishlist.` : `${g.title} is on your wishlist.`);
    }
  }

  /** On the wishlist. `fallback` is a summary's own flag, used before the list is read. */
  wished(key: string, fallback = false) {
    return this.wishlistReady ? this.wishlist.some((w) => w.key === key) : fallback;
  }
}

export const storefront = new Storefront();
