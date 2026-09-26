import type {
  LoaderType,
  InstanceDTO,
  CreateInstanceRequest,
  UpdateInstanceRequest,
  LaunchResponse,
  AccountDTO,
  ModSource,
  ModItemDTO,
  InstalledModDTO,
  CrashReportDTO,
  SearchModsRequest,
  SearchModsResultDTO,
  ListModVersionsRequest,
  ModFileDTO,
  ModInstallProgressDTO,
  ToggleModRequest,
  DeleteModRequest,
  UpdateInfoDTO,
  UpdateApplyResultDTO,
  GetSettingsResponse,
  SetSettingRequest,
  InstallModRequest,
  InstallModResponse,
  UpdateModRequest,
  JavaInstallationDTO,
  JavaDownloadStatusDTO,
  ModUpdateItemDTO,
  MrPackImportPlanDTO,
  ImportMrPackRequest,
  MrPackImportStatusDTO,
  ExportMrPackRequest,
  JavaRuntimeUpdateDTO,
  SetFavoriteRequest,
  SetGroupRequest,
  ScreenshotDTO,
  DeleteScreenshotRequest,
  GetScreenshotDataRequest,
  GetScreenshotDataResponse,
} from "../bindings/ipc_types";

interface WailsAdapterBindings {
  GetCurrentVersion?: () => Promise<string>;
  CheckForUpdates?: () => Promise<UpdateInfoDTO>;
  ApplyUpdate?: () => Promise<UpdateApplyResultDTO>;
  RestartApplication?: () => Promise<void>;
  LoginMicrosoft?: () => Promise<AccountDTO>;
  LoginOffline?: (username: string) => Promise<AccountDTO>;
  ListInstances?: () => Promise<InstanceDTO[]>;
  CreateInstance?: (req: CreateInstanceRequest) => Promise<InstanceDTO>;
  UpdateInstance?: (req: UpdateInstanceRequest) => Promise<InstanceDTO>;
  SetInstanceFavorite?: (req: SetFavoriteRequest) => Promise<InstanceDTO>;
  SetInstanceGroup?: (req: SetGroupRequest) => Promise<InstanceDTO>;
  ListScreenshots?: (instanceId: string) => Promise<ScreenshotDTO[]>;
  DeleteScreenshot?: (req: DeleteScreenshotRequest) => Promise<void>;
  GetScreenshotData?: (req: GetScreenshotDataRequest) => Promise<GetScreenshotDataResponse>;
  LaunchInstance?: (id: string) => Promise<LaunchResponse>;
  ListAccounts?: () => Promise<AccountDTO[]>;
  SetActiveAccount?: (uuid: string) => Promise<void>;
  SearchMods?: (req: SearchModsRequest) => Promise<SearchModsResultDTO>;
  ListModVersions?: (req: ListModVersionsRequest) => Promise<ModFileDTO[]>;
  GetModInstallStatus?: (instanceId: string) => Promise<ModInstallProgressDTO>;
  ListInstalledMods?: (instanceId: string) => Promise<InstalledModDTO[]>;
  ToggleMod?: (req: ToggleModRequest) => Promise<void>;
  DeleteMod?: (req: DeleteModRequest) => Promise<void>;
  GetLastCrashReport?: (instanceId: string) => Promise<CrashReportDTO | null>;
  GetSettings?: () => Promise<GetSettingsResponse>;
  SetSetting?: (req: SetSettingRequest) => Promise<void>;
  InstallMod?: (req: InstallModRequest) => Promise<InstallModResponse>;
  UpdateMod?: (req: UpdateModRequest) => Promise<InstallModResponse>;
  HasBuiltinCurseForgeKey?: () => Promise<boolean>;
  GetLogTail?: (instanceId: string, n: number) => Promise<string[]>;
  ListJavaRuntimes?: () => Promise<JavaInstallationDTO[]>;
  DownloadJavaRuntime?: (major: number) => Promise<void>;
  GetJavaDownloadStatus?: () => Promise<JavaDownloadStatusDTO>;
  RemoveJavaRuntime?: (path: string) => Promise<void>;
  AddJavaRuntime?: (path: string) => Promise<JavaInstallationDTO>;
  CheckModUpdates?: (instanceId: string) => Promise<ModUpdateItemDTO[]>;
  GetDiagnosticReport?: (instanceId: string) => Promise<string>;
  PickMrPackFile?: () => Promise<string>;
  GetMrPackImportPlan?: (mrpackPath: string) => Promise<MrPackImportPlanDTO>;
  ImportMrPack?: (req: ImportMrPackRequest) => Promise<InstanceDTO>;
  GetMrPackImportStatus?: (instanceName: string) => Promise<MrPackImportStatusDTO>;
  ExportMrPack?: (req: ExportMrPackRequest) => Promise<string>;
  CheckJavaRuntimeUpdates?: () => Promise<JavaRuntimeUpdateDTO[]>;
  UpgradeJavaRuntime?: (major: number) => Promise<JavaInstallationDTO>;
  OpenPath?: (path: string) => Promise<void>;
  [key: string]: unknown;
}

