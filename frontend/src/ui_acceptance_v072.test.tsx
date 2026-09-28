// v0.7.2 UI acceptance walk (jsdom, full App, API surface stubbed per-method).
// This complements (not replaces) component unit tests: it exercises the real
// App wiring - nav, unified import tabs, modpacks handoff, creation wizard and
// the instance settings performance tab - through the same code path the
// webview uses (invokeWails -> launcherAPI).
import { render, fireEvent, screen, waitFor, cleanup } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { App } from "./App";
import { launcherAPI } from "./services/api";
import type { InstanceDTO, ModItemDTO } from "./bindings/ipc_types";

const instances: InstanceDTO[] = [
  {
    id: "default-fabric-1-21",
    name: "Nordic Fabric 1.21",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
    min_ram_mb: 2048,
    max_ram_mb: 4096,
    jvm_args: [],
    skip_java_check: false,
    state: "idle",
    total_play_seconds: 120,
    group: "Основное",
  },
];

const pack: ModItemDTO = {
  id: "proj1",
  slug: "fabric-skyblocks",
  source: "modrinth",
  name: "Fabric Skyblocks",
  author: "mahomax",
  summary: "skyblocks but fabric",
  downloads: 2500000,
  categories: ["fabric"],
  project_type: "modpack",
};

const mcVersions = [
  { id: "26.3", type: "release", release_time: "2026-09-01T00:00:00Z" },
  { id: "1.21.4", type: "release", release_time: "2024-12-03T00:00:00Z" },
];

function stubLauncherAPI() {
  const defaults: Record<string, unknown> = {
    listInstances: instances,
    checkForUpdates: {
      has_update: false,
      version: "",
      current_version: "",
      release_date: "",
      release_notes: "",
      download_url: "",
      sha256: "",
      size: 0,
    },
    listAccounts: [],
    searchMods: { items: [pack], total_count: 1 },
    listMinecraftVersions: mcVersions,
    listLoaderVersions: { loader: "fabric", default: "0.16.4", options: ["0.16.9", "0.16.4"], source: "fabric" },
    listProjectTags: [
      { id: "fabric", label: "Fabric", searchable: true },
      { id: "modpacks", label: "Modpacks", searchable: true },
    ],
    listMrPackVersions: [
      {
        version_id: "vXYZ",
        name: "Fabric Skyblocks 2.6.0",
        version_type: "release",
        game_version: "1.21.4",
        loaders: ["fabric"],
        url: "https://cdn.modrinth.com/p.mrpack",
        filename: "pack.mrpack",
        size: 12000000,
        sha1: "aa",
      },
    ],
    importMrPackFromURL: "inst-1",
    getMrPackURLImportStatus: {
      task_id: "x",
      status: "complete",
      current_file: "",
      files_done: 1,
      total_files: 1,
      bytes_read: 1,
      total_bytes: 1,
      percentage: 100,
    },
    createInstanceWithLoader: {
      ...instances[0],
      id: "wizard-inst",
      name: "wizard pack",
      loader_version: "0.16.4",
    },
    getDiscordRpcStatus: {
      enabled: false,
      connected: false,
      app_id_set: true,
      has_activity: false,
      last_error: "",
    },
    listJavaRuntimes: [],
    getPerformancePreset: { suggested_ram_mb: 3072, aikar_args: [] },
    checkInstanceFiles: {
      version: "0.7.2",
      checked_count: 0,
      problems_count: 0,
      repaired_count: 0,
      problems_capped: false,
      virtual_assets_skipped: false,
      items: [],
    },
  };
  const spies: Record<string, ReturnType<typeof vi.spyOn>> = {};
  for (const key of Object.getOwnPropertyNames(launcherAPI)) {
    const v = (launcherAPI as unknown as Record<string, unknown>)[key];
    if (typeof v === "function" && !(key in defaults)) {
      spies[key] = vi.spyOn(launcherAPI as never, key as never).mockResolvedValue([] as never);
    }
  }
  for (const [key, value] of Object.entries(defaults)) {
    const target = launcherAPI as unknown as Record<string, unknown>;
    if (typeof target[key] !== "function") {
      throw new Error(`acceptance fixture references unknown API method: ${key}`);
    }
    spies[key] = vi.spyOn(launcherAPI as never, key as never).mockResolvedValue(value as never);
  }
  return spies;
}

