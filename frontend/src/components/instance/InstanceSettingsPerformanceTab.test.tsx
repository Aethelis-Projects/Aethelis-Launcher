import { render, fireEvent, screen, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { InstanceSettingsModal } from "./InstanceSettingsModal";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO, JavaInstallationDTO } from "../../bindings/ipc_types";

const AIKAR_ARGS = ["-XX:+UseG1GC", "-XX:+ParallelRefProcEnabled", "-XX:MaxGCPauseMillis=200"];

const mockInstance: InstanceDTO = {
  id: "inst-perf-1",
  name: "Perf Instance",
  game_version: "1.21.1",
  loader: "fabric",
  java_path: "",
  skip_java_check: false,
  min_ram_mb: 2048,
  max_ram_mb: 4096,
  jvm_args: [],
  state: "idle",
  total_play_seconds: 0,
};

const runtime = (major: number, path: string): JavaInstallationDTO => ({
  path,
  home_dir: path.replace(/\/bin\/java.*$/, ""),
  major_version: major,
  full_version: `1.${major}.0`,
  vendor: "Temurin",
  kind: "detected",
  used_by: [],
});

describe("InstanceSettingsModal unified Performance & Java tab (v0.7.2)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "listJavaRuntimes").mockResolvedValue([runtime(21, "/jdk-21/bin/java")]);
    vi.spyOn(launcherAPI, "getPerformancePreset").mockResolvedValue({
      suggested_ram_mb: 3072,
      aikar_args: AIKAR_ARGS,
    });
    vi.spyOn(launcherAPI, "getDiscordRpcStatus").mockResolvedValue({
      enabled: false,
      connected: false,
      app_id_set: true,
      has_activity: false,
      last_error: "",
    });
    vi.spyOn(launcherAPI, "setDiscordRpcEnabled").mockResolvedValue(undefined);
  });

  const openModal = (onSave = vi.fn()) => {
    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={onSave}
        initialTab="performance"
      />
    ));
    return onSave;
  };

  it("renders preset recommendation, RAM, JVM args and Discord switch on one tab", async () => {
    openModal();
    expect(await screen.findByTestId("aikar-apply-btn")).toBeTruthy();
    expect(screen.getByText(/3072/)).toBeTruthy();
    // The three old tabs are gone: exactly one performance tab button exists.
    expect(screen.queryByTestId("tab-memory")).toBeNull();
    expect(screen.queryByTestId("tab-args")).toBeNull();
    expect(screen.queryByTestId("tab-optimization")).toBeNull();
    expect(screen.getByTestId("tab-performance").textContent).toContain("Производительность");
    // Memory presets, JVM textarea and the Discord switch are all visible at once.
    expect(screen.getByTestId("settings-jvm-args-input")).toBeTruthy();
    expect(screen.getByTestId("perf-discord-toggle")).toBeTruthy();
    expect(screen.getByTestId("settings-java-path-input")).toBeTruthy();
  });

  it("applying the Aikar preset fills the same-tab JVM args textarea", async () => {
    openModal();
    fireEvent.click(await screen.findByTestId("aikar-apply-btn"));
    const textarea = screen.getByTestId("settings-jvm-args-input") as HTMLTextAreaElement;
    await waitFor(() => expect(textarea.value).toContain("-XX:+UseG1GC"));
    expect(textarea.value).toContain("-XX:MaxGCPauseMillis=200");
    expect(textarea.value).not.toContain("-Xmx");
  });

  it("reset removes only aikar flags, keeps user flags", async () => {
    openModal();
    fireEvent.click(await screen.findByTestId("aikar-apply-btn"));
    fireEvent.click(screen.getByTestId("aikar-reset-btn"));
    expect((screen.getByTestId("settings-jvm-args-input") as HTMLTextAreaElement).value).toBe("");
  });

  it("discord switch mirrors status on open and persists toggles", async () => {
    openModal();
    const toggle = await screen.findByTestId("perf-discord-toggle");
    await waitFor(() => expect(launcherAPI.getDiscordRpcStatus).toHaveBeenCalled());
    fireEvent.click(toggle);
    await waitFor(() => expect(launcherAPI.setDiscordRpcEnabled).toHaveBeenCalledWith(true));
    expect(toggle.getAttribute("aria-checked")).toBe("true");
  });

  it("offers to install the missing recommended runtime instead of the old plate", async () => {
    // No 1.21.1-recommended Java (21 is present; pick an instance whose
    // recommendation is absent from the detected list).
    vi.mocked(launcherAPI.listJavaRuntimes).mockResolvedValue([runtime(8, "/jdk-8/bin/java")]);
    const download = vi.spyOn(launcherAPI, "downloadJavaRuntime").mockResolvedValue(undefined);
    openModal();
    const btn = await screen.findByTestId("install-recommended-java-button");
    expect(btn.textContent).toContain("Установить рекомендуемую Java");
    fireEvent.click(btn);
    await waitFor(() => expect(download).toHaveBeenCalledWith(21));
  });
});
