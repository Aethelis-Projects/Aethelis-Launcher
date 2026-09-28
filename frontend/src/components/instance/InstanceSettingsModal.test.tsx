import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { InstanceSettingsModal } from "./InstanceSettingsModal";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO } from "../../bindings/ipc_types";

describe("InstanceSettingsModal (J2)", () => {
  const mockInstance: InstanceDTO = {
    id: "inst-modal-1",
    name: "Nord Survival 1.21",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
    java_path: "",
    skip_java_check: false,
    min_ram_mb: 2048,
    max_ram_mb: 4096,
    jvm_args: ["-XX:+UseG1GC"],
    state: "idle",
    total_play_seconds: 7200,
  };

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "listJavaRuntimes").mockResolvedValue([]);
  });

  it("renders when open and displays instance metadata", async () => {
    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    ));

    expect(screen.getByTestId("instance-settings-modal")).toBeTruthy();
    expect(screen.getByDisplayValue("Nord Survival 1.21")).toBeTruthy();
    expect(screen.getAllByText(/Minecraft 1.21.1/).length).toBeGreaterThanOrEqual(1);
  });

  it("shows Java, memory, JVM args and presets on a single performance tab (v0.7.2)", async () => {
    vi.spyOn(launcherAPI, "listInstalledMods").mockResolvedValue([]);
    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({ items: [], total_count: 0 });

    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        initialTab="performance"
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    ));

    // The four former tabs are one tab now.
    expect(screen.getByTestId("tab-performance")).toBeTruthy();
    expect(screen.queryByTestId("tab-java")).toBeNull();
    expect(screen.queryByTestId("tab-memory")).toBeNull();
    expect(screen.queryByTestId("tab-args")).toBeNull();
    expect(screen.queryByTestId("tab-optimization")).toBeNull();

    // Everything visible simultaneously.
    expect(screen.getByTestId("settings-java-path-input")).toBeTruthy();
    expect(screen.getByTestId("settings-skip-java-check-toggle")).toBeTruthy();
    expect(screen.getByTestId("settings-min-ram-input")).toBeTruthy();
    expect(screen.getByTestId("settings-max-ram-input")).toBeTruthy();
    expect(screen.getByTestId("settings-jvm-args-input")).toBeTruthy();
    expect(screen.getByText(/Предупреждение безопасности/)).toBeTruthy();
    expect(screen.getByTestId("aikar-apply-btn")).toBeTruthy();
    expect(screen.getByTestId("perf-discord-toggle")).toBeTruthy();

    // Mods subtabs still work.
    fireEvent.click(screen.getByTestId("tab-mods"));
    expect(screen.getByTestId("mods-subtab-installed")).toBeTruthy();
    fireEvent.click(screen.getByTestId("mods-subtab-catalog"));
    expect(await screen.findByText(/Каталог модификаций/)).toBeTruthy();
  });

  it("saves updated settings with expanded DTO parameters", async () => {
    const onSaved = vi.fn();
    const onClose = vi.fn();

    const updateSpy = vi.spyOn(launcherAPI, "updateInstance").mockResolvedValue({
      ...mockInstance,
      name: "Updated Survival",
      min_ram_mb: 3072,
      max_ram_mb: 6144,
      skip_java_check: true,
      jvm_args: ["-XX:+UseZGC"],
    });

    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={onClose}
        onSaved={onSaved}
      />
    ));

    // Change Name
    const nameInput = screen.getByTestId("settings-name-input");
    fireEvent.input(nameInput, { target: { value: "Updated Survival" } });

    // Java, memory and args all live on the single performance tab now.
    fireEvent.click(screen.getByTestId("tab-performance"));
    fireEvent.click(screen.getByTestId("settings-skip-java-check-toggle"));
    fireEvent.input(screen.getByTestId("settings-min-ram-input"), { target: { value: "3072" } });
    fireEvent.input(screen.getByTestId("settings-max-ram-input"), { target: { value: "6144" } });
    fireEvent.input(screen.getByTestId("settings-jvm-args-input"), { target: { value: "-XX:+UseZGC" } });

    fireEvent.click(screen.getByTestId("settings-save-button"));
    await vi.waitFor(() => expect(updateSpy).toHaveBeenCalledTimes(1));
    const req = updateSpy.mock.calls[0][0];
    expect(req.min_ram_mb).toBe(3072);
    expect(req.max_ram_mb).toBe(6144);
    expect(req.skip_java_check).toBe(true);
    expect(req.jvm_args).toEqual(["-XX:+UseZGC"]);
    expect(onSaved).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it("validates that min memory cannot exceed max memory", async () => {
    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    ));

    fireEvent.click(screen.getByTestId("tab-performance"));
    const minRamInput = screen.getByTestId("settings-min-ram-input");
    const maxRamInput = screen.getByTestId("settings-max-ram-input");
    fireEvent.input(minRamInput, { target: { value: "8192" } });
    fireEvent.input(maxRamInput, { target: { value: "2048" } });

    const saveBtn = screen.getByTestId("settings-save-button");
    fireEvent.click(saveBtn);

    const errorBanner = await screen.findByTestId("settings-error-banner");
    expect(errorBanner.textContent).toContain("Минимальный объем памяти не может превышать максимальный");
  });

  it("offers to install the missing recommended runtime (MC 26.3 -> Java 25)", async () => {
    const downloadSpy = vi.spyOn(launcherAPI, "downloadJavaRuntime").mockResolvedValue();
    vi.spyOn(launcherAPI, "listJavaRuntimes").mockResolvedValue([]);
    const inst26: InstanceDTO = {
      ...mockInstance,
      id: "inst-26",
      game_version: "26.3",
    };

    render(() => (
      <InstanceSettingsModal
        instance={inst26}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    ));

    fireEvent.click(await screen.findByTestId("tab-performance"));
    const installBtn = await screen.findByTestId("install-recommended-java-button");
    expect(installBtn.textContent).toContain("Java 25");
    fireEvent.click(installBtn);

    await vi.waitFor(() => {
      expect(downloadSpy).toHaveBeenCalledWith(25);
    });
    // The old standalone plate is gone (v0.7.2 de-duplication).
    expect(screen.queryByTestId("recommended-java-chip")).toBeNull();
  });

  it("picks an installed runtime from the runtimes list with one click", async () => {
    vi.spyOn(launcherAPI, "listJavaRuntimes").mockResolvedValue([
      {
        path: "C:\\Java\\jdk-17\\bin\\java.exe",
        home_dir: "C:\\Java\\jdk-17",
        major_version: 17,
        full_version: "17.0.10",
        vendor: "Eclipse Adoptium",
        kind: "managed",
        used_by: [],
      },
    ]);

    const inst17: InstanceDTO = {
      ...mockInstance,
      id: "inst-17",
      game_version: "1.20.1",
      java_path: "",
    };

    render(() => (
      <InstanceSettingsModal
        instance={inst17}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    ));

    fireEvent.click(await screen.findByTestId("tab-performance"));
    const row = await screen.findByText("C:\\Java\\jdk-17\\bin\\java.exe");
    fireEvent.click(row.closest("div[class*='cursor-pointer']") as Element);
    const pathInput = screen.getByTestId("settings-java-path-input") as HTMLInputElement;
    expect(pathInput.value).toBe("C:\\Java\\jdk-17\\bin\\java.exe");
  });

  it("calls openPath when instance folder button in General tab is clicked", async () => {
    const openPathSpy = vi.spyOn(launcherAPI, "openPath").mockResolvedValue();

    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    ));

    const openFolderBtn = await screen.findByTestId("settings-open-folder-btn");
    expect(openFolderBtn).toBeTruthy();
    fireEvent.click(openFolderBtn);

    expect(openPathSpy).toHaveBeenCalledWith("inst-modal-1");
  });

  it("renders preset avatars and saves selected avatar (UX1)", async () => {
    const onSaved = vi.fn();
    const updateSpy = vi.spyOn(launcherAPI, "updateInstance").mockResolvedValue({
      ...mockInstance,
      icon_path: "data:image/svg+xml;utf8,<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 32 32\"><rect width=\"32\" height=\"32\" rx=\"4\" fill=\"%2385552B\"/><rect width=\"32\" height=\"12\" rx=\"4\" fill=\"%235B8731\"/><path d=\"M4 12 L8 16 L12 12 L16 17 L20 12 L24 16 L28 12 L32 12 L32 8 L0 8 L0 12 Z\" fill=\"%235B8731\"/></svg>",
    });

    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={onSaved}
      />
    ));

    expect(screen.getByTestId("avatar-picker-section")).toBeTruthy();
    expect(screen.getByTestId("avatar-grid")).toBeTruthy();

    const grassPreset = screen.getByTestId("avatar-preset-grass");
    expect(grassPreset).toBeTruthy();
    fireEvent.click(grassPreset);

    const saveBtn = screen.getByTestId("settings-save-button");
    fireEvent.click(saveBtn);

    await vi.waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "inst-modal-1",
          icon_path: expect.stringContaining("data:image/svg+xml"),
        })
      );
    });
  });

  it("picks random avatar when dice button is clicked (UX1)", async () => {
    const onSaved = vi.fn();
    const updateSpy = vi.spyOn(launcherAPI, "updateInstance").mockResolvedValue({
      ...mockInstance,
    });

    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={onSaved}
      />
    ));

    const randomBtn = screen.getByTestId("random-avatar-btn");
    expect(randomBtn).toBeTruthy();
    fireEvent.click(randomBtn);

    const saveBtn = screen.getByTestId("settings-save-button");
    fireEvent.click(saveBtn);

    await vi.waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          id: "inst-modal-1",
          icon_path: expect.stringMatching(/^data:image\/svg\+xml/),
        })
      );
    });
  });

  it("renders Datapacks tab and switches to DatapackManager (Feature E)", async () => {
    vi.spyOn(launcherAPI, "listInstanceWorlds").mockResolvedValue([
      { name: "Survival", display_name: "Survival", last_played: Date.now(), datapack_count: 0 },
    ]);
    vi.spyOn(launcherAPI, "listWorldDatapacks").mockResolvedValue([]);

    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        initialTab="datapacks"
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    ));

    expect(screen.getByTestId("tab-datapacks")).toBeTruthy();
    expect(screen.getByTestId("datapack-manager")).toBeTruthy();
    expect(screen.getByTestId("settings-close-mods-button")).toBeTruthy();
  });
});


