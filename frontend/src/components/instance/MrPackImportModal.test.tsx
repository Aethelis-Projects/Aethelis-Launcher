import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MrPackImportModal } from "./MrPackImportModal";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO, MrPackImportPlanDTO } from "../../bindings/ipc_types";

describe("MrPackImportModal Component (M1.3, R1, R2)", () => {
  const mockPlan: MrPackImportPlanDTO = {
    name: "Speedrunner Pack",
    summary: "Fast load times and high fps",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
    total_files: 8,
    total_size: 12500000,
    dependencies: {},
  };

  const mockCreatedInstance: InstanceDTO = {
    id: "speedrunner-pack",
    name: "Speedrunner Pack",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
    min_ram_mb: 2048,
    max_ram_mb: 4096,
    jvm_args: [],
    skip_java_check: false,
    state: "idle",
    total_play_seconds: 0,
  };

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("does not render when isOpen is false", () => {
    render(() => <MrPackImportModal isOpen={false} onClose={() => {}} />);
    expect(screen.queryByTestId("mrpack-import-modal")).toBeNull();
  });

  it("browses for file, fetches plan preview and updates instance name", async () => {
    vi.spyOn(launcherAPI, "pickMrPackFile").mockResolvedValue("C:\\Downloads\\pack.mrpack");
    const planSpy = vi.spyOn(launcherAPI, "getMrPackImportPlan").mockResolvedValue(mockPlan);

    render(() => <MrPackImportModal isOpen={true} onClose={() => {}} />);

    const browseBtn = screen.getByTestId("mrpack-browse-btn");
    fireEvent.click(browseBtn);

    await vi.waitFor(() => {
      expect(planSpy).toHaveBeenCalledWith("C:\\Downloads\\pack.mrpack");
    });

    await vi.waitFor(() => {
      expect(screen.getByTestId("mrpack-plan-card")).toBeTruthy();
    });

    expect(screen.getByText("Speedrunner Pack")).toBeTruthy();
    expect(screen.getByText("Fast load times and high fps")).toBeTruthy();
    expect(screen.getByText("1.21.1")).toBeTruthy();

    const nameInput = screen.getByTestId("mrpack-instance-name-input") as HTMLInputElement;
    expect(nameInput.value).toBe("Speedrunner Pack");
  });

  it("executes import pipeline, polls status and renders success screen", async () => {
    vi.spyOn(launcherAPI, "getMrPackImportPlan").mockResolvedValue(mockPlan);
    const importSpy = vi.spyOn(launcherAPI, "importMrPack").mockResolvedValue(mockCreatedInstance);

    let pollCount = 0;
    vi.spyOn(launcherAPI, "getMrPackImportStatus").mockImplementation(async () => {
      pollCount++;
      if (pollCount === 1) {
        return {
          task_id: "import-task",
          status: "downloading",
          percentage: 45,
          current_file: "sodium.jar",
          files_done: 4,
          total_files: 8,
          bytes_read: 5000000,
          total_bytes: 12500000,
        };
      }
      return {
        task_id: "import-task",
        status: "complete",
        percentage: 100,
        current_file: "",
        files_done: 8,
        total_files: 8,
        bytes_read: 12500000,
        total_bytes: 12500000,
      };
    });

    const onImported = vi.fn();
    render(() => (
      <MrPackImportModal
        isOpen={true}
        onClose={() => {}}
        onImported={onImported}
      />
    ));

    const pathInput = screen.getByTestId("mrpack-path-input");
    fireEvent.input(pathInput, { target: { value: "C:\\Downloads\\pack.mrpack" } });
    fireEvent.change(pathInput);

    await vi.waitFor(() => {
      expect(screen.getByTestId("mrpack-plan-card")).toBeTruthy();
    });

    const submitBtn = screen.getByTestId("mrpack-submit-btn");
    fireEvent.click(submitBtn);

    await vi.waitFor(() => {
      expect(importSpy).toHaveBeenCalledWith({
        mrpack_path: "C:\\Downloads\\pack.mrpack",
        instance_name: "Speedrunner Pack",
      });
    });

    await vi.waitFor(() => {
      expect(screen.getByTestId("mrpack-import-success")).toBeTruthy();
    });

    expect(onImported).toHaveBeenCalledWith(mockCreatedInstance);
    expect(screen.getByText("Сборка успешно импортирована!")).toBeTruthy();
  });

  it("displays error banner when import fails", async () => {
    vi.spyOn(launcherAPI, "getMrPackImportPlan").mockResolvedValue(mockPlan);
    vi.spyOn(launcherAPI, "importMrPack").mockRejectedValue(new Error("Zip-slip detected in mrpack"));

    render(() => <MrPackImportModal isOpen={true} onClose={() => {}} />);

    const pathInput = screen.getByTestId("mrpack-path-input");
    fireEvent.input(pathInput, { target: { value: "C:\\Downloads\\malicious.mrpack" } });
    fireEvent.change(pathInput);

    await vi.waitFor(() => {
      expect(screen.getByTestId("mrpack-plan-card")).toBeTruthy();
    });

    const submitBtn = screen.getByTestId("mrpack-submit-btn");
    fireEvent.click(submitBtn);

    await vi.waitFor(() => {
      expect(screen.getByTestId("mrpack-import-error")).toBeTruthy();
    });

    expect(screen.getByText(/Zip-slip detected in mrpack/)).toBeTruthy();
  });
});
