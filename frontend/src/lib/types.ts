// Shapes of what the Go side sends. They mirror the JSON of
// internal/library.Game, internal/settings.Settings and friends.

export interface Meta {
  description?: string;
  developers?: string[];
  publishers?: string[];
  genres?: string[];
  releaseDate?: string;
  releaseYear?: number;
  dualSense?: string; // "yes" | "no" | ""
  controller?: string;
  cover?: string;
  hero?: string;
  backdrop?: string; // 16:9, for full-screen backgrounds
  tile?: string; // square: key art with the logo, for round tiles (Orbit)
  logo?: string;
  icon?: string;
  accent?: string;
  fetchedAt?: number;
  source?: string;
  /** Art the user chose (cover, hero, backdrop, logo): kept through refreshes. */
  artOverrides?: string[];
}

export interface Game {
  id: number;
  key: string;
  title: string;
  customTitle?: string;
  sortTitle: string;
  source: string;
  sourceLabel: string;
  external: boolean;
  emulator?: string;
  emuDir?: string;
  repacker?: string;
  drmFree?: string;
  installed: boolean;
  padHint?: string; // "libScePad" | "SDL"
  dir: string;
  exe?: string;
  args?: string;
  workDir?: string;
  launchUri?: string;
  userExe?: boolean;
  owned?: boolean; // a connected store account owns it
  installUri?: string; // asks the store to install it
  sizeBytes?: number;
  steamAppId?: number;
  metaAppId?: number;
  gogId?: string;
  epicApp?: string;
  how: string;
  matchHow: string;
  confidence: number;
  needsReview: boolean;
  confirmed?: boolean;
  addedAt: number;
  initial?: boolean;
  seenAt: number;
  lastPlayed?: number;
  playtime?: number;
  storePlaytime?: number;
  storeLastPlayed?: number;
  favorite?: boolean;
  /** The user's own groups the game is in ("Co-op", "Finished"). */
  collections?: string[];
  hidden?: boolean;
  padMode?: string; // "" | "native" | "steam"
  meta?: Meta;
}

export type Layout = "deck" | "console" | "orbit";

export interface Settings {
  folders: string[];
  autoFolders: boolean;
  detectExternal: boolean;
  reviewUncertain: boolean;
  showNotInstalled: boolean;
  showOwned: boolean;
  ownedGOG: boolean;
  /** Libraries whose games aren't shown (ids from SOURCE_GROUPS). */
  hiddenSources: string[];
  theme: "system" | "dark" | "light";
  bigPictureLayout: Layout;
  openBigPictureOnController: boolean;
  startInBigPicture: boolean;
  sounds: boolean;
  haptics: boolean;
  lightbar: boolean;
  psButton: boolean;
  glyphs: "auto" | "playstation" | "xbox";
  closeWhilePlaying: boolean;
  padWhilePlaying: "listen" | "off";
  noticeExternal: boolean;
  syncSavesBefore: boolean;
  backupSavesAfter: boolean;
  /** Seconds to wait for a sync before playing: 30, 60, 150 or 300. */
  syncWait: number;
  /** Start Syncer (without its window) when it isn't running. */
  startSyncer: boolean;
  /** Syncer syncs playtime, achievements and settings between PCs. */
  syncProfile: boolean;
  /** Take the settings saved last on another PC (the PC's own ones stay). */
  sameSettings: boolean;
  /** With several Syncer accounts, ask who's playing before a game starts. */
  askWhoPlays: boolean;
  autoUpdate: boolean;
  /** Read and show achievements. */
  achievements: boolean;
  /** Show hidden achievements before they're unlocked (spoilers). */
  showHiddenAchievements: boolean;
  /** Experimental: the store, with catalogs from feeds the user adds. This PC only. */
  experimentalStore: boolean;
  store: StoreSettings;
  /** The first-start welcome was seen (or skipped). */
  welcomed: boolean;
}

/** The experimental store's settings. Mirrors internal/settings.StoreSettings. */
export interface StoreSettings {
  /** qbittorrent.exe; "" uses the installed one. */
  qbittorrent: string;
  /** Where downloads go; "" is Downloads\Seaglass. */
  downloads: string;
  /** Where games are installed; "" is Games in the user's folder. */
  games: string;
  /** Keep a download (and share it) after its game is installed. */
  keepDownloads: boolean;
  pauseWhilePlaying: boolean;
  /** Defender or VirusTotal detections block an install (else they warn). */
  blockDetections: boolean;
  /** The language versions are recommended in; "" for any. */
  language: string;
  network: TorrentNetwork;
  /** Catalog feeds, in the order they were added. */
  feeds: FeedSource[];
}

