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

  it("switches across all 5 tabs (General, Java, Memory, Arguments, Mods) and respects initialTab (C5)", async () => {
    vi.spyOn(launcherAPI, "listInstalledMods").mockResolvedValue([]);
    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({ items: [], total_count: 0 });

    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        initialTab="mods"
        onClose={vi.fn()}
        onSaved={vi.fn()}
      />
    ));

    // Initial tab is mods
    expect(screen.getByTestId("tab-mods")).toBeTruthy();
    expect(screen.getByTestId("mods-subtab-installed")).toBeTruthy();
    expect(screen.getByTestId("mods-subtab-catalog")).toBeTruthy();

    // Switch subtabs
    fireEvent.click(screen.getByTestId("mods-subtab-catalog"));
    expect(await screen.findByText(/Каталог модификаций/)).toBeTruthy();

    // Java tab
    fireEvent.click(screen.getByTestId("tab-java"));
    expect(screen.getByTestId("settings-java-path-input")).toBeTruthy();
    expect(screen.getByTestId("settings-skip-java-check-toggle")).toBeTruthy();

    // Memory tab
    fireEvent.click(screen.getByTestId("tab-memory"));
    expect(screen.getByTestId("settings-min-ram-input")).toBeTruthy();
    expect(screen.getByTestId("settings-max-ram-input")).toBeTruthy();

    // Arguments tab
    fireEvent.click(screen.getByTestId("tab-args"));
    expect(screen.getByTestId("settings-jvm-args-input")).toBeTruthy();
    expect(screen.getByText(/Предупреждение безопасности/)).toBeTruthy();
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

    // Change Java & skip check
    fireEvent.click(screen.getByTestId("tab-java"));
    const skipCheckToggle = screen.getByTestId("settings-skip-java-check-toggle");
    fireEvent.click(skipCheckToggle);

    // Change Memory
    fireEvent.click(screen.getByTestId("tab-memory"));
    const minRamInput = screen.getByTestId("settings-min-ram-input");
    const maxRamInput = screen.getByTestId("settings-max-ram-input");
    fireEvent.input(minRamInput, { target: { value: "3072" } });
    fireEvent.input(maxRamInput, { target: { value: "6144" } });

    // Change Args
    fireEvent.click(screen.getByTestId("tab-args"));
    const argsInput = screen.getByTestId("settings-jvm-args-input");
    fireEvent.input(argsInput, { target: { value: "-XX:+UseZGC" } });

    // Save
    const saveBtn = screen.getByTestId("settings-save-button");
    fireEvent.click(saveBtn);

    await vi.waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith({
        id: "inst-modal-1",
        name: "Updated Survival",
        java_path: undefined,
        clear_java_path: false,
        skip_java_check: true,
        min_ram_mb: 3072,
        max_ram_mb: 6144,
        jvm_args: ["-XX:+UseZGC"],
      });
      expect(onSaved).toHaveBeenCalled();
      expect(onClose).toHaveBeenCalled();
    });
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

    fireEvent.click(screen.getByTestId("tab-memory"));
    const minRamInput = screen.getByTestId("settings-min-ram-input");
    const maxRamInput = screen.getByTestId("settings-max-ram-input");
    fireEvent.input(minRamInput, { target: { value: "8192" } });
    fireEvent.input(maxRamInput, { target: { value: "2048" } });

    const saveBtn = screen.getByTestId("settings-save-button");
    fireEvent.click(saveBtn);

    const errorBanner = await screen.findByTestId("settings-error-banner");
    expect(errorBanner.textContent).toContain("Минимальный объем памяти не может превышать максимальный");
  });
});
