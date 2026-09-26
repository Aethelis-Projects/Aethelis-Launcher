import { render, screen, fireEvent, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ImportInstanceModal } from "./ImportInstanceModal";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO } from "../../bindings/ipc_types";

describe("ImportInstanceModal Component (Feature D'4a)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders modal and scans official .minecraft on mount", async () => {
    const scanSpy = vi.spyOn(launcherAPI, "scanOfficialMinecraft").mockResolvedValue({
      path: "C:\\Users\\Test\\AppData\\Roaming\\.minecraft",
      versions: ["1.21.1", "1.20.4"],
      default_version: "1.21.1",
      world_count: 5,
      resource_packs: 3,
      screenshots: 12,
      mod_count: 2,
      has_options: true,
      has_servers: true,
    });

    render(() => (
      <ImportInstanceModal
        isOpen={true}
        onClose={() => {}}
      />
    ));

    expect(screen.getByText("Импорт инстанса")).toBeTruthy();
    expect(scanSpy).toHaveBeenCalled();

    await screen.findByText("Обнаруженные компоненты");
    expect(screen.getByText("5")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
  });

  it("switches to Prism tab and performs scan on custom path", async () => {
    vi.spyOn(launcherAPI, "scanOfficialMinecraft").mockResolvedValue({
      path: "C:\\default\\.minecraft",
      versions: ["1.21.1"],
      default_version: "1.21.1",
      world_count: 0,
      resource_packs: 0,
      screenshots: 0,
      mod_count: 0,
      has_options: false,
      has_servers: false,
    });

    const prismScanSpy = vi.spyOn(launcherAPI, "scanPrismInstance").mockResolvedValue({
      path: "C:\\Prism\\instances\\Speedrun",
      instance_name: "Speedrun 1.20",
      game_version: "1.20.1",
      loader: "fabric",
      loader_version: "0.15.11",
      world_count: 2,
      resource_packs: 1,
      screenshots: 4,
      mod_count: 8,
      has_options: true,
      has_servers: true,
    });

    render(() => (
      <ImportInstanceModal
        isOpen={true}
        onClose={() => {}}
      />
    ));

    await waitFor(() => {
      const btn = screen.getByTestId("scan-btn") as HTMLButtonElement;
      expect(btn.disabled).toBe(false);
    });

    const prismTab = screen.getByTestId("source-tab-prism");
    fireEvent.click(prismTab);

    const input = screen.getByTestId("import-source-path-input") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "C:\\Prism\\instances\\Speedrun" } });

    const scanBtn = screen.getByTestId("scan-btn");
    fireEvent.click(scanBtn);

    await waitFor(() => {
      expect(prismScanSpy).toHaveBeenCalledWith({
        dir_path: "C:\\Prism\\instances\\Speedrun",
      });
    });

    const nameInput = screen.getByTestId("import-instance-name-input") as HTMLInputElement;
    expect(nameInput.value).toBe("Speedrun 1.20");
  });

  it("imports official minecraft instance and triggers onImported callback", async () => {
    vi.spyOn(launcherAPI, "scanOfficialMinecraft").mockResolvedValue({
      path: "C:\\Users\\Test\\.minecraft",
      versions: ["1.21.1"],
      default_version: "1.21.1",
      world_count: 1,
      resource_packs: 0,
      screenshots: 0,
      mod_count: 0,
      has_options: true,
      has_servers: false,
    });

    const mockImported: InstanceDTO = {
      id: "official-import-123",
      name: "Official Minecraft",
      game_version: "1.21.1",
      loader: "vanilla",
      min_ram_mb: 2048,
      max_ram_mb: 4096,
      jvm_args: [],
      skip_java_check: false,
      state: "idle",
      total_play_seconds: 0,
    };

    const importSpy = vi.spyOn(launcherAPI, "importOfficialMinecraft").mockResolvedValue(mockImported);
    const onImported = vi.fn();

    render(() => (
      <ImportInstanceModal
        isOpen={true}
        onClose={() => {}}
        onImported={onImported}
      />
    ));

    await screen.findByText("Обнаруженные компоненты");

    const startBtn = screen.getByTestId("start-import-btn");
    fireEvent.click(startBtn);

    await screen.findByTestId("import-complete-view");
    expect(importSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        instance_name: "Minecraft 1.21.1",
        game_version: "1.21.1",
        copy_saves: true,
      })
    );
    expect(onImported).toHaveBeenCalledWith(mockImported);
  });
});