export interface FeedSource {
  url: string;
  enabled: boolean;
  /** -2 (less) … 2 (more): weighs in when versions are recommended. */
  trust: number;
}

/** A feed and its last fetch. Mirrors internal/app.FeedInfo. */
export interface FeedInfo {
  url: string;
  /** From the feed; "" before its first good fetch. */
  name: string;
  items: number;
  /** Items left out because they didn't pass the checks. */
  skipped: number;
  /** Unix seconds of the last good fetch. */
  fetched: number;
  error?: string;
  enabled: boolean;
}

/** One downloadable version of a game, from one feed. Mirrors internal/store/catalog.Offer. */
export interface CatalogOffer {
  title: string;
  version?: string;
  buildDate?: string;
  sizeBytes?: number;
  installedSizeBytes?: number;
  magnet?: string;
  torrentUrl?: string;
  languages?: string[];
  platform?: string;
  installerType?: string;
  sha256?: string;
  steamAppId?: number;
  notes?: string;
  feedUrl: string;
  feedName: string;
}

/** One game in the catalog. Mirrors internal/store/catalog.Entry. */
export interface CatalogEntry {
  key: string;
  title: string;
  steamAppId?: number;
  /** Newest version first. */
  offers: CatalogOffer[];
  version: string;
  /** Newest build date, YYYY-MM-DD. */
  updated: string;
  size: number;
  languages: string[];
  /** The version to get, and why. */
  recommended?: { offer: number; why: string[] };
  /** The store installed this game. */
  installed?: { download: string; version: string; dir: string; update: boolean };
}

export interface CatalogQuery {
  text: string;
  language: string;
  sort: "title" | "updated" | "size";
  offset: number;
  limit: number;
}

export interface CatalogPage {
  entries: CatalogEntry[];
  total: number;
}

/** How the download engine connects. Mirrors internal/torrent.Network. */
export interface TorrentNetwork {
  /** Binds all traffic to this interface (the engine's id for it); "" uses any. */
  interface: string;
  /** One address of the interface; "" uses all. */
  address: string;
  /** Incoming connections; 0 picks a random port at each start. */
  port: number;
  upnp: boolean;
  proxy: "none" | "socks5" | "http";
  proxyHost: string;
  proxyPort: number;
  proxyUser: string;
  proxyPeers: boolean;
  encryption: "prefer" | "require" | "off";
  dht: boolean;
  pex: boolean;
  lsd: boolean;
  anonymous: boolean;
  /** KiB/s; 0 for no limit. */
  downLimit: number;
  upLimit: number;
  maxActive: number;
  /** Stop seeding at this ratio; 0 when complete, -1 never. */
  seedRatio: number;
}

/** The download engine. Mirrors internal/app.EngineStatus. */
export interface EngineStatus {
  installed: boolean;
  exe: string;
  running: boolean;
  version?: string;
  error?: string;
  /** Bound to an interface that's gone (a VPN that disconnected): nothing moves. */
  interfaceMissing: boolean;
  /** Downloads wait for the game to close. */
  gameRunning: boolean;
  /** Paused from the tray (or here) until resumed. */
  held: boolean;
}

export interface TorrentInterface {
  id: string;
  name: string;
}

export type DownloadState = "queued" | "downloading" | "paused" | "scanning" | "downloaded" | "blocked" | "installing" | "installed" | "failed";
export type DownloadAction = "pause" | "resume" | "remove" | "install" | "uninstall" | "recheck";

export type SafetyLevel = "ok" | "info" | "warn" | "block";

/** What the safety checks found. Mirrors internal/safety.Report. */
export interface SafetyReport {
  verdict: "clean" | "warn" | "block";
  findings: { check: string; level: SafetyLevel; text: string }[];
  /** The installer, relative to the download. */
  main?: string;
  sha256?: string;
  checked: number;
  /** The person chose to install it anyway. */
  overridden?: boolean;
}