describe("v0.7.2 UI acceptance walk", () => {
  let api: ReturnType<typeof stubLauncherAPI>;

  beforeEach(() => {
    vi.restoreAllMocks();
    api = stubLauncherAPI();
  });
  afterEach(() => cleanup());

  it("unified import: one button, four tabs, .mrpack panel default + tab handoff", async () => {
    render(() => <App />);
    await screen.findByTestId("import-btn");
    expect(screen.queryByTestId("import-mrpack-btn")).toBeNull();

    fireEvent.click(screen.getByTestId("import-btn"));
    await waitFor(() => expect(screen.getByTestId("import-instance-modal")).toBeTruthy());
    expect(screen.getByTestId("mrpack-import-panel")).toBeTruthy();
    expect(screen.getByTestId("source-tab-mrpack")).toBeTruthy();
    expect(screen.queryByTestId("scan-btn")).toBeNull();

    for (const tab of ["minecraft", "prism", "curseforge"]) {
      fireEvent.click(screen.getByTestId(`source-tab-${tab}`));
      expect(screen.queryByTestId("mrpack-import-panel")).toBeNull();
    }
    await waitFor(() => expect(screen.getByTestId("cf-zip-path-input")).toBeTruthy());
    fireEvent.click(screen.getByTestId("source-tab-minecraft"));
    await waitFor(() => expect(api.scanOfficialMinecraft).toHaveBeenCalled());

    fireEvent.click(screen.getByTestId("source-tab-mrpack"));
    expect(screen.getByTestId("mrpack-import-panel")).toBeTruthy();
  });

  it("modpacks page: search, card import with sha1, handoff to CurseForge tab", async () => {
    render(() => <App />);
    fireEvent.click(await screen.findByTestId("nav-modpacks"));
    await screen.findByTestId("modpack-card-fabric-skyblocks");
    expect(api.searchMods).toHaveBeenCalledWith(
      expect.objectContaining({ source: "modrinth", project_type: "modpack" }),
    );

    fireEvent.click(screen.getByTestId("modpack-import-btn-fabric-skyblocks"));
    fireEvent.click(await screen.findByTestId("modpack-version-fabric-skyblocks-vXYZ"));
    await waitFor(() => expect(api.importMrPackFromURL).toHaveBeenCalled());
    expect(api.importMrPackFromURL.mock.calls[0][0]).toEqual({
      url: "https://cdn.modrinth.com/p.mrpack",
      instance_name: "Fabric Skyblocks",
      sha1: "aa",
      size: 12000000,
    });
    await screen.findByText("Импортировано");

    fireEvent.click(screen.getByTestId("modpacks-open-unified-btn"));
    await waitFor(() => expect(screen.getByTestId("import-instance-modal")).toBeTruthy());
    // the handoff must land on the CurseForge tab, not on the default .mrpack
    expect(screen.queryByTestId("mrpack-import-panel")).toBeNull();
    expect(screen.getByTestId("cf-zip-path-input")).toBeTruthy();
  });

  it("wizard: release channel default, per-loader recommended, snapshot switch", async () => {
    render(() => <App />);
    fireEvent.click(await screen.findByTestId("create-instance-btn"));
    await screen.findByTestId("wizard-name-input");
    expect(api.listMinecraftVersions).toHaveBeenCalledWith({ channel: "release" });

    const versionSel = (await waitFor(() => {
      const el = screen.getByTestId("wizard-version-select") as HTMLSelectElement;
      expect(el.value).toBe("26.3");
      return el;
    })) as HTMLSelectElement;

    await waitFor(() => expect(api.listLoaderVersions).toHaveBeenCalledWith({ game_version: "26.3", loader: "fabric" }));
    await waitFor(() => {
      const lv = screen.getByTestId("wizard-loader-version-select") as HTMLSelectElement;
      expect(lv.value).toBe("0.16.4");
    });

    for (const l of ["forge", "neoforge", "quilt", "vanilla"]) {
      fireEvent.click(screen.getByTestId(`wizard-loader-${l}`));
      if (l === "vanilla") {
        expect(screen.queryByTestId("wizard-loader-version-select")).toBeNull();
      } else {
        await waitFor(() =>
          expect(api.listLoaderVersions).toHaveBeenCalledWith({ game_version: "26.3", loader: l }),
        );
      }
    }

    fireEvent.click(screen.getByTestId("wizard-channel-snapshot"));
    await waitFor(() => expect(api.listMinecraftVersions).toHaveBeenCalledWith({ channel: "snapshot" }));

    fireEvent.click(screen.getByTestId("wizard-loader-fabric"));
    fireEvent.input(screen.getByTestId("wizard-name-input"), { target: { value: "wizard pack" } });
    await waitFor(() => {
      const lv = screen.getByTestId("wizard-loader-version-select") as HTMLSelectElement;
      expect(lv.value).toBe("0.16.4");
    });
    fireEvent.click(screen.getByTestId("wizard-create-btn"));
    await waitFor(() =>
      expect(api.createInstanceWithLoader).toHaveBeenCalledWith({
        name: "wizard pack",
        game_version: "26.3",
        loader: "fabric",
        loader_version: "0.16.4",
      }),
    );
    void versionSel;
  });

  it("wizard degrades honestly without network (manifest unreachable)", async () => {
    api.listMinecraftVersions.mockRejectedValue(new Error("offline mode: manifest unreachable"));
    render(() => <App />);
    fireEvent.click(await screen.findByTestId("create-instance-btn"));
    await screen.findByTestId("wizard-error");
    expect(screen.getByTestId("wizard-error").textContent).toContain("offline mode");
    expect((screen.getByTestId("wizard-create-btn") as HTMLButtonElement).disabled).toBe(true);
  });

  it("settings performance tab still hosts Java + Aikar + Discord toggle; mods tab has no catalog", async () => {
    render(() => <App />);
    fireEvent.click(await screen.findByTestId("open-instance-settings-button"));
    await screen.findByTestId("instance-settings-modal");
    fireEvent.click(screen.getByTestId("tab-performance"));
    await waitFor(() => expect(screen.getByTestId("perf-discord-toggle")).toBeTruthy());
    fireEvent.click(screen.getByTestId("perf-discord-toggle"));
    await waitFor(() => expect(api.setDiscordRpcEnabled).toHaveBeenCalledWith(true));
    expect(screen.getByTestId("settings-max-ram-input")).toBeTruthy();
    expect(screen.getByTestId("aikar-apply-btn")).toBeTruthy();
    expect(screen.queryByTestId("tab-optimization")).toBeNull();

    fireEvent.click(screen.getByTestId("tab-mods"));
    expect(screen.queryByTestId("mods-subtab-catalog")).toBeNull();
    fireEvent.click(screen.getByTestId("tab-datapacks"));
    expect(screen.getByTestId("tab-datapacks").className).toContain("");
  });
});
