import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { DatapackManager } from "./DatapackManager";
import { launcherAPI } from "../../services/api";
import type { WorldDTO, DatapackDTO } from "../../bindings/ipc_types";

describe("DatapackManager Component (Feature E)", () => {
  const mockWorlds: WorldDTO[] = [
    {
      name: "SurvivalWorld",
      display_name: "SurvivalWorld",
      last_played: 1727395200000,
      datapack_count: 1,
    },
    {
      name: "CreativeWorld",
      display_name: "CreativeWorld",
      last_played: 1727308800000,
      datapack_count: 0,
    },
  ];

  const mockDatapacks: DatapackDTO[] = [
    {
      file_name: "armor-statues.zip",
      name: "armor-statues",
      enabled: true,
      size_bytes: 524288,
      world_name: "SurvivalWorld",
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders empty state when instance has no saved worlds", async () => {
    vi.spyOn(launcherAPI, "listInstanceWorlds").mockResolvedValue([]);
    vi.spyOn(launcherAPI, "listWorldDatapacks").mockResolvedValue([]);

    render(() => <DatapackManager instanceId="test-inst" />);

    await vi.waitFor(() => {
      expect(screen.getByTestId("datapack-no-worlds")).toBeTruthy();
    });
    expect(screen.getByTestId("datapack-reload-hint")).toBeTruthy();
  });

  it("renders worlds in select dropdown and lists world datapacks", async () => {
    vi.spyOn(launcherAPI, "listInstanceWorlds").mockResolvedValue(mockWorlds);
    vi.spyOn(launcherAPI, "listWorldDatapacks").mockResolvedValue(mockDatapacks);

    render(() => <DatapackManager instanceId="test-inst" />);

    await vi.waitFor(() => {
      expect(screen.getByTestId("datapack-world-select")).toBeTruthy();
      expect(screen.getByTestId("datapack-item-armor-statues.zip")).toBeTruthy();
    });

    expect(screen.getByTestId("datapack-status-armor-statues.zip").textContent).toContain("Включен");
    expect(screen.getByTestId("datapack-toggle-armor-statues.zip").textContent).toContain("Отключить");
  });

  it("toggles datapack enabled state via physical move (datapacks-disabled/)", async () => {
    vi.spyOn(launcherAPI, "listInstanceWorlds").mockResolvedValue(mockWorlds);
    vi.spyOn(launcherAPI, "listWorldDatapacks").mockResolvedValue(mockDatapacks);
    const toggleSpy = vi.spyOn(launcherAPI, "setDatapackEnabled").mockResolvedValue();

    render(() => <DatapackManager instanceId="test-inst" />);

    await vi.waitFor(() => {
      expect(screen.getByTestId("datapack-toggle-armor-statues.zip")).toBeTruthy();
    });

    const toggleBtn = screen.getByTestId("datapack-toggle-armor-statues.zip");
    fireEvent.click(toggleBtn);

    await vi.waitFor(() => {
      expect(toggleSpy).toHaveBeenCalledWith({
        instance_id: "test-inst",
        world_name: "SurvivalWorld",
        file_name: "armor-statues.zip",
        enabled: false,
      });
    });
  });

  it("deletes datapack from world", async () => {
    vi.spyOn(launcherAPI, "listInstanceWorlds").mockResolvedValue(mockWorlds);
    vi.spyOn(launcherAPI, "listWorldDatapacks").mockResolvedValue(mockDatapacks);
    const deleteSpy = vi.spyOn(launcherAPI, "deleteDatapack").mockResolvedValue();

    render(() => <DatapackManager instanceId="test-inst" />);

    await vi.waitFor(() => {
      expect(screen.getByTestId("datapack-delete-armor-statues.zip")).toBeTruthy();
    });

    const deleteBtn = screen.getByTestId("datapack-delete-armor-statues.zip");
    fireEvent.click(deleteBtn);

    await vi.waitFor(() => {
      expect(deleteSpy).toHaveBeenCalledWith({
        instance_id: "test-inst",
        world_name: "SurvivalWorld",
        file_name: "armor-statues.zip",
      });
    });
  });

  it("opens datapack folder on disk using ensureInstanceDir and openPath", async () => {
    vi.spyOn(launcherAPI, "listInstanceWorlds").mockResolvedValue(mockWorlds);
    vi.spyOn(launcherAPI, "listWorldDatapacks").mockResolvedValue(mockDatapacks);
    const ensureDirSpy = vi.spyOn(launcherAPI, "ensureInstanceDir").mockResolvedValue("instances/test-inst/saves/SurvivalWorld/datapacks");
    const openPathSpy = vi.spyOn(launcherAPI, "openPath").mockResolvedValue();

    render(() => <DatapackManager instanceId="test-inst" />);

    await vi.waitFor(() => {
      expect(screen.getByTestId("datapack-open-folder-btn")).toBeTruthy();
    });

    const folderBtn = screen.getByTestId("datapack-open-folder-btn");
    fireEvent.click(folderBtn);

    await vi.waitFor(() => {
      expect(ensureDirSpy).toHaveBeenCalledWith("test-inst", "saves/SurvivalWorld/datapacks");
      expect(openPathSpy).toHaveBeenCalledWith("instances/test-inst/saves/SurvivalWorld/datapacks");
    });
  });
});