interface WailsRuntimeCall {
  ByName: (methodName: string, ...args: unknown[]) => Promise<unknown>;
  ByID?: (methodID: number, ...args: unknown[]) => Promise<unknown>;
}

declare global {
  interface Window {
    go?: {
      wails?: {
        WailsAdapter?: WailsAdapterBindings;
      };
    };
    wails?: {
      Call?: WailsRuntimeCall;
      [key: string]: unknown;
    };
  }
}

const WAILS_ADAPTER_PREFIX =
  "github.com/nord-launcher/launcher/internal/adapters/wails.WailsAdapter";

let wailsCallPromise: Promise<WailsRuntimeCall | null> | null = null;

export async function getWailsCall(): Promise<WailsRuntimeCall | null> {
  if (typeof window === "undefined") {
    return null;
  }
  if (window.wails?.Call?.ByName) {
    return window.wails.Call;
  }
  if (!wailsCallPromise) {
    wailsCallPromise = (async () => {
      try {
        const dynamicImport = new Function("u", "return import(u)");
        const mod = await dynamicImport("/wails/runtime.js");
        if (mod?.Call?.ByName) {
          window.wails = { ...window.wails, ...mod };
          return mod.Call;
        }
      } catch {
        // Not running in Wails desktop webview (Vitest / pure browser / dev server)
      }
      return null;
    })();
  }
  return await wailsCallPromise;
}

/**
 * Detects whether the code is running in a development or unit-test environment.
 * Per review requirement D2: uses strictly import.meta.env to prevent fragile runtime process dependencies.
 */
function isDevEnvironment(): boolean {
  return Boolean(
    typeof import.meta !== "undefined" &&
      import.meta.env &&
      (import.meta.env.DEV || import.meta.env.MODE === "test")
  );
}

async function invokeWails<T>(
  methodName: string,
  fallback: () => Promise<T> | T,
  ...args: unknown[]
): Promise<T> {
  if (typeof window !== "undefined") {
    const fn = window.go?.wails?.WailsAdapter?.[methodName];
    if (typeof fn === "function") {
      return (await (fn as (...args: unknown[]) => Promise<T>)(...args));
    }
  }

  const call = await getWailsCall();
  if (call) {
    return (await call.ByName(`${WAILS_ADAPTER_PREFIX}.${methodName}`, ...args)) as T;
  }

  // P1 Blocker: Silent mocks are strictly forbidden in production.
  // Fallback to mock is permitted only in dev / test environments.
  if (isDevEnvironment()) {
    return await fallback();
  }

  throw new Error(
    `Wails IPC bridge unavailable for ${methodName}. Application is not connected to desktop runtime.`
  );
}

// Mock store for dev & Vitest environments
let mockInstances: InstanceDTO[] = [
  {
    id: "nord-opti-1",
    name: "Nordic Optimized 1.21",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
    min_ram_mb: 2048,
    max_ram_mb: 4096,
    jvm_args: [],
    skip_java_check: false,
    group: "",
    is_favorite: false,
    state: "idle",
    total_play_seconds: 14200,
  },
  {
    id: "vanilla-pack",
    name: "Vanilla Exploration",
    game_version: "1.21.1",
    loader: "vanilla",
    min_ram_mb: 2048,
    max_ram_mb: 4096,
    jvm_args: [],
    skip_java_check: false,
    group: "",
    is_favorite: false,
    state: "idle",
    total_play_seconds: 3600,
  },
];