/** One download. Mirrors internal/store/jobs.Job. */
export interface Download {
  id: string;
  title: string;
  source: string;
  savePath: string;
  hash?: string;
  name?: string;
  state: DownloadState;
  error?: string;
  /** The engine's own state: metadata, checking, queued, … */
  engine?: string;
  size: number;
  done: number;
  downSpeed: number;
  upSpeed: number;
  seeds: number;
  peers: number;
  /** Seconds; 0 when unknown. */
  eta: number;
  seeding: boolean;
  created: number;
  finished?: number;
  /** From the catalog; empty for a pasted link. */
  gameKey?: string;
  version?: string;
  feedName?: string;
  installDir?: string;
  language?: string;
  autoInstall?: boolean;
  sha256?: string;
  safety?: SafetyReport;
  /** inno, nsis, msi, archive, portable or other. */
  installer?: string;
  uninstaller?: string;
  installedAt?: number;
  /** Bytes in the game's folder while installing. */
  installDone?: number;
  /** The installer has done nothing visible for a while. */
  stalled?: boolean;
}

/** Chosen before a game downloads. Mirrors internal/app.InstallOptions. */
export interface InstallOptions {
  /** The game's own folder; "" uses one in the games folder. */
  dir: string;
  /** One of the offer's languages; "" for the installer's default. */
  language: string;
  /** Install once downloaded and checked; false only downloads. */
  install: boolean;
  /** Install over the version the store installed before, in its folder. */
  update?: boolean;
}

/** A catalog game's art and description. */
export interface StoreArt {
  key: string;
  meta: Meta;
}

/** One achievement. Mirrors internal/achievements.Achievement. */
export interface Achievement {
  id: string;
  name: string;
  desc?: string;
  /** /ach/… URL; "" when there's none (a generic icon is drawn). */
  icon?: string;
  iconGray?: string;
  hidden?: boolean;
  unlocked: boolean;
  /** Unix seconds; 0 or missing when unlocked at an unknown time. */
  unlockedAt?: number;
  progress?: number;
  max?: number;
  /** Share of all players who have it, 0–100. */
  percent?: number;
}

/** A game's achievements. Mirrors internal/achievements.List. */
export interface Achievements {
  gameId: number;
  /** "steam", "epic", "gog", "Goldberg", "CODEX", …; "" when none was found. */
  source: string;
  total: number;
  unlocked: number;
  items: Achievement[];
  updatedAt: number;
  /** What's missing, and how to get it. */
  hint?: string;
  /** What Seaglass can do about the hint: "uplay-ini" turns achievements on in a Uplay emulator's ini. */
  fix?: string;
}

/** What a play session unlocked (achievements:session). */
export interface SessionAchievements {
  gameId: number;
  title: string;
  unlocked: Achievement[];
}

/** One save folder Syncer looks after. */
export interface SaveFolder {
  id: string;
  label: string;
  path: string;
  sync: boolean;
  backup: boolean;
  state: string;
  needBytes: number;
  errors: number;
  conflicts: number;
  exists: boolean;
  modified: string;
  backedUp: string;
  newerOn: string;
  newerAt: string;
}

/** What Syncer knows about a game's saves. */
/** How Seaglass and Syncer get on. */
export interface SyncerStatus {
  installed: boolean;
  version?: string;
  outdated: boolean;
  connected: boolean;
  running: boolean;
  error?: string;
  syncing: boolean;
  paused: boolean;
  pausedUntil?: number;
  backingUp: boolean;
  lastBackup?: number;
  games: number;
  conflicts: number;
  checkedAt: number;
}

/** One of Syncer's accounts: a person with their own saves. */
export interface SyncerAccount {
  id: string;
  name: string;
  color?: string;
  active: boolean;
}

/** Who's playing on this PC, and how their playtime, achievements and
 * settings get to their other PCs. */
export interface Profile {
  /** Syncer has accounts turned on. */
  enabled: boolean;
  active?: string;
  accounts: SyncerAccount[];
  /** Whose playtime this PC adds to: an account id or "shared". */
  owner: string;
  ownerName?: string;
  installed: boolean;
  reachable: boolean;
  /** Syncer knows accounts (new enough). */
  supported: boolean;
  /** Syncer syncs the profile folder (and backs it up). */
  synced: boolean;
  backup: boolean;
  /** Someone stopped syncing it in Syncer. */
  dismissed: boolean;
  dataError?: string;
  dir: string;
  /** The PC the settings in use were saved on ("" this one). */
  settingsFrom?: string;
  checkedAt?: number;
}

export interface Saves {
  installed: boolean;
  outdated?: boolean;
  available: boolean;
  error?: string;
  known: boolean;
  folders: SaveFolder[];
}

export interface ScanState {
  running: boolean;
  lastScan: number;
  tookMs: number;
  games: number;
  added: number;
  known: number;
  error?: string;
}

