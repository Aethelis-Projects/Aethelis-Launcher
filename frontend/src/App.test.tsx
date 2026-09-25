import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { App } from "./App";
import { launcherAPI } from "./services/api";
import type { InstanceDTO } from "./bindings/ipc_types";

describe("App Component (B1, B2, D2, M1)", () => {
  const mockInstances: InstanceDTO[] = [
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
    },
    {
      id: "vanilla-1-20",
      name: "Vanilla Survival",
      game_version: "1.20.6",
      loader: "vanilla",
      min_ram_mb: 2048,
      max_ram_mb: 4096,
      jvm_args: [],
      skip_java_check: false,
      state: "idle",
      total_play_seconds: 500,
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "checkForUpdates").mockResolvedValue({
      has_update: false,
      version: "0.6.1",
      current_version: "0.6.1",
      release_date: "2026-09-25T12:00:00Z",
      release_notes: "",
      download_url: "",
      sha256: "",
      size: 0,
    });
  });

  it("loads and displays instances from launcherAPI.listInstances() on mount (B2)", async () => {
    const listSpy = vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);

    render(() => <App />);

    await vi.waitFor(() => {
      expect(listSpy).toHaveBeenCalled();
    });

    const titles = await screen.findAllByText("Nordic Fabric 1.21");
    expect(titles.length).toBeGreaterThanOrEqual(1);
  });

  it("launches the active instance when launch button is clicked (B1)", async () => {
    vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);
    const launchSpy = vi.spyOn(launcherAPI, "launchInstance").mockResolvedValue({
      success: true,
      pid: 9999,
    });

    render(() => <App />);

    const launchBtn = await screen.findByTestId("launch-button");
    expect(launchBtn.textContent).toContain("Запустить игру");

    fireEvent.click(launchBtn);

    await vi.waitFor(() => {
      expect(launchSpy).toHaveBeenCalledWith("default-fabric-1-21");
    });
  });

  it("handles launch failure and displays error on LaunchButton", async () => {
    vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);
    vi.spyOn(launcherAPI, "launchInstance").mockResolvedValue({
      success: false,
      error: "Java runtime not found",
    });

    render(() => <App />);

    const launchBtn = await screen.findByTestId("launch-button");
    fireEvent.click(launchBtn);

    const errorMsg = await screen.findByTestId("launch-error-msg");
    expect(errorMsg.textContent).toContain("Java runtime not found");
  });

  it("handles crash state during polling and opens CrashModal", async () => {
    vi.spyOn(launcherAPI, "listInstances")
      .mockResolvedValueOnce(mockInstances)
      .mockResolvedValueOnce([
        {
          ...mockInstances[0],
          state: "crashed",
        },
      ]);

    vi.spyOn(launcherAPI, "launchInstance").mockResolvedValue({
      success: true,
      pid: 1234,
    });

    const crashSpy = vi.spyOn(launcherAPI, "getLastCrashReport").mockResolvedValue({
      category: "out_of_memory",
      summary: "Out of Memory in Test",
      remedy: "Allocate more RAM",
      details: "Heap space",
      relevant_lines: ["Line 1", "Line 2"],
      exit_code: 1,
    });

    render(() => <App />);

    const launchBtn = await screen.findByTestId("launch-button");
    fireEvent.click(launchBtn);

    await vi.waitFor(() => {
      expect(crashSpy).toHaveBeenCalledWith("default-fabric-1-21");
    });

    const crashModalTitle = await screen.findByText(/Сбой выполнения игры/);
    expect(crashModalTitle).toBeTruthy();
    expect(screen.getByText("Out of Memory in Test")).toBeTruthy();
  });

  it("stops state polling when instance returns to idle after running (D2 guard)", async () => {
    vi.spyOn(launcherAPI, "listInstances")
      .mockResolvedValueOnce(mockInstances)
      .mockResolvedValueOnce([
        {
          ...mockInstances[0],
          state: "running",
        },
      ])
      .mockResolvedValueOnce([
        {
          ...mockInstances[0],
          state: "idle",
        },
      ]);

    vi.spyOn(launcherAPI, "launchInstance").mockResolvedValue({
      success: true,
      pid: 1234,
    });

    render(() => <App />);

    const launchBtn = await screen.findByTestId("launch-button");
    fireEvent.click(launchBtn);

    // 1. Observed running
    await vi.waitFor(() => {
      expect(launchBtn.textContent).toContain("Игра запущена");
    });

    // 2. Observed idle after running -> state resets to default
    await vi.waitFor(() => {
      expect(launchBtn.textContent).toContain("Запустить игру");
    });
  });

  it("displays system error banner when listInstances fails on mount (H6)", async () => {
    vi.spyOn(launcherAPI, "listInstances").mockRejectedValue(new Error("Database connection lost"));

    render(() => <App />);

    const banner = await screen.findByTestId("system-error-banner");
    expect(banner.textContent).toContain("Не удалось загрузить список сборок");
    expect(banner.textContent).toContain("Database connection lost");
  });

  it("configures instance Java path via updateInstance (S3)", async () => {
    vi.spyOn(launcherAPI, "listInstances").mockResolvedValue([
      {
        id: "inst-1",
        name: "Vanilla 1.21.1",
        game_version: "1.21.1",
        loader: "vanilla",
        min_ram_mb: 2048,
        max_ram_mb: 4096,
        jvm_args: [],
        skip_java_check: false,
        state: "idle",
        total_play_seconds: 0,
      },
    ]);

    const updateSpy = vi.spyOn(launcherAPI, "updateInstance").mockResolvedValue({
      id: "inst-1",
      name: "Vanilla 1.21.1",
      game_version: "1.21.1",
      loader: "vanilla",
      java_path: "C:\\Java21\\bin\\java.exe",
      min_ram_mb: 2048,
      max_ram_mb: 4096,
      jvm_args: [],
      skip_java_check: false,
      state: "idle",
      total_play_seconds: 0,
    });

    render(() => <App />);

    const input = await screen.findByTestId("instance-java-path-input");
    const saveBtn = await screen.findByTestId("save-java-path-button");

    fireEvent.input(input, { target: { value: "C:\\Java21\\bin\\java.exe" } });
    fireEvent.click(saveBtn);

    await vi.waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith({
        id: "inst-1",
        java_path: "C:\\Java21\\bin\\java.exe",
      });
    });
  });

  it("renders cockpit data density badges and log tail panel (J3)", async () => {
    vi.spyOn(launcherAPI, "listInstances").mockResolvedValue([
      {
        id: "cockpit-inst-1",
        name: "Cockpit Heavy Pack",
        game_version: "1.21.1",
        loader: "fabric",
        loader_version: "0.16.5",
        min_ram_mb: 4096,
        max_ram_mb: 8192,
        jvm_args: [],
        skip_java_check: false,
        state: "idle",
        total_play_seconds: 14400,
      },
    ]);
    vi.spyOn(launcherAPI, "listInstalledMods").mockResolvedValue([
      { file_name: "mod1.jar", name: "Mod 1", version: "1.0", enabled: true, size_bytes: 100 },
      { file_name: "mod2.jar", name: "Mod 2", version: "1.0", enabled: true, size_bytes: 200 },
    ]);
    vi.spyOn(launcherAPI, "getLogTail").mockResolvedValue([
      "[12:00:01] [main/INFO]: Loading Minecraft 1.21.1...",
      "[12:00:03] [main/INFO]: Minecraft client ready.",
    ]);

    render(() => <App />);

    const ramBadge = await screen.findByTestId("cockpit-ram-badge");
    expect(ramBadge.textContent).toContain("4096 - 8192 MB");

    const playtimeBadge = await screen.findByTestId("cockpit-playtime-badge");
    expect(playtimeBadge.textContent).toContain("4 ч 0 мин");

    const modsBadge = await screen.findByTestId("cockpit-mods-count-badge");
    await vi.waitFor(() => {
      expect(modsBadge.textContent).toContain("2 модов");
    });

    const javaChip = await screen.findByTestId("cockpit-java-chip");
    expect(javaChip.textContent).toContain("Adoptium (Auto)");

    const logTail = await screen.findByTestId("cockpit-log-tail");
    await vi.waitFor(() => {
      expect(logTail.textContent).toContain("Loading Minecraft 1.21.1...");
    });
  });

  it("opens InstanceSettingsModal from cockpit button", async () => {
    vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);

    render(() => <App />);

    const openSettingsBtn = await screen.findByTestId("open-instance-settings-button");
    fireEvent.click(openSettingsBtn);

    const modal = await screen.findByTestId("instance-settings-modal");
    expect(modal).toBeTruthy();
  });

  describe("7-State Update Badge & Startup Modal (B6, C5)", () => {
    beforeEach(() => {
      localStorage.clear();
    });

    it("hides update badge on offline startup / unknown state (B6)", async () => {
      vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);
      vi.spyOn(launcherAPI, "checkForUpdates").mockRejectedValue(new Error("Network offline"));

      render(() => <App />);

      await vi.waitFor(() => {
        expect(screen.queryByTestId("nav-settings-update-dot")).toBeNull();
        expect(screen.queryByTestId("startup-update-modal")).toBeNull();
      });
    });

    it("hides update badge when up-to-date (B6)", async () => {
      vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);
      vi.spyOn(launcherAPI, "checkForUpdates").mockResolvedValue({
        has_update: false,
        version: "0.6.1",
        current_version: "0.6.1",
        release_date: "2026-09-25T12:00:00Z",
        release_notes: "",
        download_url: "",
        sha256: "",
        size: 0,
      });

      render(() => <App />);

      await vi.waitFor(() => {
        expect(screen.queryByTestId("nav-settings-update-dot")).toBeNull();
        expect(screen.queryByTestId("startup-update-modal")).toBeNull();
      });
    });

    it("displays available badge and opens StartupUpdateModal on startup update (B6, C5)", async () => {
      vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);
      vi.spyOn(launcherAPI, "checkForUpdates").mockResolvedValue({
        has_update: true,
        version: "0.6.2",
        current_version: "0.6.1",
        release_date: "2026-09-25T12:00:00Z",
        release_notes: "### Added\n- Awesome feature",
        download_url: "https://example.com/update.exe",
        sha256: "abc",
        size: 20000000,
      });

      render(() => <App />);

      const modal = await screen.findByTestId("startup-update-modal");
      expect(modal).toBeTruthy();
      expect(screen.getByText("Доступно обновление Nord Launcher v0.6.2")).toBeTruthy();
      expect(screen.getByTestId("nav-settings-update-dot")).toBeTruthy();
    });

    it("snoozing update closes StartupUpdateModal but keeps badge visible as snoozed-visible (B6, C5)", async () => {
      vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);
      vi.spyOn(launcherAPI, "checkForUpdates").mockResolvedValue({
        has_update: true,
        version: "0.6.2",
        current_version: "0.6.1",
        release_date: "2026-09-25T12:00:00Z",
        release_notes: "### Added\n- Awesome feature",
        download_url: "https://example.com/update.exe",
        sha256: "abc",
        size: 20000000,
      });

      render(() => <App />);

      const snoozeBtn = await screen.findByTestId("startup-modal-snooze-btn");
      fireEvent.click(snoozeBtn);

      await vi.waitFor(() => {
        expect(screen.queryByTestId("startup-update-modal")).toBeNull();
      });

      // Badge must stay visible! (B6)
      expect(screen.getByTestId("nav-settings-update-dot")).toBeTruthy();
      expect(localStorage.getItem("nord_update_snooze")).toBeTruthy();
    });

    it("shows snoozed-visible badge without opening modal if snooze is active in localStorage (B6, C5)", async () => {
      localStorage.setItem("nord_update_snooze", String(Date.now() + 12 * 60 * 60 * 1000));
      vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);
      vi.spyOn(launcherAPI, "checkForUpdates").mockResolvedValue({
        has_update: true,
        version: "0.6.2",
        current_version: "0.6.1",
        release_date: "2026-09-25T12:00:00Z",
        release_notes: "Changelog",
        download_url: "https://example.com/update.exe",
        sha256: "abc",
        size: 20000000,
      });

      render(() => <App />);

      await vi.waitFor(() => {
        expect(screen.getByTestId("nav-settings-update-dot")).toBeTruthy();
      });
      expect(screen.queryByTestId("startup-update-modal")).toBeNull();
    });

    it("handles install and restart from StartupUpdateModal (C5)", async () => {
      vi.spyOn(launcherAPI, "listInstances").mockResolvedValue(mockInstances);
      vi.spyOn(launcherAPI, "checkForUpdates").mockResolvedValue({
        has_update: true,
        version: "0.6.2",
        current_version: "0.6.1",
        release_date: "2026-09-25T12:00:00Z",
        release_notes: "Changelog",
        download_url: "https://example.com/update.exe",
        sha256: "abc",
        size: 20000000,
      });

      const applySpy = vi.spyOn(launcherAPI, "applyUpdate").mockResolvedValue({
        success: true,
        restart_required: true,
      });
      const restartSpy = vi.spyOn(launcherAPI, "restartApplication").mockResolvedValue(undefined);

      render(() => <App />);

      const installBtn = await screen.findByTestId("startup-modal-install-btn");
      fireEvent.click(installBtn);

      await vi.waitFor(() => {
        expect(applySpy).toHaveBeenCalled();
        expect(restartSpy).toHaveBeenCalled();
      });
    });
  });
});

