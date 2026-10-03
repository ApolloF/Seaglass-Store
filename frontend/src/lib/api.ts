// The frontend's one door to the Go side. In mock mode (`npm run dev:mock`)
// the same interface is served by made-up data, so the interface can be
// built and checked in a normal browser. Vite drops the unused one.
import type { BrowseQuery, CompletionCandidate, LibraryCompletion, SteamAccount, WishlistImport, DiscoveryChange, DiscoveryStatus, Enrichment, GameDetails, PreparedRelease, ReviewPage, ReviewQuery, SearchProgress, SearchResult, StoreHome, WishlistItem, SourceSnapshot, SourceRelease, DownloadLanguageOptions, Accounts, Achievements, AppInfo, ArtChoice, ArtKind, CatalogEntry, CatalogPage, CatalogQuery, Download, InstallOptions, StoreArt, DownloadAction, EngineStatus, FeedInfo, Game, MetaState, PadRaw, PadState, Profile, Saves, ScanState, Session, SessionAchievements, Settings, Startup, StoreHit, SyncerStatus, TorrentInterface, UpdateState } from "./types";
import { realApi } from "./api.real";
import { mockApi } from "./api.mock";

export interface Api {
  games(): Promise<Game[]>;
  scanState(): Promise<ScanState>;
  rescan(): Promise<void>;
  setFavorite(id: number, on: boolean): Promise<Game>;
  setHidden(id: number, on: boolean): Promise<Game>;
  setPadMode(id: number, mode: string): Promise<Game>;
  rename(id: number, title: string): Promise<Game>;
  confirmMatch(id: number): Promise<Game>;
  chooseExe(id: number): Promise<Game>;
  openFolder(id: number): Promise<void>;
  /** Asks the game's store to install it. */
  install(id: number): Promise<void>;
  metaState(): Promise<MetaState>;
  refreshMetadata(id: number): Promise<void>;
  searchSteam(query: string): Promise<StoreHit[]>;
  setMatch(id: number, appId: number, name: string): Promise<Game>;
  /** Pictures the game's art of a kind could be, current first (asks the stores: seconds). */
  artChoices(id: number, kind: ArtKind): Promise<ArtChoice[]>;
  /** Makes a picture from artChoices the game's art, kept through refreshes. */
  setArt(id: number, kind: ArtKind, art: string): Promise<Game>;
  /** Puts the game in these collections (and out of the rest). */
  setCollections(id: number, names: string[]): Promise<Game>;
  /** Renames a collection in every game; "" deletes it (the games stay). */
  renameCollection(old: string, name: string): Promise<void>;

  /** HowLongToBeat times for library games, from any source; works with the Store off. */
  completion: {
    /** Cached times at once; with fetch, missing or week-old times are fetched first. */
    get(id: number, fetch: boolean): Promise<LibraryCompletion>;
    /** Other possible matches; "" searches the game's title. */
    candidates(id: number, query: string): Promise<CompletionCandidate[]>;
    /** Shared with the Store's page for the same game; 0 goes back to the automatic match. */
    setMatch(id: number, hltbId: number): Promise<LibraryCompletion>;
    /** Opens a HowLongToBeat link in the browser. */
    openLink(url: string): Promise<void>;
  };

  settings(): Promise<Settings>;
  saveSettings(s: Settings): Promise<Settings>;
  /** Settings saved on another PC were taken. */
  onSettingsChanged(cb: (s: Settings) => void): () => void;
  addFolder(): Promise<Settings>;
  removeFolder(path: string): Promise<Settings>;
  autoFolders(): Promise<string[]>;
  info(): Promise<AppInfo>;
  openLog(): Promise<void>;
  /** Puts a diagnostics report (no keys, user folder shortened) on the clipboard. */
  copyDiagnostics(): Promise<void>;
  /** Opens a new GitHub issue. */
  reportProblem(): Promise<void>;
  /** Logs an error the interface ran into. */
  reportUIError(message: string): void;
  hasSteamGridDBKey(): Promise<boolean>;
  setSteamGridDBKey(key: string): Promise<void>;
  startWithWindows(): Promise<Startup>;
  setStartWithWindows(on: boolean): Promise<Startup>;