let mockAccounts: AccountDTO[] = [
  {
    uuid: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    username: "NordHero",
    type: "microsoft",
    is_active: true,
  },
  {
    uuid: "offline-steve-123",
    username: "OfflineSteve",
    type: "offline",
    is_active: false,
  },
];

let mockScreenshots: Record<string, ScreenshotDTO[]> = {
  "nord-opti-1": [
    {
      file_name: "2026-09-20_15.30.00.png",
      path: "instances/nord-opti-1/screenshots/2026-09-20_15.30.00.png",
      size: 1048576,
      created_at: new Date().toISOString(),
    },
  ],
};

let mockInstalledMods: Record<string, InstalledModDTO[]> = {
  "nord-opti-1": [
    {
      file_name: "sodium-fabric-0.5.8.jar",
      mod_id: "sodium",
      name: "Sodium",
      version: "0.5.8",
      enabled: true,
      size_bytes: 1048576,
    },
    {
      file_name: "fabric-api-0.96.0.jar",
      mod_id: "fabric-api",
      name: "Fabric API",
      version: "0.96.0",
      enabled: true,
      size_bytes: 2097152,
    },
    {
      file_name: "iris-1.7.0.jar.disabled",
      mod_id: "iris",
      name: "Iris Shaders",
      version: "1.7.0",
      enabled: false,
      size_bytes: 3145728,
    },
  ],
};

const mockModCatalog: ModItemDTO[] = [
  {
    id: "AANobbMI",
    slug: "sodium",
    source: "modrinth",
    name: "Sodium",
    author: "jellysquid3",
    summary: "Modern rendering engine for Minecraft that greatly improves frame rates and reduces micro-stutter",
    icon_url: "https://cdn.modrinth.com/sodium.png",
    downloads: 18500000,
    categories: ["fabric", "optimization"],
  },
  {
    id: "YL57xq9U",
    slug: "iris",
    source: "modrinth",
    name: "Iris Shaders",
    author: "coderbot",
    summary: "A modern shaders mod for Minecraft compatible with existing Shaderspacks",
    icon_url: "https://cdn.modrinth.com/iris.png",
    downloads: 12400000,
    categories: ["fabric", "shaders"],
  },
  {
    id: "P7dR8mSH",
    slug: "fabric-api",
    source: "modrinth",
    name: "Fabric API",
    author: "modmuss50",
    summary: "Essential core library for the Fabric mod ecosystem",
    icon_url: "https://cdn.modrinth.com/fabric-api.png",
    downloads: 45000000,
    categories: ["fabric", "library"],
  },
  {
    id: "238222",
    slug: "jei",
    source: "curseforge",
    name: "Just Enough Items (JEI)",
    author: "mezz",
    summary: "View Items and Recipes directly from your inventory",
    icon_url: "https://edge.forgecdn.net/jei.png",
    downloads: 150000000,
    categories: ["fabric", "utility"],
  },
];

let mockUpdateInfo: UpdateInfoDTO = {
  has_update: false,
  version: "0.6.0",
  current_version: "0.6.0",
  release_date: "2026-09-24T12:00:00Z",
  release_notes: "Nord Launcher v0.6.0 (stable channel) release.",
  download_url: "",
  sha256: "",
  size: 0,
};

let mockApplyResult: UpdateApplyResultDTO = {
  success: true,
  message: "Update applied successfully. Restart required for changes to take effect.",
  restart_required: true,
};

