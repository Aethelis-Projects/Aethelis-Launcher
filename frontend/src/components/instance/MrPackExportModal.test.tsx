import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MrPackExportModal } from "./MrPackExportModal";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO } from "../../bindings/ipc_types";

describe("MrPackExportModal Component (M1.3, R2)", () => {
  const mockInstance: InstanceDTO = {
    id: "speedrunner-1",
    name: "Speedrunner 1.21",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
    min_ram_mb: 2048,
    max_ram_mb: 4096,
    jvm_args: [],
    skip_java_check: false,
    state: "idle",
    total_play_seconds: 1200,
  };

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("does not render when isOpen is false", () => {
    render(() => (
      <MrPackExportModal
        instance={mockInstance}
        isOpen={false}
        onClose={() => {}}
      />
    ));
    expect(screen.queryByTestId("mrpack-export-modal")).toBeNull();
  });

  it("initializes form with instance values and allows editing", async () => {
    render(() => (
      <MrPackExportModal
        instance={mockInstance}
        isOpen={true}
        onClose={() => {}}
      />
    ));

    const nameInput = screen.getByTestId("mrpack-export-name-input") as HTMLInputElement;
    const versionInput = screen.getByTestId("mrpack-export-version-input") as HTMLInputElement;

    expect(nameInput.value).toBe("Speedrunner 1.21");
    expect(versionInput.value).toBe("1.0.0");

    fireEvent.input(nameInput, { target: { value: "Speedrunner Pro" } });
    fireEvent.input(versionInput, { target: { value: "2.0.0" } });

    expect(nameInput.value).toBe("Speedrunner Pro");
    expect(versionInput.value).toBe("2.0.0");
  });

  it("executes export call and renders success screen with file path", async () => {
    const exportSpy = vi
      .spyOn(launcherAPI, "exportMrPack")
      .mockResolvedValue("C:\\Exports\\Speedrunner-Pro.mrpack");
    const onExported = vi.fn();

    render(() => (
      <MrPackExportModal
        instance={mockInstance}
        isOpen={true}
        onClose={() => {}}
        onExported={onExported}
      />
    ));

    const submitBtn = screen.getByTestId("mrpack-export-submit-btn");
    fireEvent.click(submitBtn);

    await vi.waitFor(() => {
      expect(exportSpy).toHaveBeenCalledWith({
        instance_id: "speedrunner-1",
        name: "Speedrunner 1.21",
        version: "1.0.0",
        summary: "Speedrunner 1.21 for Minecraft 1.21.1",
      });
    });

    await vi.waitFor(() => {
      expect(screen.getByTestId("mrpack-export-success")).toBeTruthy();
    });

    expect(onExported).toHaveBeenCalledWith("C:\\Exports\\Speedrunner-Pro.mrpack");
    expect(screen.getByText("C:\\Exports\\Speedrunner-Pro.mrpack")).toBeTruthy();
  });

  it("renders error banner when export fails", async () => {
    vi.spyOn(launcherAPI, "exportMrPack").mockRejectedValue(new Error("Disk quota exceeded"));

    render(() => (
      <MrPackExportModal
        instance={mockInstance}
        isOpen={true}
        onClose={() => {}}
      />
    ));

    const submitBtn = screen.getByTestId("mrpack-export-submit-btn");
    fireEvent.click(submitBtn);

    await vi.waitFor(() => {
      expect(screen.getByTestId("mrpack-export-error")).toBeTruthy();
    });

    expect(screen.getByText(/Disk quota exceeded/)).toBeTruthy();
  });
});
