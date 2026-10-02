// What a download's row says: pure, so it can be tested.
import { bytes } from "./format";
import type { Download, EngineStatus } from "./types";

/** Downloads still on their way, checks and installs included (what the sidebar counts). */
export function activeCount(ds: Download[]): number {
  return ds.filter((d) => d.state === "queued" || d.state === "downloading" || d.state === "scanning" || d.state === "installing").length;
}

/** Downloads waiting for the person: checked and ready, blocked, or failed. */
export function needsYou(d: Download): boolean {
  return (d.state === "downloaded" && !!d.safety && !d.autoInstall) || (d.state === "downloaded" && d.safety?.verdict === "warn") || d.state === "blocked";
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
  const sharing = d.seeding ? ` · sharing at ${speed(d.upSpeed)}` : "";
  switch (d.state) {
    case "failed":
      return d.error || "Stopped with an error";
    case "paused":
      return ["Paused", size].filter(Boolean).join(" · ");
    case "scanning":
      return "Downloaded · running the safety checks…";
    case "blocked":
      return "Not installed: the safety checks found a problem";
    case "installing":
      if (d.stalled) return "The installer has shown no progress for a while. It may be waiting for an answer in a window of its own.";
      return ["Installing…", d.installDone ? `${bytes(d.installDone)} so far` : ""].filter(Boolean).join(" · ");
    case "installed":
      return d.installDir ? `Installed in ${d.installDir}` : "Installed where its installer chose";
    case "downloaded":
      if (!d.safety) return `Downloaded · ${bytes(d.size)}${sharing}`;
      if (d.safety.verdict === "warn" && !d.safety.overridden) return `Downloaded · the safety checks have warnings${sharing}`;
      return `Downloaded and checked · ready to install${sharing}`;
  }
  if (engine?.held) return ["Paused until you resume all downloads", size].filter(Boolean).join(" · ");
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