let mockSettings: Record<string, string> = {};
let mockHasBuiltinCurseForgeKey = false;
let mockJavaRuntimes: JavaInstallationDTO[] = [
  {
    path: "C:\\Program Files\\Eclipse Adoptium\\jdk-21.0.2.13-hotspot\\bin\\java.exe",
    home_dir: "C:\\Program Files\\Eclipse Adoptium\\jdk-21.0.2.13-hotspot",
    major_version: 21,
    full_version: "21.0.2",
    vendor: "Eclipse Adoptium",
    kind: "managed",
    used_by: ["Nordic Optimized 1.21"],
  },
  {
    path: "C:\\Program Files\\Java\\jdk-17\\bin\\java.exe",
    home_dir: "C:\\Program Files\\Java\\jdk-17",
    major_version: 17,
    full_version: "17.0.10",
    vendor: "Oracle Corporation",
    kind: "detected",
    used_by: [],
  },
];
let mockJavaDownloadStatus: JavaDownloadStatusDTO = {
  task_id: "",
  major: 0,
  status: "idle",
  bytes_read: 0,
  total_bytes: 0,
  percentage: 0,
};

let mockMrPackPlan: MrPackImportPlanDTO = {
  name: "Nordic Optimized 1.21",
  summary: "Curated performance modpack for Minecraft 1.21.1",
  game_version: "1.21.1",
  loader: "fabric",
  loader_version: "0.16.5",
  total_files: 12,
  total_size: 14500000,
  dependencies: {},
};

let mockMrPackStatus: MrPackImportStatusDTO = {
  task_id: "import-mock-1",
  status: "complete",
  current_file: "",
  files_done: 12,
  total_files: 12,
  bytes_read: 14500000,
  total_bytes: 14500000,
  percentage: 100,
  error: "",
};

let mockJavaRuntimeUpdates: JavaRuntimeUpdateDTO[] = [
  {
    major_version: 21,
    current_version: "21.0.2",
    latest_version: "21.0.3",
    update_available: true,
  },
];

