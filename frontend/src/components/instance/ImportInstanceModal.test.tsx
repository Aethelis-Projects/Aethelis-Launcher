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

describe("ImportInstanceModal CurseForge .zip tab (Feature D'4b)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  const plan = {
    format: "manifest",
    instance_name: "Vault Hunters",
    game_version: "1.20.1",
    loader: "forge",
    loader_version: "14.23.5.2847",
    files: [
      { project_id: 1, file_id: 10, file_name: "sodium.jar", required: true, download_url: "http://x/10.jar" },
      { project_id: 2, file_id: 20, file_name: "lithium.jar", required: true, download_url: "http://x/20.jar" },
    ],
    unresolved: [{ project_id: 3, file_id: 30, required: true, resolve_error: "curseforge API key unavailable" }],
    override_names: ["options.txt", "config/x.toml"],
    blocked_names: ["launcher_accounts.json"],
    required_total: 3,
    required_failed: 1,
  } satisfies import("../../bindings/ipc_types").CFPackPlanDTO;

  it("scans the zip and shows the honest plan (counts, blocked note, API warning)", async () => {
    vi.spyOn(launcherAPI, "scanOfficialMinecraft").mockResolvedValue({
      path: "C:\\x\\.minecraft", versions: ["1.21.1"], default_version: "1.21.1",
      world_count: 0, resource_packs: 0, screenshots: 0, mod_count: 0, has_options: false, has_servers: false,
    });
    const scanSpy = vi.spyOn(launcherAPI, "scanCurseForgePackZip").mockResolvedValue(plan);

    render(() => <ImportInstanceModal isOpen={true} onClose={() => {}} />);
    fireEvent.click(screen.getByTestId("source-tab-curseforge"));
    fireEvent.input(screen.getByTestId("cf-zip-path-input"), { target: { value: "C:\\Downloads\\Vault.zip" } });
    await new Promise((r) => setTimeout(r, 10)); // let solid flush the signal update before clicking
    fireEvent.click(screen.getByTestId("cf-scan-btn"));

    await screen.findByTestId("cf-plan-card");
    expect(scanSpy).toHaveBeenCalledWith({ zip_path: "C:\\Downloads\\Vault.zip" });
    expect(screen.getByTestId("cf-import-warning-banner").textContent).toContain("1 файл(ов) не удалось разрешить");
    expect(screen.getByTestId("cf-blocked-note").textContent).toContain("учётных файл(ов)");
    expect(screen.getByTestId("cf-plan-card").textContent).toContain("Vault Hunters");
    expect(screen.getByTestId("cf-plan-card").textContent).toContain("1.20.1");
    expect(screen.getByTestId("start-import-btn").hasAttribute("disabled")).toBe(false);
  });

  it("imports and surfaces the partial result note", async () => {
    vi.spyOn(launcherAPI, "scanOfficialMinecraft").mockResolvedValue({
      path: "C:\\x\\.minecraft", versions: ["1.21.1"], default_version: "1.21.1",
      world_count: 0, resource_packs: 0, screenshots: 0, mod_count: 0, has_options: false, has_servers: false,
    });
    vi.spyOn(launcherAPI, "scanCurseForgePackZip").mockResolvedValue(plan);
    const importSpy = vi.spyOn(launcherAPI, "importCurseForgePackZip").mockResolvedValue({
      instance_id: "cf-inst-1", downloaded: 2, override_files: 2,
      skipped_credentials: ["overrides/launcher_accounts.json"],
      failed_files: [], unresolved: ["cf:3/30 (curseforge API key unavailable)"],
    });
    const onImported = vi.fn();

    render(() => <ImportInstanceModal isOpen={true} onClose={() => {}} onImported={onImported} />);
    fireEvent.click(screen.getByTestId("source-tab-curseforge"));
    fireEvent.input(screen.getByTestId("cf-zip-path-input"), { target: { value: "C:\\Downloads\\Vault.zip" } });
    await new Promise((r) => setTimeout(r, 10)); // solid signal flush
    fireEvent.click(screen.getByTestId("cf-scan-btn"));
    await screen.findByTestId("cf-plan-card");
    fireEvent.click(screen.getByTestId("start-import-btn"));

    await waitFor(() => expect(importSpy).toHaveBeenCalledWith({ zip_path: "C:\\Downloads\\Vault.zip" }));
    await screen.findByTestId("import-complete-view");
    expect(screen.getByTestId("import-complete-view").textContent).toContain("Загружено модов: 2");
    expect(onImported).toHaveBeenCalledWith({ id: "cf-inst-1", name: "cf-inst-1" });
  });

  it("modlist-html fallback is declared as manual-install, never silent", async () => {
    vi.spyOn(launcherAPI, "scanOfficialMinecraft").mockResolvedValue({
      path: "C:\\x\\.minecraft", versions: ["1.21.1"], default_version: "1.21.1",
      world_count: 0, resource_packs: 0, screenshots: 0, mod_count: 0, has_options: false, has_servers: false,
    });
    vi.spyOn(launcherAPI, "scanCurseForgePackZip").mockResolvedValue({
      ...plan, format: "modlist-html", files: [], unresolved: [{ file_name: "sodium", required: true, project_id: 0, file_id: 0 }],
      override_names: [], blocked_names: [], required_failed: 0,
    });

    render(() => <ImportInstanceModal isOpen={true} onClose={() => {}} />);
    fireEvent.click(screen.getByTestId("source-tab-curseforge"));
    fireEvent.input(screen.getByTestId("cf-zip-path-input"), { target: { value: "C:\\pack.zip" } });
    await new Promise((r) => setTimeout(r, 10)); // solid signal flush
    fireEvent.click(screen.getByTestId("cf-scan-btn"));

    await screen.findByTestId("cf-import-warning-banner");
    expect(screen.getByTestId("cf-import-warning-banner").textContent).toContain("modlist.html");
    expect(screen.getByText("modlist.html")).toBeTruthy();
  });
});
