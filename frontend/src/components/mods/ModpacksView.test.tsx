import { render, fireEvent, screen, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ModpacksView } from "./ModpacksView";
import { launcherAPI } from "../../services/api";
import type { ModItemDTO, MrPackImportStatusDTO, MrPackVersionDTO } from "../../bindings/ipc_types";

const pack: ModItemDTO = {
  id: "proj1",
  slug: "fabric-skyblocks",
  source: "modrinth",
  name: "Fabric Skyblocks",
  author: "mahomax",
  summary: "skyblocks but fabric",
  downloads: 2500000,
  categories: ["fabric", "adventure"],
  project_type: "modpack" as never,
};

const mrpackVersion: MrPackVersionDTO = {
  version_id: "vXYZ",
  name: "Fabric Skyblocks 2.6.0",
  version_type: "release",
  game_version: "1.21.4",
  loaders: ["fabric"],
  url: "https://cdn.modrinth.com/data/proj1/versions/vXYZ/pack.mrpack",
  filename: "pack.mrpack",
  size: 12000000,
};

const doneStatus: MrPackImportStatusDTO = {
  task_id: "Fabric Skyblocks",
  status: "complete",
  current_file: "",
  files_done: 10,
  total_files: 10,
  bytes_read: 12,
  total_bytes: 12,
  percentage: 100,
};

function stubCommon() {
  vi.spyOn(launcherAPI, "listMinecraftVersions").mockResolvedValue([]);
  vi.spyOn(launcherAPI, "listProjectTags").mockResolvedValue([]);
  vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({ items: [pack], total_count: 1 } as never);
}

describe("ModpacksView (v0.7.2 G5)", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("searches modpacks with server-side filters and renders cards", async () => {
    stubCommon();
    render(() => <ModpacksView />);
    await screen.findByTestId("modpack-card-fabric-skyblocks");
    expect(launcherAPI.searchMods).toHaveBeenCalledWith(
      expect.objectContaining({ source: "modrinth", project_type: "modpack" }),
    );
    expect(screen.getByText("Fabric Skyblocks")).toBeTruthy();
    expect((screen.getByTestId("modpacks-provider-note") as HTMLElement).textContent).toContain("CurseForge");
  });

  it("imports a selected .mrpack version and reports completion", async () => {
    stubCommon();
    vi.spyOn(launcherAPI, "listMrPackVersions").mockResolvedValue([mrpackVersion]);
    const importSpy = vi.spyOn(launcherAPI, "importMrPackFromURL").mockResolvedValue("inst-1");
    vi.spyOn(launcherAPI, "getMrPackURLImportStatus").mockResolvedValue(doneStatus);
    const onImported = vi.fn();

    render(() => <ModpacksView onImported={onImported} />);
    fireEvent.click(await screen.findByTestId("modpack-import-btn-fabric-skyblocks"));
    fireEvent.click(await screen.findByTestId("modpack-version-fabric-skyblocks-vXYZ"));

    await waitFor(() => expect(importSpy).toHaveBeenCalled());
    expect(importSpy.mock.calls[0][0]).toEqual({
      url: mrpackVersion.url,
      instance_name: "Fabric Skyblocks",
    });
    await screen.findByText("Импортировано");
    expect(onImported).toHaveBeenCalled();
  });

  it("hands the CurseForge path over to the unified import modal", async () => {
    stubCommon();
    const onOpen = vi.fn();
    render(() => <ModpacksView onOpenUnifiedImport={onOpen} />);
    fireEvent.click(await screen.findByTestId("modpacks-open-unified-btn"));
    expect(onOpen).toHaveBeenCalledWith("curseforge");
  });

  it("surfaces search errors honestly", async () => {
    vi.spyOn(launcherAPI, "listMinecraftVersions").mockResolvedValue([]);
    vi.spyOn(launcherAPI, "listProjectTags").mockResolvedValue([]);
    vi.spyOn(launcherAPI, "searchMods").mockRejectedValue(new Error("rate limited"));
    render(() => <ModpacksView />);
    await screen.findByTestId("modpacks-error-banner");
    expect(screen.getByTestId("modpacks-error-banner").textContent).toContain("rate limited");
  });
});