  onLibraryChanged(cb: () => void): () => void;
  /** Some games changed (metadata, a favorite, playtime); the rest didn't. */
  onGamesUpdated(cb: (games: Game[]) => void): () => void;
  onScanState(cb: (s: ScanState) => void): () => void;
  onMetaState(cb: (s: MetaState) => void): () => void;

  updates: {
    state(): Promise<UpdateState>;
    /** Looks for a new version now (and downloads it). */
    check(): void;
    /** Installs the downloaded update and restarts Seaglass. */
    install(): Promise<void>;
    openReleasePage(): Promise<void>;
    onState(cb: (s: UpdateState) => void): () => void;
  };

  saves: {
    /** Syncer's view of a game's saves; fresh skips the short cache. */
    get(id: number, fresh?: boolean): Promise<Saves>;
    openSyncer(): Promise<void>;
    /** Downloads Syncer's latest release and installs it (or updates it). */
    installSyncer(): Promise<void>;
    /** Opens Syncer's home page. */
    syncerProject(): Promise<void>;
    /** Syncer's state; start starts it (without its window) when it isn't running. */
    syncer(start: boolean): Promise<SyncerStatus>;
  };

  /** Who's playing on this PC (Syncer's accounts). */
  profile: {
    /** fresh asks Syncer first (without starting it). */
    get(fresh?: boolean): Promise<Profile>;
    /** Puts that account's saves in place and shows their playtime, achievements and settings. */
    switch(id: string): Promise<Profile>;
    onChange(cb: (p: Profile) => void): () => void;
  };

  achievements: {
    /** A game's achievements; fresh reads them again (else a result from unchanged files is reused). */
    get(id: number, fresh?: boolean): Promise<Achievements>;
    /** Turns achievements on in a Uplay emulator's ini (fix "uplay-ini") and reads them again. */
    enableUplay(id: number): Promise<Achievements>;
    /** A play session unlocked achievements (after the game exited). */
    onSession(cb: (s: SessionAchievements) => void): () => void;
  };

  accounts: {
    get(): Promise<Accounts>;
    sync(): void;
    setSteamKey(key: string): Promise<Accounts>;
    openSteamKeyPage(): Promise<void>;
    setGOG(on: boolean): Promise<Accounts>;
    openEpicSignIn(): Promise<void>;
    epicSignIn(pasted: string): Promise<Accounts>;
    epicSignOut(): Promise<Accounts>;
    /** GOG sign-in, for achievements. */
    openGOGSignIn(): Promise<void>;
    gogSignIn(pasted: string): Promise<Accounts>;
    gogSignOut(): Promise<Accounts>;
    onChange(cb: (a: Accounts) => void): () => void;
  };

  launch: {
    play(id: number): Promise<void>;
    session(): Promise<Session>;
    skip(stepId: string): void;
    answer(questionId: number, option: string): void;
    cancel(): void;
    quitGame(): Promise<void>;
    setUIMode(mode: "desktop" | "bigpicture"): void;
    closeOverlay(): void;
    openMain(): void;
    onSession(cb: (s: Session) => void): () => void;
    /** Controller actions while the in-game overlay shows. */
    onOverlayAction(cb: (action: string, repeat: boolean) => void): () => void;
    /** The tray asks for a mode. */
    onUIMode(cb: (mode: "desktop" | "bigpicture") => void): () => void;
  };

