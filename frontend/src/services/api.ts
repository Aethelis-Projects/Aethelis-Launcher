import type {
  InstanceDTO,
  CreateInstanceRequest,
  UpdateInstanceRequest,
  LaunchResponse,
  AccountDTO,
  ModItemDTO,
  InstalledModDTO,
  CrashReportDTO,
  SearchModsRequest,
  ToggleModRequest,
  DeleteModRequest,
  UpdateInfoDTO,
  UpdateApplyResultDTO,
  GetSettingsResponse,
  SetSettingRequest,
  InstallModRequest,
  InstallModResponse,
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
  LaunchInstance?: (id: string) => Promise<LaunchResponse>;
  ListAccounts?: () => Promise<AccountDTO[]>;
  SetActiveAccount?: (uuid: string) => Promise<void>;
  SearchMods?: (req: SearchModsRequest) => Promise<ModItemDTO[]>;
  ListInstalledMods?: (instanceId: string) => Promise<InstalledModDTO[]>;
  ToggleMod?: (req: ToggleModRequest) => Promise<void>;
  DeleteMod?: (req: DeleteModRequest) => Promise<void>;
  GetLastCrashReport?: (instanceId: string) => Promise<CrashReportDTO | null>;
  GetSettings?: () => Promise<GetSettingsResponse>;
  SetSetting?: (req: SetSettingRequest) => Promise<void>;
  InstallMod?: (req: InstallModRequest) => Promise<InstallModResponse>;
  HasBuiltinCurseForgeKey?: () => Promise<boolean>;
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
    state: "idle",
    total_play_seconds: 14200,
  },
  {
    id: "vanilla-pack",
    name: "Vanilla Exploration",
    game_version: "1.21.1",
    loader: "vanilla",
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
  version: "0.2.1",
  current_version: "0.2.1",
  release_date: "2026-09-19T18:00:00Z",
  release_notes: "Nord Launcher v0.2.1 (stable channel) release.",
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
      version: "0.2.1",
      current_version: "0.2.1",
      release_date: "2026-09-19T18:00:00Z",
      release_notes: "Nord Launcher v0.2.1 (stable channel) release.",
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
    return invokeWails("GetCurrentVersion", () => "0.2.1");
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
        inst.java_path = req.java_path;
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

  async searchMods(req: SearchModsRequest): Promise<ModItemDTO[]> {
    return invokeWails(
      "SearchMods",
      () => {
        const q = req.query.toLowerCase().trim();
        return mockModCatalog.filter((m) => {
          if (req.source && m.source !== req.source) return false;
          if (!q) return true;
          return (
            m.name.toLowerCase().includes(q) ||
            m.summary.toLowerCase().includes(q) ||
            m.slug.toLowerCase().includes(q)
          );
        });
      },
      req
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

  async installMod(instanceId: string, mod: ModItemDTO): Promise<InstallModResponse> {
    const res = await invokeWails<InstallModResponse>(
      "InstallMod",
      () => {
        if (!mockInstalledMods[instanceId]) {
          mockInstalledMods[instanceId] = [];
        }
        const fileName = `${mod.slug}-1.0.0.jar`;
        mockInstalledMods[instanceId].push({
          file_name: fileName,
          mod_id: mod.slug,
          name: mod.name,
          version: "1.0.0",
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
      } as InstallModRequest
    );
    if (!res.success) {
      throw new Error(res.message || "Failed to install mod");
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
};