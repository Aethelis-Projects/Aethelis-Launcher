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

  it("checks for updates, displays updates banner, and installs individual update (C6)", async () => {
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
    const installSpy = vi.spyOn(launcherAPI, "installMod").mockResolvedValue({
      success: true,
      file_name: "sodium-fabric-0.5.9.jar",
      message: "Mod installed",
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
      expect(installSpy).toHaveBeenCalledWith(
        "nord-opti-1",
        expect.objectContaining({ id: "sodium", source: "modrinth" }),
        "sodium-0.5.9-id"
      );
    });
  });

  it("updates all available mods when 'Обновить все' is clicked (C6)", async () => {
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
    const installSpy = vi.spyOn(launcherAPI, "installMod").mockResolvedValue({
      success: true,
      file_name: "sodium-fabric-0.5.9.jar",
      message: "Mod installed",
    });

    render(() => <InstalledModsManager instanceId="nord-opti-1" />);
    await screen.findByText("Sodium");

    const checkBtn = screen.getByTestId("check-updates-btn");
    fireEvent.click(checkBtn);

    const updateAllBtn = await screen.findByTestId("update-all-btn");
    expect(updateAllBtn.textContent).toContain("Обновить все");
    fireEvent.click(updateAllBtn);

    await vi.waitFor(() => {
      expect(installSpy).toHaveBeenCalledTimes(1);
    });
  });
});