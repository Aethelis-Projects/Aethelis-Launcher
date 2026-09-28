import { render, fireEvent, screen, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { InstanceSettingsModal } from "./InstanceSettingsModal";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO, IntegrityResultDTO } from "../../bindings/ipc_types";

const mockInstance: InstanceDTO = {
  id: "inst-int-ui",
  name: "Integrity Instance",
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

const cleanResult: IntegrityResultDTO = {
  version: "1.21.1",
  checked_count: 42,
  problems_count: 0,
  repaired_count: 0,
  problems_capped: false,
  virtual_assets_skipped: false,
  items: [],
};

const brokenResult: IntegrityResultDTO = {
  version: "1.21.1",
  checked_count: 42,
  problems_count: 2,
  repaired_count: 0,
  problems_capped: false,
  virtual_assets_skipped: false,
  items: [
    { path: "Client JAR", reason: "ChecksumMismatch" },
    { path: "Asset a293", reason: "Missing" },
  ],
};

const repairedResult: IntegrityResultDTO = {
  ...brokenResult,
  problems_count: 0,
  repaired_count: 2,
  items: [],
};

describe("InstanceSettingsModal Integrity tab (D'2)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "listJavaRuntimes").mockResolvedValue([]);
    vi.spyOn(launcherAPI, "getPerformancePreset").mockResolvedValue({ suggested_ram_mb: 2048, aikar_args: [] });
  });

  const openTab = () => {
    render(() => (
      <InstanceSettingsModal
        instance={mockInstance}
        isOpen={true}
        onClose={vi.fn()}
        onSaved={vi.fn()}
        initialTab="integrity"
      />
    ));
  };

  it("runs check pass and renders clean summary without repair call", async () => {
    const checkSpy = vi.spyOn(launcherAPI, "checkInstanceFiles").mockResolvedValue(cleanResult);
    const repairSpy = vi.spyOn(launcherAPI, "repairInstanceFiles");
    openTab();
    fireEvent.click(await screen.findByTestId("integrity-check-btn"));
    await waitFor(() => expect(checkSpy).toHaveBeenCalledWith("inst-int-ui"));
    expect(repairSpy).not.toHaveBeenCalled();
    const result = await screen.findByTestId("integrity-result");
    expect(result.textContent).toContain("42");
    expect(result.textContent).not.toContain("ChecksumMismatch");
  });

  it("renders findings list and repair fixes them", async () => {
    vi.spyOn(launcherAPI, "checkInstanceFiles").mockResolvedValue(brokenResult);
    const repairSpy = vi.spyOn(launcherAPI, "repairInstanceFiles").mockResolvedValue(repairedResult);
    openTab();
    fireEvent.click(await screen.findByTestId("integrity-check-btn"));
    const result = await screen.findByTestId("integrity-result");
    expect(result.textContent).toContain("ChecksumMismatch");
    expect(result.textContent).toContain("Missing");

    fireEvent.click(screen.getByTestId("integrity-repair-btn"));
    await waitFor(() => expect(repairSpy).toHaveBeenCalledWith("inst-int-ui"));
    await waitFor(() => expect(screen.getByTestId("integrity-result").textContent).toContain("Починено: 2"));
  });

  it("surfaces check failure in the settings error banner", async () => {
    vi.spyOn(launcherAPI, "checkInstanceFiles").mockRejectedValue(new Error("integrity verifier not initialized"));
    openTab();
    fireEvent.click(await screen.findByTestId("integrity-check-btn"));
    const banner = await screen.findByTestId("settings-error-banner");
    expect(banner.textContent).toContain("integrity verifier not initialized");
  });

  it("disables both actions while a pass is running", async () => {
    let resolveCheck: (v: IntegrityResultDTO) => void = () => {};
    vi.spyOn(launcherAPI, "checkInstanceFiles").mockReturnValue(new Promise((res) => { resolveCheck = res; }));
    openTab();
    const checkBtn = await screen.findByTestId("integrity-check-btn");
    fireEvent.click(checkBtn);
    await waitFor(() => expect((screen.getByTestId("integrity-repair-btn") as HTMLButtonElement).disabled).toBe(true));
    resolveCheck(cleanResult);
    await waitFor(() => expect((checkBtn as HTMLButtonElement).disabled).toBe(false));
  });
});
