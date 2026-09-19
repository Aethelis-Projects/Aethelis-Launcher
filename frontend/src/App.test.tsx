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
      state: "idle",
      total_play_seconds: 120,
    },
    {
      id: "vanilla-1-20",
      name: "Vanilla Survival",
      game_version: "1.20.6",
      loader: "vanilla",
      state: "idle",
      total_play_seconds: 500,
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "checkForUpdates").mockResolvedValue({
      has_update: false,
      version: "0.2.2",
      current_version: "0.2.2",
      release_date: "2026-09-19T22:00:00Z",
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
});