  /** The experimental store. Everything fails while it's turned off. */
  store: {
    engine(): Promise<EngineStatus>;
    /** Starts the download engine now (it stops again when idle). */
    startEngine(): Promise<EngineStatus>;
    /** Network interfaces downloads can be bound to (starts the engine). */
    interfaces(): Promise<TorrentInterface[]>;
    addresses(iface: string): Promise<string[]>;
    hasProxyPassword(): Promise<boolean>;
    /** "" removes it. */
    setProxyPassword(password: string): Promise<void>;
    chooseQBittorrent(): Promise<Settings>;
    /** Opens qBittorrent's download page. */
    getQBittorrent(): Promise<void>;
    downloadsFolder(): Promise<string>;
    chooseDownloadsFolder(): Promise<Settings>;
    downloads(): Promise<Download[]>;
    downloadLanguages(id: string): Promise<DownloadLanguageOptions>;
    setDownloadLanguages(id: string, language: string, setupLanguage: string, ask: boolean): Promise<Download>;
    discoverReleases(source: string, query: string, resolve: boolean): Promise<SourceSnapshot>;
    attachReleaseTorrent(source: string, id: string): Promise<SourceRelease>;
    openReleasePage(source: string, id: string): Promise<void>;
    reviewRelease(source: string, id: string, transport: number): Promise<string>;
    /** Queues a magnet link or a link to a .torrent file; title "" uses the link's own name. */
    addDownload(source: string, title: string): Promise<Download>;
    action(id: string, action: DownloadAction, deleteFiles: boolean): Promise<void>;
    showDownload(id: string): Promise<void>;
    onDownloads(cb: (d: Download[]) => void): () => void;
    onEngine(cb: (s: EngineStatus) => void): () => void;

    feeds(): Promise<FeedInfo[]>;
    /** Fetches the feed first: only one Seaglass can read is added. */
    addFeed(url: string): Promise<Settings>;
    removeFeed(url: string): Promise<Settings>;
    setFeedEnabled(url: string, on: boolean): Promise<Settings>;
    /** Fetches every enabled feed now. */
    refreshFeeds(): Promise<FeedInfo[]>;
    catalog(q: CatalogQuery): Promise<CatalogPage>;
    catalogLanguages(): Promise<string[]>;
    catalogEntry(key: string): Promise<CatalogEntry>;
    /** Queues one of a game's offers, by its place in entry.offers. */
    downloadOffer(key: string, offer: number, opts: InstallOptions): Promise<Download>;
    /** A suggested folder for a game. */
    installFolder(title: string): Promise<string>;
    /** Asks for a game's folder; returns current when cancelled. */
    chooseInstallFolder(current: string): Promise<string>;
    gamesFolder(): Promise<string>;
    chooseGamesFolder(): Promise<Settings>;
    /** Known art for these games; the rest is looked up and arrives through onArt. */
    art(keys: string[]): Promise<StoreArt[]>;
    /** Installs a blocked download after all; confirm is its title, typed by the person. */
    allowDownload(id: string, confirm: string): Promise<void>;
    hasVirusTotalKey(): Promise<boolean>;
    /** "" removes it. Only file hashes are looked up. */
    setVirusTotalKey(key: string): Promise<void>;
    sandboxAvailable(): Promise<boolean>;
    /** Installed catalog games with a newer version. */
    updates(): Promise<CatalogEntry[]>;
    /** Pauses every download until resumed (as the tray does). */
    holdDownloads(on: boolean): Promise<void>;
    /** Opens Windows Sandbox with the download on its desktop (read-only, no network). */
    openInSandbox(id: string): Promise<void>;
    onArt(cb: (a: StoreArt) => void): () => void;
    /** The catalog changed (its number of games). */
    onCatalog(cb: (games: number) => void): () => void;

    /** Automatic discovery of source releases. Indexing reads public metadata only. */
    discovery: {
      status(): Promise<DiscoveryStatus>;
      /** The one-time source choice (or a later change): saves it and starts indexing. */
      setupSources(sources: string[]): Promise<Settings>;
      /** Fetches the newest listings of every chosen source now. */
      refresh(): Promise<DiscoveryStatus>;
      /** Pauses or resumes background indexing on this PC. */
      pauseIndexing(paused: boolean): Promise<DiscoveryStatus>;
      home(): Promise<StoreHome>;
      /** Answers from the local index at once. */
      browse(q: BrowseQuery): Promise<SearchResult>;
      /** Searches the source sites and Steam to fill gaps; a newer call cancels an older one. */
      search(q: BrowseQuery): Promise<SearchResult>;
      /** A game's page; missing release details arrive through onGames. */
      game(key: string): Promise<GameDetails>;
      /** appId 0: not on Steam. The key may change: use the returned one. */
      setSteamMatch(key: string, appId: number, name: string): Promise<GameDetails>;
      /** Fetches the article and resolves torrent metadata when it can; never downloads a game. */
      prepareRelease(key: string, releaseId: string): Promise<PreparedRelease>;
      /** Asks for a .torrent file got in the browser and validates it. */
      attachTorrent(key: string, releaseId: string): Promise<PreparedRelease>;
      /** Opens the release's article in the browser. */
      openRelease(key: string, releaseId: string): Promise<void>;
      /** Queues a prepared release's transport after the person confirmed it. */
      downloadRelease(key: string, releaseId: string, transport: number, opts: InstallOptions): Promise<Download>;
      onStatus(cb: (s: DiscoveryStatus) => void): () => void;
      onGames(cb: (c: DiscoveryChange) => void): () => void;
      onSearch(cb: (p: SearchProgress) => void): () => void;
    };

    /** Steam reviews, popularity, Metacritic and HowLongToBeat, cached; failures say so. */
    enrich: {
      /** Cached enrichment for cards on screen; the rest arrives through onEnrichment. */
      games(keys: string[]): Promise<Enrichment[]>;
      /** Everything for a game's page, fetched first when missing or old. */
      game(key: string): Promise<Enrichment>;
      reviews(q: ReviewQuery): Promise<ReviewPage>;
      completionCandidates(key: string, title: string): Promise<CompletionCandidate[]>;
      /** 0 goes back to the automatic match. */
      setCompletionMatch(key: string, hltbId: number): Promise<Enrichment>;
      /** Opens an attribution link (Steam, Metacritic, HowLongToBeat) in the browser. */
      openLink(url: string): Promise<void>;
      onEnrichment(cb: (e: Enrichment) => void): () => void;
    };

    /** Saved games, on this PC. */
    wishlist: {
      list(): Promise<WishlistItem[]>;
      add(key: string, title: string, steamAppId: number): Promise<WishlistItem[]>;
      remove(key: string): Promise<WishlistItem[]>;
      /** Marks a game's activity read; "" marks all. */
      acknowledge(key: string): Promise<WishlistItem[]>;
      /** The Steam account signed in on this PC, to prefill the import. */
      steamAccount(): Promise<SteamAccount>;
      /** Imports a public Steam wishlist (SteamID64); merges by AppID and never removes a saved game. */
      importSteam(steamId: string): Promise<WishlistImport>;
      onChange(cb: (items: WishlistItem[]) => void): () => void;
    };
  };

  window: {
    minimise(): void;
    toggleMaximise(): void;
    close(): void;
    fullscreen(on: boolean): void;
  };

  pad: {
    state(): Promise<PadState>;
    rumble(effect: "tick" | "bump" | "confirm" | "error" | "launch"): void;
    setLight(hex: string): void;
    onAction(cb: (action: string, repeat: boolean) => void): () => void;
    onState(cb: (s: PadState) => void): () => void;
    /** Streams every button and axis (onRaw) while on, for the test screen. */
    testInput(on: boolean): void;
    onRaw(cb: (r: PadRaw) => void): () => void;
  };
}

export const api: Api = import.meta.env.VITE_MOCK === "1" ? mockApi : realApi;
