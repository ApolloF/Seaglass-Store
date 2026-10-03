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
  disablePayloadScanning: boolean;
  /** Browse repack sources: discovery runs only with this on. */
  privateSources: boolean;
  /** The sources discovery indexes on this PC (fitgirl, dodi). */
  sources: string[];
  /** "": decided when the Store is turned on; "ask": choose once; "done". */
  sourceSetup: "" | "ask" | "done";
  /** The person paused background indexing on this PC. */
  indexingPaused: boolean;
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
  payloadSkipped?: boolean;
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
export interface SourceRelease {
  id: string; sourceId: string; title: string; rawTitle: string; version?: string; pageUrl: string;
  releaseKind: string; summaryOnly?: boolean; languageClaim?: string; warnings: string[];
  transports: { infoHash?: string; torrentName?: string; uri: string }[];
  references: { url: string; state?: string; reason?: string }[];
}
export interface SourceSnapshot { entries: SourceRelease[]; warnings?: string[]; }
export interface DownloadLanguageOptions {
  game: string[]; installer: { id: string; name: string }[]; torrent: boolean; note: string;
}

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
  languages?: string[];
  setupLanguage?: string;
  askInstaller?: boolean;
  languageApplied?: boolean;
  pendingLanguages?: boolean;
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
  /** "" for Seaglass; "Store Edition" for the edition with the store. */
  edition: string;
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

// ---------------------------------------------------------------------------
// Store discovery, enrichment and wishlist. Mirror internal/store/discovery,
// internal/store/enrich, internal/store/wishlist and internal/app
// (store_discovery_service.go). Times are unix seconds; 0 is unknown.

/** Provider and request states. A failure never hides what is cached: it comes back "stale". */
export type ProviderState = "ok" | "loading" | "stale" | "unavailable" | "error" | "skipped";
export type CrawlState = "idle" | "recent" | "backfill" | "paused" | "backoff" | "disabled";
export type ReleaseAvailability = "installable" | "unresolved" | "manual" | "update-only" | "preview" | "summary" | "unavailable";

/** One source's indexing state. Mirrors discovery.SourceStatus. */
export interface DiscoverySourceStatus {
  id: string;
  name: string;
  enabled: boolean;
  state: CrawlState;
  releases: number;
  recentAt: number;
  /** Next older listing page; 0 before the first pass. */
  backfillPage: number;
  backfillDone: boolean;
  retryAt: number;
  error?: string;
  /** What the provider supports (sources.Provider). */
  host: string;
  /** Its own site search fills search results. */
  search: boolean;
  /** Older listing pages exist; false: one finite catalog page. */
  paged: boolean;
  /** Releases may be installable; false: they open in the browser. */
  torrents: boolean;
  /** Chosen for a new Store user. */
  defaultOn: boolean;
  /** Verified limitations, plain sentences. */
  notes: string[];
}

/** What discovery is doing. Mirrors discovery.Status. */
export interface DiscoveryStatus {
  /** The Store and source browsing are on and a source is chosen. */
  enabled: boolean;
  /** An existing Store user makes the one-time source choice first. */
  setupNeeded: boolean;
  sources: DiscoverySourceStatus[];
  games: number;
  releases: number;
  refreshing: boolean;
  /** The person paused indexing; nothing is requested until resumed. */
  paused: boolean;
  /** A game is running, so indexing waits for it to end. */
  playing: boolean;
  /** Newest listings older than six hours, or their last refresh failed. */
  stale: boolean;
}

/** One game on a shelf, in Browse or search. Mirrors discovery.GameSummary. */
export interface GameSummary {
  /** "steam:<appid>" or "title:<normalized title>", as CatalogEntry.key. */
  key: string;
  title: string;
  steamAppId?: number;
  /** A source release or feed offer exists; Steam-only results have none. */
  sourceBacked: boolean;
  /** fitgirl, dodi, feeds */
  sources: string[];
  releases: number;
  version?: string;
  /** Newest source publication (not the game's release date). */
  publishedAt: number;
  /** Newest change to a source release. */
  updatedAt: number;
  /** The game's own release date from Steam ("12 Mar, 2024"); never a source date. */
  releaseDate?: string;
  sizeBytes: number;
  /** Empty: unknown. */
  languages: string[];
  /** Empty: unknown. */
  genres: string[];
  installable: boolean;
  /** Place on Steam's most-played chart; 0 off the chart or unknown. */
  popularRank: number;
  reviewPercent: number;
  /** 0: unknown or no reviews. */
  reviewTotal: number;
  reviewLabel?: string;
  /** HowLongToBeat Main Story minutes from the cache; 0 unknown. */
  completionMain: number;
  installed?: { download: string; version: string; update: boolean };
  wishlisted: boolean;
  /** Unread wishlist activity. */
  activity: boolean;
}