export interface MetaState {
  running: boolean;
  done: number;
  total: number;
}

/** A picture a game's art can be changed to (already stored). */
export interface ArtChoice {
  art: string;
  source: string;
  width: number;
  height: number;
}
export type ArtKind = "cover" | "backdrop" | "hero" | "logo";

export interface StoreHit {
  appId: number;
  name: string;
  image: string;
}

/** Every button (bit n = SDL gamepad button n) and axis of the controller in use. */
export interface PadRaw {
  buttons: number;
  axes: number[]; // left x, y, right x, y, left trigger, right trigger
}

export type PadKind = "playstation" | "xbox" | "nintendo" | "other";

export interface PadState {
  connected: boolean;
  name: string;
  kind: PadKind;
  dualSense: boolean;
  battery: number; // -1 unknown
  wireless: boolean;
  error?: string;
}

export type Phase = "preparing" | "starting" | "running" | "finishing" | "ended" | "failed" | "cancelled" | "";

export interface StepState {
  id: string;
  label: string;
  status: "pending" | "running" | "done" | "skipped" | "failed";
  detail?: string;
}

export interface Question {
  id: number;
  text: string;
  options: { id: string; label: string }[];
}

/** A game being launched or played (or the last one). */
export interface Session {
  id: number;
  gameId: number;
  title: string;
  phase: Phase;
  /** "external": started outside Seaglass and noticed. */
  route: "direct" | "store" | "steamInput" | "external" | "";
  before: StepState[];
  after: StepState[];
  question?: Question;
  startedAt?: number;
  seconds: number;
  error?: string;
  note?: string;
  /** The game's own window has come to the front (before that it's loading). */
  shown?: boolean;
  /** The exit code (0xC0000005) when the game crashed. */
  crash?: string;
  /** Where it was started: a mode of the interface, or outside it (a shortcut, a game noticed). */
  from?: "bigpicture" | "desktop";
}

/** Still loading: running, but its window hasn't come to the front yet
 * (some games never bring one forward themselves: after a minute it's
 * taken as running). */
export const sessionLoading = (s: Session) => s.phase === "running" && !s.shown && s.route !== "external" && s.seconds < 60;

/** What a crash says, or "" when the game quit normally. */
export const crashText = (s: Session) =>
  s.crash ? `${s.title || "The game"} closed unexpectedly (error ${s.crash}). Its playtime up to then is counted.` : "";

export const sessionActive = (s: Session | null | undefined) =>
  !!s && s.phase !== "" && s.phase !== "ended" && s.phase !== "failed" && s.phase !== "cancelled";

export interface StoreAccount {
  connected: boolean;
  available: boolean;
  name?: string;
  games: number;
  synced?: number;
  syncing: boolean;
  error?: string;
}

export interface Accounts {
  steam: StoreAccount;
  gog: StoreAccount;
  epic: StoreAccount;
  /** The GOG account signed in for achievements (Galaxy's library needs none). */
  gogSignIn: StoreAccount;
}

/** Only known from a store account: never found on this PC. */
export const ownedOnly = (g: Game) => g.key.startsWith("owned:");

/** The store's name for an install button. */
export const storeName = (g: Game) => ({ steam: "Steam", gog: "GOG Galaxy", epic: "Epic" })[g.source] ?? "its store";

export interface AppInfo {
  version: string;
  dataDir: string;
  logFile: string;
  /** The previous run crashed. */
  crashedLastTime: boolean;
}

/** Mirrors internal/app.UpdateState. */
export interface UpdateState {
  current: string;
  latest?: string;
  status: "off" | "idle" | "checking" | "uptodate" | "available" | "downloading" | "ready" | "error";
  /** 0 to 1 while downloading. */
  progress: number;
  notes?: string;
  page: string;
  error?: string;
  checkedAt: number;
  /** An install of `latest` was started before and didn't take. */
  failed: boolean;
}

/** Whether Seaglass starts when you sign in to Windows. */
export interface Startup {
  on: boolean;
  /** Turned off in Task Manager's Startup apps. */
  disabledByUser: boolean;
}

export const title = (g: Game) => g.customTitle || g.title;

/** Playtime in seconds: Seaglass's own or the store's, whichever is larger. */
export const played = (g: Game) => Math.max(g.playtime ?? 0, g.storePlaytime ?? 0);

/** When the game was last played, by Seaglass or the store. */
export const lastPlayed = (g: Game) => Math.max(g.lastPlayed ?? 0, g.storeLastPlayed ?? 0);
