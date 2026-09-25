import { render, screen, fireEvent } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { InstalledModsManager } from "./InstalledModsManager";
import { launcherAPI } from "../../services/api";

describe("InstalledModsManager Component", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    launcherAPI.setMockInstalledMods("nord-opti-1", [
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
    ]);
  });

  it("renders installed mods manager, search input, and session diff plate (C6)", async () => {
    render(() => <InstalledModsManager instanceId="nord-opti-1" />);

    expect(await screen.findByText(/Установленные модификации/)).toBeTruthy();
    expect(screen.getByPlaceholderText("Фильтр модов...")).toBeTruthy();

    const diff = await screen.findByTestId("session-diff");
    expect(diff.textContent).toContain("Изменено в этой сессии: (+0, -0, ~0)");
  });

  it("reflects session diff updates when mod is toggled (C6)", async () => {
    render(() => <InstalledModsManager instanceId="nord-opti-1" />);

    await screen.findByText("Sodium");
    const diff = screen.getByTestId("session-diff");
    expect(diff.textContent).toContain("Изменено в этой сессии: (+0, -0, ~0)");

    // Click toggle on first mod
    const toggleButtons = screen.getAllByTitle("Отключить мод");
    expect(toggleButtons.length).toBeGreaterThan(0);
    fireEvent.click(toggleButtons[0]);

    await vi.waitFor(() => {
      expect(diff.textContent).toContain("Изменено в этой сессии: (+0, -0, ~1)");
    });
  });

  it("checks for updates, displays updates banner, and updates individual mod via updateMod (C1)", async () => {
    vi.spyOn(launcherAPI, "checkModUpdates").mockResolvedValue([
      {
        file_name: "sodium-fabric-0.5.8.jar",
        mod_id: "sodium",
        source: "modrinth",
        current_version: "0.5.8",
        latest_version: "0.5.9",
        latest_version_id: "sodium-0.5.9-id",
        release_type: "release",
      },
    ]);
    const updateSpy = vi.spyOn(launcherAPI, "updateMod").mockResolvedValue({
      success: true,
      file_name: "sodium-fabric-0.5.9.jar",
      message: "Mod updated",
    });

    render(() => <InstalledModsManager instanceId="nord-opti-1" />);
    await screen.findByText("Sodium");

    const checkBtn = screen.getByTestId("check-updates-btn");
    expect(checkBtn.textContent).toContain("Проверить обновления");
    fireEvent.click(checkBtn);

    const banner = await screen.findByTestId("updates-banner");
    expect(banner.textContent).toContain("1 обновлений доступно");

    const updateBadge = screen.getByTestId("update-badge-sodium-fabric-0.5.8.jar");
    expect(updateBadge.textContent).toContain("Обновление: v0.5.9");

    const updateModBtn = updateBadge.querySelector("button");
    expect(updateModBtn).toBeTruthy();
    fireEvent.click(updateModBtn!);

    await vi.waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith({
        instance_id: "nord-opti-1",
        mod_id: "sodium",
        old_file_name: "sodium-fabric-0.5.8.jar",
        source: "modrinth",
        target_version_id: "sodium-0.5.9-id",
      });
    });
  });

  it("updates all available mods via updateMod and shows duplicate heal toast with undo (C1)", async () => {
    vi.spyOn(launcherAPI, "checkModUpdates").mockResolvedValue([
      {
        file_name: "sodium-fabric-0.5.8.jar",
        mod_id: "sodium",
        source: "modrinth",
        current_version: "0.5.8",
        latest_version: "0.5.9",
        latest_version_id: "sodium-0.5.9-id",
        release_type: "release",
      },
    ]);
    const updateSpy = vi.spyOn(launcherAPI, "updateMod").mockResolvedValue({
      success: true,
      file_name: "sodium-fabric-0.5.9.jar",
      message: "Mod updated",
      disabled_duplicates: ["sodium-fabric-0.5.8.jar"],
    });
    const toggleSpy = vi.spyOn(launcherAPI, "toggleMod").mockResolvedValue();

    render(() => <InstalledModsManager instanceId="nord-opti-1" />);
    await screen.findByText("Sodium");

    const checkBtn = screen.getByTestId("check-updates-btn");
    fireEvent.click(checkBtn);

    const updateAllBtn = await screen.findByTestId("update-all-btn");
    expect(updateAllBtn.textContent).toContain("Обновить все");
    fireEvent.click(updateAllBtn);

    await vi.waitFor(() => {
      expect(updateSpy).toHaveBeenCalledTimes(1);
    });

    const toast = await screen.findByTestId("duplicate-heal-toast");
    expect(toast.textContent).toContain("Отключены устаревшие дубликаты (1)");
    expect(toast.textContent).toContain("sodium-fabric-0.5.8.jar");

    const undoBtn = screen.getByTestId("undo-duplicate-heal-btn");
    expect(undoBtn).toBeTruthy();
    fireEvent.click(undoBtn);

    await vi.waitFor(() => {
      expect(toggleSpy).toHaveBeenCalledWith({
        instance_id: "nord-opti-1",
        file_name: "sodium-fabric-0.5.8.jar.disabled",
        enable: true,
      });
    });
  });

  it("opens ModUpdatesDiffModal when updates chip is clicked (G1, G3)", async () => {
    vi.spyOn(launcherAPI, "checkModUpdates").mockResolvedValue([
      {
        file_name: "sodium-fabric-0.5.8.jar",
        mod_id: "sodium",
        source: "modrinth",
        current_version: "0.5.8",
        latest_version: "0.5.9",
        latest_version_id: "sodium-0.5.9-id",
        release_type: "release",
        dependencies: ["fabric-api"],
        changelog: "Bug fixes and optimizations",
      },
    ]);

    render(() => <InstalledModsManager instanceId="nord-opti-1" />);
    await screen.findByText("Sodium");

    const checkBtn = screen.getByTestId("check-updates-btn");
    fireEvent.click(checkBtn);

    const diffChipBtn = await screen.findByTestId("updates-diff-chip-btn");
    expect(diffChipBtn.textContent).toContain("1 обновлений доступно");

    // Modal is not visible initially
    expect(screen.queryByTestId("mod-updates-diff-modal")).toBeNull();

    // Click chip
    fireEvent.click(diffChipBtn);

    // Modal should now be open
    const modal = await screen.findByTestId("mod-updates-diff-modal");
    expect(modal).toBeTruthy();
    expect(modal.textContent).toContain("Доступные обновления");
    expect(modal.textContent).toContain("0.5.8");
    expect(modal.textContent).toContain("0.5.9");
    expect(modal.textContent).toContain("fabric-api");
  });

  it("displays duplicate heal toast on mount if disabled duplicate mods exist (C1/tail)", async () => {
    launcherAPI.setMockInstalledMods("nord-opti-1", [
      {
        file_name: "sodium-fabric-0.5.9.jar",
        mod_id: "sodium",
        name: "Sodium",
        version: "0.5.9",
        enabled: true,
        size_bytes: 1048576,
      },
      {
        file_name: "sodium-fabric-0.5.8.jar.disabled",
        mod_id: "sodium",
        name: "Sodium",
        version: "0.5.8",
        enabled: false,
        size_bytes: 1048000,
      },
    ]);

    render(() => <InstalledModsManager instanceId="nord-opti-1" />);

    const toast = await screen.findByTestId("duplicate-heal-toast");
    expect(toast.textContent).toContain("Отключены устаревшие дубликаты (1)");
    expect(toast.textContent).toContain("sodium-fabric-0.5.8.jar");

    const closeBtn = toast.querySelector('button[title="Скрыть"]');
    expect(closeBtn).toBeTruthy();
    fireEvent.click(closeBtn!);

    await vi.waitFor(() => {
      expect(screen.queryByTestId("duplicate-heal-toast")).toBeNull();
    });
  });
});