export type BrowseSort = "title" | "published" | "popular" | "reviews";

/** Mirrors discovery.BrowseQuery. */
export interface BrowseQuery {
  text: string;
  /** fitgirl, dodi, feeds; empty: all. */
  sources: string[];
  language: string;
  genre: string;
  availability: "" | "installable" | "unresolved";
  installed: "" | "installed" | "not-installed";
  sort: BrowseSort;
  offset: number;
  /** 60 by default, at most 200. */
  limit: number;
}

/** Mirrors discovery.BrowsePage. */
export interface BrowsePage {
  games: GameSummary[];
  total: number;
  /** Left out only because the filtered field is unknown for them. */
  unknown: number;
  languages: string[];
  genres: string[];
}

/** One remote search. Mirrors discovery.ProviderProgress. */
export interface ProviderProgress {
  /** fitgirl, dodi, steam */
  id: string;
  name: string;
  state: ProviderState;
  found: number;
  error?: string;
  cached: boolean;
}

/** Mirrors discovery.SearchResult. */
export interface SearchResult {
  query: BrowseQuery;
  page: BrowsePage;
  /** Steam games with no known source release: wishlist only. */
  other: GameSummary[];
  remote: ProviderProgress[];
  complete: boolean;
}

/** Mirrors discovery.SearchProgress (the store:search event). */
export interface SearchProgress {
  text: string;
  remote: ProviderProgress[];
}

/** The Store's front page. Mirrors discovery.Home. */
export interface StoreHome {
  new: GameSummary[];
  popular: GameSummary[];
  /** "unavailable": no chart, the shelf shows its empty state. */
  popularState: ProviderState;
  updated: GameSummary[];
  wishlist: GameSummary[];
  /** A few source-backed games for the top of the page, never installed ones. */
  featured: GameSummary[];
  /** Games sharing genres with recently played or wishlisted ones; popular games without history. */
  recommended: Recommendation[];
  recommendedBasis: RecommendationBasis;
  status: DiscoveryStatus;
}

/** Mirrors the discovery.Basis* constants. */
export type RecommendationBasis = "" | "played" | "wishlist" | "played+wishlist" | "popular";

/** One recommended game and why. Mirrors discovery.Recommendation. */
export interface Recommendation {
  game: GameSummary;
  /** Played or wishlisted games it shares genres with, at most three; empty for "popular". */
  because: string[];
  /** The shared genres. */
  genres: string[];
}

/** One release choice on a game's page. Mirrors discovery.Release. */
export interface Release {
  /** The source entry ID, or "feed:<n>". */
  id: string;
  origin: "source" | "feed";
  /** fitgirl, dodi, or the feed's URL. */
  source: string;
  sourceName: string;
  title: string;
  rawTitle: string;
  version?: string;
  pageUrl?: string;
  publishedAt: number;
  updatedAt: number;
  sizeBytes: number;
  sizeClaim?: string;
  sizeIsMinimum?: boolean;
  installedSizeBytes?: number;
  /** Empty: unknown. */
  languages: string[];
  languageClaim?: string;
  kind: "release" | "update" | "preview";
  availability: ReleaseAvailability;
  /** Validated torrent identities. */
  transports: number;
  /** The source offers no torrents; its files open in a browser only. */
  browserOnly: boolean;
  /** Why mirrors aren't usable yet, one line each. */
  unresolved: string[];
  warnings: string[];
  /** A confirmed newer game version than the installed one. */
  newer: boolean;
  feedKey?: string;
  feedOffer: number;
}

/** How a game's Steam match was made. Mirrors discovery.Identity. */
export interface Identity {
  steamAppId: number;
  name?: string;
  how: "game-database" | "correction" | "feed" | "";
  corrected: boolean;
}

/** A game's page. Mirrors discovery.GameDetails. */
export interface GameDetails {
  summary: GameSummary;
  /** Newest publication first. */
  releases: Release[];
  /** Index in releases to offer first; -1 none. */
  recommended: number;
  why: string[];
  identity: Identity;
  /** Release details are loading; onGames brings them. */
  loading: boolean;
}

/** One validated way to download a prepared release. Mirrors discovery.PreparedOffer. */
export interface PreparedOffer {
  transport: number;
  title: string;
  version?: string;
  sizeBytes: number;
  installedSizeBytes?: number;
  languages: string[];
  torrentName?: string;
  infoHash: string;
  sourceName: string;
}

