// What a download's row says: pure, so it can be tested.
import { bytes } from "./format";
import type { Download, EngineStatus } from "./types";

/** Downloads still on their way (what the sidebar counts). */
export function activeCount(ds: Download[]): number {
  return ds.filter((d) => d.state === "queued" || d.state === "downloading").length;
}

export function progress(d: Download): number {
  return d.size > 0 ? Math.min(1, d.done / d.size) : d.state === "downloaded" ? 1 : 0;
}

/** Seconds as "3 min", "1 h 5 min", "under a minute". */
export function eta(sec: number): string {
  if (sec <= 0) return "";
  if (sec < 60) return "under a minute";
  const m = Math.round(sec / 60);
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  return m % 60 ? `${h} h ${m % 60} min` : `${h} h`;
}

const speed = (n: number) => `${bytes(n)}/s`;

/** The line under a download's title. */
export function statusLine(d: Download, engine: EngineStatus | null): string {
  const size = d.size ? `${bytes(d.done)} of ${bytes(d.size)}` : "";
  switch (d.state) {
    case "failed":
      return d.error || "Stopped with an error";
    case "paused":
      return ["Paused", size].filter(Boolean).join(" · ");
    case "downloaded":
      return d.seeding ? `Done · ${bytes(d.size)} · sharing at ${speed(d.upSpeed)}` : `Done · ${bytes(d.size)}`;
  }
  if (engine?.interfaceMissing) return "Waiting: the network interface downloads are bound to is gone";
  if (engine?.gameRunning) return "Waiting for your game to close";
  if (engine && !engine.running) return engine.error ? `Waiting: ${engine.error}` : "Starting qBittorrent…";
  if (d.state === "queued") return "Queued";
  switch (d.engine) {
    case "metadata":
      return "Asking peers for the file list…";
    case "checking":
      return ["Checking what's already downloaded", size].filter(Boolean).join(" · ");
    case "queued":
      return ["Waiting for a free slot", size].filter(Boolean).join(" · ");
  }
  if (!d.downSpeed) return [size, d.seeds + d.peers ? `${d.seeds + d.peers} peers, no data yet` : "Looking for peers…"].filter(Boolean).join(" · ");
  return [size, speed(d.downSpeed), eta(d.eta) && `${eta(d.eta)} left`].filter(Boolean).join(" · ");
}