export const launcherAPI = {
  setMockUpdateInfo(info: UpdateInfoDTO): void {
    mockUpdateInfo = { ...info };
  },

  setMockApplyResult(res: UpdateApplyResultDTO): void {
    mockApplyResult = { ...res };
  },

  resetMockUpdater(): void {
    mockUpdateInfo = {
      has_update: false,
      version: "0.6.1",
      current_version: "0.6.1",
      release_date: "2026-09-25T12:00:00Z",
      release_notes: "Nord Launcher v0.6.1 (stable channel) release.",
      download_url: "",
      sha256: "",
      size: 0,
    };
    mockApplyResult = {
      success: true,
      message: "Update applied successfully. Restart required for changes to take effect.",
      restart_required: true,
    };
  },

  async getCurrentVersion(): Promise<string> {
    return invokeWails("GetCurrentVersion", () => "0.6.1");
  },

  async listInstances(): Promise<InstanceDTO[]> {
    return invokeWails("ListInstances", () => [...mockInstances]);
  },

  async createInstance(req: CreateInstanceRequest): Promise<InstanceDTO> {
    return invokeWails(
      "CreateInstance",
      () => {
        const newInst: InstanceDTO = {
          id: `${req.name.toLowerCase().replace(/\s+/g, "-")}-${Date.now()}`,
          name: req.name,
          game_version: req.game_version,
          loader: req.loader,
          min_ram_mb: 2048,
          max_ram_mb: 4096,
          jvm_args: [],
          skip_java_check: false,
          group: req.group || "",
          is_favorite: false,
          state: "idle",
          total_play_seconds: 0,
        };
        mockInstances.push(newInst);
        return newInst;
      },
      req
    );
  },

  async updateInstance(req: UpdateInstanceRequest): Promise<InstanceDTO> {
    return invokeWails(
      "UpdateInstance",
      () => {
        const inst = mockInstances.find((i) => i.id === req.id);
        if (!inst) {
          throw new Error("Instance not found");
        }
        if (req.name) inst.name = req.name;
        if (req.clear_java_path) {
          inst.java_path = undefined;
        } else if (req.java_path !== undefined) {
          inst.java_path = req.java_path;
        }
        if (req.min_ram_mb !== undefined) inst.min_ram_mb = req.min_ram_mb;
        if (req.max_ram_mb !== undefined) inst.max_ram_mb = req.max_ram_mb;
        if (req.jvm_args !== undefined) inst.jvm_args = req.jvm_args;
        if (req.skip_java_check !== undefined) inst.skip_java_check = req.skip_java_check;
        if (req.group !== undefined) inst.group = req.group;
        if (req.is_favorite !== undefined) inst.is_favorite = req.is_favorite;
        if (req.icon_path !== undefined) inst.icon_path = req.icon_path;
        return { ...inst };
      },
      req
    );
  },

  async setInstanceFavorite(req: SetFavoriteRequest): Promise<InstanceDTO> {
    return invokeWails(
      "SetInstanceFavorite",
      () => {
        const inst = mockInstances.find((i) => i.id === req.id);
        if (!inst) {
          throw new Error("Instance not found");
        }
        inst.is_favorite = req.is_favorite;
        return { ...inst };
      },
      req
    );
  },

  async setInstanceGroup(req: SetGroupRequest): Promise<InstanceDTO> {
    return invokeWails(
      "SetInstanceGroup",
      () => {
        const inst = mockInstances.find((i) => i.id === req.id);
        if (!inst) {
          throw new Error("Instance not found");
        }
        inst.group = req.group;
        return { ...inst };
      },
      req
    );
  },

  async launchInstance(id: string): Promise<LaunchResponse> {
    return invokeWails(
      "LaunchInstance",
      () => {
        const inst = mockInstances.find((i) => i.id === id);
        if (!inst) {
          return { success: false, error: "Instance not found" };
        }
        inst.state = "running";
        return { success: true, pid: Math.floor(Math.random() * 8000) + 1000 };
      },
      id
    );
  },

  async listAccounts(): Promise<AccountDTO[]> {
    return invokeWails("ListAccounts", () => [...mockAccounts]);
  },

  async setActiveAccount(uuid: string): Promise<void> {
    return invokeWails(
      "SetActiveAccount",
      () => {
        mockAccounts = mockAccounts.map((a) => ({
          ...a,
          is_active: a.uuid === uuid,
        }));
      },
      uuid
    );
  },

  async loginOffline(username: string): Promise<AccountDTO> {
    return invokeWails(
      "LoginOffline",
      () => {
        const newAcc: AccountDTO = {
          uuid: `offline-${Date.now()}`,
          username,
          type: "offline",
          is_active: true,
        };
        mockAccounts = mockAccounts.map((a) => ({ ...a, is_active: false }));
        mockAccounts.push(newAcc);
        return newAcc;
      },
      username
    );
  },

  async loginMicrosoft(): Promise<AccountDTO> {
    return invokeWails("LoginMicrosoft", () => {
      const newAcc: AccountDTO = {
        uuid: `ms-${Date.now()}`,
        username: "MicrosoftPlayer",
        type: "microsoft",
        is_active: true,
      };
      mockAccounts = mockAccounts.map((a) => ({ ...a, is_active: false }));
      mockAccounts.push(newAcc);
      return newAcc;
    });
  },

  async searchMods(req: SearchModsRequest): Promise<SearchModsResultDTO> {
    return invokeWails(
      "SearchMods",
      () => {
        const q = req.query.toLowerCase().trim();
        const cat = req.category ? req.category.toLowerCase().trim() : "";
        const filtered = mockModCatalog.filter((m) => {
          if (req.source && m.source !== req.source) return false;
          if (cat && !m.categories.some((c) => c.toLowerCase().includes(cat))) return false;
          if (!q) return true;
          return (
            m.name.toLowerCase().includes(q) ||
            m.summary.toLowerCase().includes(q) ||
            m.slug.toLowerCase().includes(q)
          );
        });
        return {
          items: filtered,
          total_count: filtered.length,
        };
      },
      req
    );
  },

  async listModVersions(req: ListModVersionsRequest): Promise<ModFileDTO[]> {
    return invokeWails<ModFileDTO[]>(
      "ListModVersions",
      () => [
        {
          id: `${req.mod_id}-v1`,
          mod_id: req.mod_id,
          file_name: `${req.mod_id}-1.0.0.jar`,
          display_name: `${req.mod_id} 1.0.0`,
          release_type: "release",
          file_size: 1048576,
          file_date: "2026-09-20T12:00:00Z",
          game_versions: ["1.21.1"],
          loaders: ["fabric"],
          download_url: "https://cdn.modrinth.com/mock.jar",
        },
      ],
      req
    );
  },

  async getModInstallStatus(instanceId: string): Promise<ModInstallProgressDTO> {
    return invokeWails<ModInstallProgressDTO>(
      "GetModInstallStatus",
      () => ({
        task_id: `install-${instanceId}-mock`,
        instance_id: instanceId,
        mod_id: "mock",
        file_name: "mock.jar",
        status: "idle",
        bytes_read: 0,
        total_bytes: 0,
        percentage: 0,
      }),
      instanceId
    );
  },

  async listInstalledMods(instanceId: string): Promise<InstalledModDTO[]> {
    return invokeWails(
      "ListInstalledMods",
      () => [...(mockInstalledMods[instanceId] || [])],
      instanceId
    );
  },

  async toggleMod(req: ToggleModRequest): Promise<void> {
    return invokeWails(
      "ToggleMod",
      () => {
        const list = mockInstalledMods[req.instance_id] || [];
        const target = list.find((m) => m.file_name === req.file_name);
        if (target) {
          target.enabled = req.enable;
          if (req.enable && target.file_name.endsWith(".disabled")) {
            target.file_name = target.file_name.replace(/\.disabled$/, "");
          } else if (!req.enable && !target.file_name.endsWith(".disabled")) {
            target.file_name = `${target.file_name}.disabled`;
          }
        }
      },
      req
    );
  },

  async deleteMod(req: DeleteModRequest): Promise<void> {
    return invokeWails(
      "DeleteMod",
      () => {
        const list = mockInstalledMods[req.instance_id] || [];
        mockInstalledMods[req.instance_id] = list.filter(
          (m) => m.file_name !== req.file_name
        );
      },
      req
    );
  },

  async installMod(
    instanceId: string,
    mod: ModItemDTO | { id: string; source: ModSource; name?: string; slug?: string },
    versionId?: string
  ): Promise<InstallModResponse> {
    const modSlug = ("slug" in mod && mod.slug) ? mod.slug : mod.id;
    const modName = ("name" in mod && mod.name) ? mod.name : mod.id;
    const res = await invokeWails<InstallModResponse>(
      "InstallMod",
      () => {
        if (!mockInstalledMods[instanceId]) {
          mockInstalledMods[instanceId] = [];
        }
        const fileName = `${modSlug}-1.0.0.jar`;
        mockInstalledMods[instanceId].push({
          file_name: fileName,
          mod_id: modSlug,
          name: modName,
          version: versionId || "1.0.0",
          enabled: true,
          size_bytes: 1572864,
        });
        return {
          success: true,
          file_name: fileName,
          message: `Mod ${fileName} installed successfully`,
        };
      },
      {
        instance_id: instanceId,
        mod_id: mod.id,
        source: mod.source,
        version_id: versionId,
      } as InstallModRequest
    );
    if (!res.success) {
      throw new Error(res.message || "Failed to install mod");
    }
    return res;
  },

  async updateMod(req: UpdateModRequest): Promise<InstallModResponse> {
    const res = await invokeWails<InstallModResponse>(
      "UpdateMod",
      () => {
        const instMods = mockInstalledMods[req.instance_id] || [];
        const oldIdx = instMods.findIndex((m) => m.file_name === req.old_file_name);
        const newFileName = `${req.mod_id}-${req.target_version_id || "latest"}.jar`;
        if (oldIdx !== -1) {
          instMods.splice(oldIdx, 1);
        }
        instMods.push({
          file_name: newFileName,
          mod_id: req.mod_id,
          name: req.mod_id,
          version: req.target_version_id,
          enabled: true,
          size_bytes: 1572864,
        });
        mockInstalledMods[req.instance_id] = instMods;
        return {
          success: true,
          file_name: newFileName,
          message: `Mod ${newFileName} updated successfully`,
        };
      },
      req
    );
    if (!res.success) {
      throw new Error(res.message || "Failed to update mod");
    }
    return res;
  },

  async getLastCrashReport(instanceId: string): Promise<CrashReportDTO | null> {
    return invokeWails("GetLastCrashReport", () => null, instanceId);
  },

  async checkForUpdates(): Promise<UpdateInfoDTO> {
    return invokeWails("CheckForUpdates", () => ({ ...mockUpdateInfo }));
  },

  async applyUpdate(): Promise<UpdateApplyResultDTO> {
    return invokeWails("ApplyUpdate", () => ({ ...mockApplyResult }));
  },

  async restartApplication(): Promise<void> {
    return invokeWails("RestartApplication", () => {});
  },

  setMockSettings(settings: Record<string, string>): void {
    mockSettings = { ...settings };
  },

  async getSettings(): Promise<GetSettingsResponse> {
    return invokeWails("GetSettings", () => ({ settings: { ...mockSettings } }));
  },

  async setSetting(key: string, value: string): Promise<void> {
    return invokeWails(
      "SetSetting",
      () => {
        mockSettings[key] = value;
      },
      { key, value }
    );
  },

  setMockHasBuiltinCurseForgeKey(val: boolean): void {
    mockHasBuiltinCurseForgeKey = val;
  },

  async hasBuiltinCurseForgeKey(): Promise<boolean> {
    return invokeWails("HasBuiltinCurseForgeKey", () => mockHasBuiltinCurseForgeKey);
  },

  async getLogTail(instanceId: string, n = 100): Promise<string[]> {
    return invokeWails<string[]>("GetLogTail", () => [], instanceId, n);
  },

  async listJavaRuntimes(): Promise<JavaInstallationDTO[]> {
    return invokeWails<JavaInstallationDTO[]>("ListJavaRuntimes", () => [...mockJavaRuntimes]);
  },

  async downloadJavaRuntime(major: number): Promise<void> {
    return invokeWails<void>(
      "DownloadJavaRuntime",
      () => {
        mockJavaDownloadStatus = {
          task_id: `adoptium-${major}-mock`,
          major,
          status: "downloading",
          bytes_read: 1024,
          total_bytes: 2048,
          percentage: 50,
        };
      },
      major
    );
  },

  async getJavaDownloadStatus(): Promise<JavaDownloadStatusDTO> {
    return invokeWails<JavaDownloadStatusDTO>("GetJavaDownloadStatus", () => ({ ...mockJavaDownloadStatus }));
  },

  async removeJavaRuntime(path: string): Promise<void> {
    return invokeWails<void>(
      "RemoveJavaRuntime",
      () => {
        mockJavaRuntimes = mockJavaRuntimes.filter((r) => r.path !== path);
      },
      path
    );
  },

  async addJavaRuntime(path: string): Promise<JavaInstallationDTO> {
    return invokeWails<JavaInstallationDTO>(
      "AddJavaRuntime",
      () => {
        const added: JavaInstallationDTO = {
          path,
          home_dir: path,
          major_version: 21,
          full_version: "21.0.2",
          vendor: "Custom",
          kind: "detected",
          used_by: [],
        };
        mockJavaRuntimes.push(added);
        return added;
      },
      path
    );
  },

  async checkModUpdates(instanceId: string): Promise<ModUpdateItemDTO[]> {
    return invokeWails<ModUpdateItemDTO[]>(
      "CheckModUpdates",
      () => [],
      instanceId
    );
  },

  async getDiagnosticReport(instanceId: string): Promise<string> {
    return invokeWails<string>(
      "GetDiagnosticReport",
      () => "=== Nord Launcher Diagnostic Report (Mock) ===\nLauncher Version: 0.6.1",
      instanceId
    );
  },

  setMockJavaRuntimes(runtimes: JavaInstallationDTO[]): void {
    mockJavaRuntimes = [...runtimes];
  },

  setMockJavaDownloadStatus(status: JavaDownloadStatusDTO): void {
    mockJavaDownloadStatus = { ...status };
  },

  setMockInstalledMods(instanceId: string, mods: InstalledModDTO[]): void {
    mockInstalledMods[instanceId] = [...mods];
  },

  async pickMrPackFile(): Promise<string> {
    return invokeWails<string>("PickMrPackFile", () => "C:\\Downloads\\Nordic-Optimized.mrpack");
  },

  async getMrPackImportPlan(mrpackPath: string): Promise<MrPackImportPlanDTO> {
    return invokeWails<MrPackImportPlanDTO>(
      "GetMrPackImportPlan",
      () => ({ ...mockMrPackPlan }),
      mrpackPath
    );
  },

  async importMrPack(req: ImportMrPackRequest): Promise<InstanceDTO> {
    return invokeWails<InstanceDTO>(
      "ImportMrPack",
      () => {
        const newInst: InstanceDTO = {
          id: req.instance_name.toLowerCase().replace(/[^a-z0-9_-]/g, "-"),
          name: req.instance_name,
          game_version: mockMrPackPlan.game_version,
          loader: (mockMrPackPlan.loader as LoaderType) || "fabric",
          loader_version: mockMrPackPlan.loader_version,
          min_ram_mb: 2048,
          max_ram_mb: 4096,
          jvm_args: [],
          skip_java_check: false,
          state: "idle",
          total_play_seconds: 0,
        };
        mockInstances.push(newInst);
        return newInst;
      },
      req
    );
  },

  async getMrPackImportStatus(instanceName: string): Promise<MrPackImportStatusDTO> {
    return invokeWails<MrPackImportStatusDTO>(
      "GetMrPackImportStatus",
      () => ({ ...mockMrPackStatus }),
      instanceName
    );
  },

  async exportMrPack(req: ExportMrPackRequest): Promise<string> {
    return invokeWails<string>(
      "ExportMrPack",
      () => `C:\\Exports\\${req.name}.mrpack`,
      req
    );
  },

  async checkJavaRuntimeUpdates(): Promise<JavaRuntimeUpdateDTO[]> {
    return invokeWails<JavaRuntimeUpdateDTO[]>(
      "CheckJavaRuntimeUpdates",
      () => [...mockJavaRuntimeUpdates]
    );
  },

  async upgradeJavaRuntime(major: number): Promise<JavaInstallationDTO> {
    return invokeWails<JavaInstallationDTO>(
      "UpgradeJavaRuntime",
      () => {
        const found = mockJavaRuntimes.find((r) => r.major_version === major);
        if (found) {
          found.full_version = `${major}.0.3`;
          return { ...found };
        }
        return {
          path: `C:\\Nord\\runtimes\\adoptium-${major}-${major}.0.3\\bin\\java.exe`,
          home_dir: `C:\\Nord\\runtimes\\adoptium-${major}-${major}.0.3`,
          major_version: major,
          full_version: `${major}.0.3`,
          vendor: "Eclipse Adoptium",
          kind: "managed",
          used_by: [],
        };
      },
      major
    );
  },

  async openPath(path: string): Promise<void> {
    return invokeWails<void>(
      "OpenPath",
      () => Promise.resolve(),
      path
    );
  },

  async listScreenshots(instanceId: string): Promise<ScreenshotDTO[]> {
    return invokeWails<ScreenshotDTO[]>(
      "ListScreenshots",
      () => [...(mockScreenshots[instanceId] || [])],
      instanceId
    );
  },

  async deleteScreenshot(req: DeleteScreenshotRequest): Promise<void> {
    return invokeWails<void>(
      "DeleteScreenshot",
      () => {
        if (mockScreenshots[req.instance_id]) {
          mockScreenshots[req.instance_id] = mockScreenshots[req.instance_id].filter(
            (s) => s.file_name !== req.file_name
          );
        }
      },
      req
    );
  },

  async getScreenshotData(req: GetScreenshotDataRequest): Promise<GetScreenshotDataResponse> {
    return invokeWails<GetScreenshotDataResponse>(
      "GetScreenshotData",
      () => ({
        data_url: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
      }),
      req
    );
  },

  setMockScreenshots(instanceId: string, list: ScreenshotDTO[]): void {
    mockScreenshots[instanceId] = [...list];
  },

  setMockMrPackPlan(plan: MrPackImportPlanDTO): void {
    mockMrPackPlan = { ...plan };
  },

  setMockMrPackStatus(status: MrPackImportStatusDTO): void {
    mockMrPackStatus = { ...status };
  },

  setMockJavaRuntimeUpdates(updates: JavaRuntimeUpdateDTO[]): void {
    mockJavaRuntimeUpdates = [...updates];
  },
};