/** A release made ready for the install confirmation. Mirrors discovery.PreparedRelease. */
export interface PreparedRelease {
  gameKey: string;
  release: Release;
  ready: boolean;
  offers: PreparedOffer[];
  state: "ready" | "unresolved" | "update-only" | "unavailable" | "preview" | "browser";
  reason?: string;
  warnings: string[];
  /** The copy the Store installed, which an update replaces in its folder. */
  installed?: { version: string; dir: string };
}

/** Which games changed. Mirrors discovery.Change (the store:games event). */
export interface DiscoveryChange {
  keys: string[];
  all: boolean;
}

/** Mirrors enrich.Score. */
export interface ReviewScore {
  label?: string;
  percent: number;
  /** 0: no reviews or unknown. */
  total: number;
}

/** Mirrors enrich.ReviewSummary. */
export interface ReviewSummary {
  appId: number;
  overall: ReviewScore;
  /** Last 30 days. */
  recent: ReviewScore;
  url: string;
  fetchedAt: number;
  state: ProviderState;
  error?: string;
}

/** One Steam review, shown as plain text. Mirrors enrich.Review. */
export interface Review {
  id: string;
  author: string;
  recommended: boolean;
  text: string;
  language?: string;
  helpful: number;
  funny: number;
  /** Minutes; 0 unknown. */
  playtimeAtReview: number;
  playtimeForever: number;
  posted: number;
  url: string;
}

/** Mirrors enrich.ReviewQuery. */
export interface ReviewQuery {
  appId: number;
  /** "" for the first page, then ReviewPage.cursor. */
  cursor: string;
  filter: "helpful" | "recent";
  /** Steam language name ("english"); "" all. */
  language: string;
}

/** Mirrors enrich.ReviewPage. */
export interface ReviewPage {
  appId: number;
  reviews: Review[];
  cursor: string;
  more: boolean;
  state: ProviderState;
  error?: string;
}

/** Metacritic score and link as Steam gives them. Mirrors enrich.Critic. */
export interface Critic {
  appId: number;
  /** 0: none given. */
  score: number;
  url?: string;
  fetchedAt: number;
  state: ProviderState;
  error?: string;
}

/** HowLongToBeat times in minutes; 0 unknown. Mirrors enrich.Completion. */
export interface Completion {
  /** 0: no confident match. */
  hltbId: number;
  title?: string;
  main: number;
  mainExtras: number;
  completionist: number;
  /** The game's page, or a search on HowLongToBeat. */
  url: string;
  corrected: boolean;
  fetchedAt: number;
  state: ProviderState;
  error?: string;
}

/** A HowLongToBeat search result. Mirrors enrich.Candidate. */
export interface CompletionCandidate {
  hltbId: number;
  title: string;
  year: number;
  /** HowLongToBeat's kind of entry: game, dlc, mod, …; "" when not given. */
  type: string;
  main: number;
  mainExtras: number;
  completionist: number;
  url: string;
}

/** Everything known about a game besides its releases. Mirrors enrich.Enrichment. */
export interface Enrichment {
  key: string;
  steamAppId: number;
  reviews: ReviewSummary;
  critic: Critic;
  completion: Completion;
  popularRank: number;
}

/** Mirrors wishlist.ActivityView. */
export interface WishlistActivity {
  id: string;
  /** available: a first source release; newer: a confirmed newer version. */
  kind: "available" | "newer";
  releaseId: string;
  source: string;
  sourceName: string;
  version?: string;
  at: number;
  read: boolean;
}

/** A saved game. Mirrors internal/app.WishlistItem. */
export interface WishlistItem {
  key: string;
  title: string;
  steamAppId?: number;
  addedAt: number;
  game: GameSummary;
  /** Newest first. */
  activity: WishlistActivity[];
  unread: number;
  /** "": saved in Seaglass; "steam": imported from a Steam wishlist. */
  origin: "" | "steam";
}

/** The Steam account a wishlist import starts from. Mirrors app.SteamAccount. */
export interface SteamAccount {
  /** SteamID64; "" when none was found. */
  steamId: string;
  /** The account Steam signs in with on this PC. */
  detected: boolean;
  error?: string;
}

/** What importing a Steam wishlist did. Mirrors app.WishlistImport. */
export interface WishlistImport {
  steamId: string;
  /** Games on the Steam wishlist. */
  fetched: number;
  added: number;
  /** Already saved, matched by Steam AppID. */
  existing: number;
  /** With a known source release now. */
  available: number;
  /** Queued for a source search while idle. */
  searching: number;
  items: WishlistItem[];
}

/** A library game's HowLongToBeat times. Mirrors app.LibraryCompletion. */
export interface LibraryCompletion {
  gameId: number;
  /** "steam:<appid>" or "title:<normalized title>": shared with the Store, never the folder key. */
  key: string;
  completion: Completion;
}
