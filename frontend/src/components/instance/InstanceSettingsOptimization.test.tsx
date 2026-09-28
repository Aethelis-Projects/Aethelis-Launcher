import { render, fireEvent, screen, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { InstanceSettingsModal } from "./InstanceSettingsModal";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO, ModItemDTO } from "../../bindings/ipc_types";

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

const curatedMod: ModItemDTO = {
  id: "proj-sodium",
  slug: "sodium",
  source: "modrinth",
  name: "Sodium",
  author: "jellysquid3",
  summary: "Rendering engine replacement",
  downloads: 1000,
  categories: ["fabric"],
  project_type: "mod",
};

describe("InstanceSettingsModal Optimization tab (Feature B)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "listJavaRuntimes").mockResolvedValue([]);
    vi.spyOn(launcherAPI, "getPerformancePreset").mockResolvedValue({
      suggested_ram_mb: 3072,
      aikar_args: AIKAR_ARGS,
    });
    vi.spyOn(launcherAPI, "listOptimizationMods").mockResolvedValue({
      items: [curatedMod],
      total_count: 1,
    });
  });

  const openModal = (onSave = vi.fn()) => {
    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={onSave}
        initialTab="optimization"
      />
    ));
    return onSave;
  };

  it("renders optimization tab content with preset recommendation", async () => {
    openModal();
    expect(await screen.findByTestId("aikar-apply-btn")).toBeTruthy();
    expect(screen.getByTestId("aikar-apply-btn").textContent).toContain("Aikar");
    await waitFor(() => expect(screen.getByText(/3072/)).toBeTruthy());
    expect(launcherAPI.getPerformancePreset).toHaveBeenCalled();
  });

  it("applying preset fills JVM args textarea with the full Aikar set", async () => {
    openModal();
    const apply = await screen.findByTestId("aikar-apply-btn");
    fireEvent.click(apply);
    await waitFor(() => expect(screen.getByTestId("tab-args")).toBeTruthy());
    fireEvent.click(screen.getByTestId("tab-args"));
    const textarea = screen.getByTestId("settings-jvm-args-input") as HTMLTextAreaElement;
    expect(textarea.value).toContain("-XX:+UseG1GC");
    expect(textarea.value).toContain("-XX:MaxGCPauseMillis=200");
    expect(textarea.value).not.toContain("-Xmx");
  });

  it("reset removes only aikar flags, keeps user flags", async () => {
    openModal();
    fireEvent.click(await screen.findByTestId("aikar-apply-btn"));
    fireEvent.click(screen.getByTestId("aikar-reset-btn"));
    fireEvent.click(await screen.findByTestId("tab-args"));
    const textarea = screen.getByTestId("settings-jvm-args-input") as HTMLTextAreaElement;
    expect(textarea.value).toBe("");
  });

  it("curated list loads on demand and installs through existing InstallMod", async () => {
    const installSpy = vi.spyOn(launcherAPI, "installMod").mockResolvedValue({
      success: true,
      file_name: "sodium-1.0.jar",
    } as Awaited<ReturnType<typeof launcherAPI.installMod>>);
    openModal();
    fireEvent.click(await screen.findByTestId("optimization-catalog-toggle"));
    await waitFor(() => expect(launcherAPI.listOptimizationMods).toHaveBeenCalledWith({
      game_version: "1.21.1",
      loader: "fabric",
    }));
    const btn = await screen.findByTestId("optimization-install-sodium");
    fireEvent.click(btn);
    await waitFor(() => expect(installSpy).toHaveBeenCalledTimes(1));
    const [instanceId, modArg] = installSpy.mock.calls[0];
    expect(instanceId).toBe("inst-perf-1");
    expect((modArg as ModItemDTO).slug).toBe("sodium");
    await waitFor(() => expect(screen.getByText(/Установлено/)).toBeTruthy());
  });

  it("empty curated set shows honest hint instead of fake rows", async () => {
    vi.spyOn(launcherAPI, "listOptimizationMods").mockResolvedValue({ items: [], total_count: 0 });
    openModal();
    fireEvent.click(await screen.findByTestId("optimization-catalog-toggle"));
    expect(await screen.findByText(/проверенного набора нет/)).toBeTruthy();
  });
});
