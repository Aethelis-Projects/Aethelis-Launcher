import type {
  InstanceDTO,
  CreateInstanceRequest,
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
} from "../bindings/ipc_types";

interface WailsAdapterBindings {
  LoginMicrosoft?: () => Promise<AccountDTO>;
  CheckForUpdates?: () => Promise<UpdateInfoDTO>;
  ApplyUpdate?: () => Promise<UpdateApplyResultDTO>;
  RestartApplication?: () => Promise<void>;
}

declare global {
  interface Window {
    go?: {
      wails?: {
        WailsAdapter?: WailsAdapterBindings;
      };
    };
  }
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

export const launcherAPI = {
  async listInstances(): Promise<InstanceDTO[]> {
    return [...mockInstances];
  },

  async createInstance(req: CreateInstanceRequest): Promise<InstanceDTO> {
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

  async launchInstance(id: string): Promise<LaunchResponse> {
    const inst = mockInstances.find((i) => i.id === id);
    if (!inst) {
      return { success: false, error: "Instance not found" };
    }
    inst.state = "running";
    return { success: true, pid: Math.floor(Math.random() * 8000) + 1000 };
  },

  async listAccounts(): Promise<AccountDTO[]> {
    return [...mockAccounts];
  },

  async setActiveAccount(uuid: string): Promise<void> {
    mockAccounts = mockAccounts.map((a) => ({
      ...a,
      is_active: a.uuid === uuid,
    }));
  },

  async loginOffline(username: string): Promise<AccountDTO> {
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

  async loginMicrosoft(): Promise<AccountDTO> {
    const fn = window.go?.wails?.WailsAdapter?.LoginMicrosoft;
    if (typeof fn === "function") {
      return await fn();
    }
    const newAcc: AccountDTO = {
      uuid: `ms-${Date.now()}`,
      username: "MicrosoftPlayer",
      type: "microsoft",
      is_active: true,
    };
    mockAccounts = mockAccounts.map((a) => ({ ...a, is_active: false }));
    mockAccounts.push(newAcc);
    return newAcc;
  },

  async searchMods(req: SearchModsRequest): Promise<ModItemDTO[]> {
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

  async listInstalledMods(instanceId: string): Promise<InstalledModDTO[]> {
    return [...(mockInstalledMods[instanceId] || [])];
  },

  async toggleMod(req: ToggleModRequest): Promise<void> {
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

  async deleteMod(req: DeleteModRequest): Promise<void> {
    const list = mockInstalledMods[req.instance_id] || [];
    mockInstalledMods[req.instance_id] = list.filter((m) => m.file_name !== req.file_name);
  },

  async installMod(instanceId: string, mod: ModItemDTO): Promise<void> {
    if (!mockInstalledMods[instanceId]) {
      mockInstalledMods[instanceId] = [];
    }
    mockInstalledMods[instanceId].push({
      file_name: `${mod.slug}-1.0.0.jar`,
      mod_id: mod.slug,
      name: mod.name,
      version: "1.0.0",
      enabled: true,
      size_bytes: 1572864,
    });
  },

  async getLastCrashReport(_instanceId: string): Promise<CrashReportDTO | null> {
    return null;
  },

  async checkForUpdates(): Promise<UpdateInfoDTO> {
    const fn = window.go?.wails?.WailsAdapter?.CheckForUpdates;
    if (typeof fn === "function") {
      return await fn();
    }
    return {
      has_update: false,
      version: "0.1.1",
      release_notes: "",
      download_url: "",
      sha256: "",
      size: 0,
    };
  },

  async applyUpdate(): Promise<UpdateApplyResultDTO> {
    const fn = window.go?.wails?.WailsAdapter?.ApplyUpdate;
    if (typeof fn === "function") {
      return await fn();
    }
    return {
      success: true,
      message: "Update applied successfully",
      restart_required: true,
    };
  },

  async restartApplication(): Promise<void> {
    const fn = window.go?.wails?.WailsAdapter?.RestartApplication;
    if (typeof fn === "function") {
      await fn();
    }
  },
};