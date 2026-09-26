import { render, screen, fireEvent, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { GameConsoleModal } from "./GameConsoleModal";
import type { InstanceDTO } from "../../bindings/ipc_types";
import { launcherAPI } from "../../services/api";

describe("GameConsoleModal Component", () => {
  const mockInstance: InstanceDTO = {
    id: "test-console-inst",
    name: "Test World 1.21",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
    min_ram_mb: 2048,
    max_ram_mb: 4096,
    jvm_args: [],
    skip_java_check: false,
    group: "Survival",
    is_favorite: true,
    state: "running",
    total_play_seconds: 500,
  };

  const sampleLogs = [
    "[12:00:00] [main/INFO]: Loading Minecraft 1.21.1",
    "[12:00:01] [main/WARN]: Deprecated shader syntax detected",
    "[12:00:02] [main/ERROR]: Failed to load custom texture pack",
    "[12:00:03] [main/INFO]: Game initialized successfully",
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "getGameLogs").mockResolvedValue([...sampleLogs]);
    vi.spyOn(launcherAPI, "saveGameLog").mockResolvedValue({
      success: true,
      file_path: "C:\\Nord\\instances\\test-console-inst\\logs\\saved.txt",
    });
  });

  it("renders console modal and loads logs when open", async () => {
    const onClose = vi.fn();
    render(() => (
      <GameConsoleModal
        isOpen={true}
        onClose={onClose}
        instance={mockInstance}
      />
    ));

    expect(screen.getByTestId("game-console-modal")).toBeTruthy();
    expect(screen.getByText(/Консоль процесса: Test World 1.21/)).toBeTruthy();

    await waitFor(() => {
      expect(screen.getByText(/Loading Minecraft 1.21.1/)).toBeTruthy();
      expect(screen.getByText(/Deprecated shader syntax detected/)).toBeTruthy();
      expect(screen.getByText(/Failed to load custom texture pack/)).toBeTruthy();
    });
  });

  it("filters logs by search query", async () => {
    render(() => (
      <GameConsoleModal
        isOpen={true}
        onClose={() => {}}
        instance={mockInstance}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText(/Loading Minecraft 1.21.1/)).toBeTruthy();
    });

    const searchInput = screen.getByTestId("console-search-input");
    fireEvent.input(searchInput, { target: { value: "shader" } });

    await waitFor(() => {
      expect(screen.getByText(/Deprecated shader syntax detected/)).toBeTruthy();
      expect(screen.queryByText(/Loading Minecraft 1.21.1/)).toBeNull();
    });
  });

  it("filters logs by log level buttons", async () => {
    render(() => (
      <GameConsoleModal
        isOpen={true}
        onClose={() => {}}
        instance={mockInstance}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText(/Loading Minecraft 1.21.1/)).toBeTruthy();
    });

    // Filter by ERROR
    const errorBtn = screen.getByTestId("console-level-error");
    fireEvent.click(errorBtn);

    await waitFor(() => {
      expect(screen.getByText(/Failed to load custom texture pack/)).toBeTruthy();
      expect(screen.queryByText(/Loading Minecraft 1.21.1/)).toBeNull();
      expect(screen.queryByText(/Deprecated shader syntax detected/)).toBeNull();
    });

    // Filter by WARN
    const warnBtn = screen.getByTestId("console-level-warn");
    fireEvent.click(warnBtn);

    await waitFor(() => {
      expect(screen.getByText(/Deprecated shader syntax detected/)).toBeTruthy();
      expect(screen.queryByText(/Failed to load custom texture pack/)).toBeNull();
    });

    // Reset to ALL
    const allBtn = screen.getByTestId("console-level-all");
    fireEvent.click(allBtn);

    await waitFor(() => {
      expect(screen.getByText(/Loading Minecraft 1.21.1/)).toBeTruthy();
      expect(screen.getByText(/Deprecated shader syntax detected/)).toBeTruthy();
    });
  });

  it("clears logs when clear button is clicked", async () => {
    render(() => (
      <GameConsoleModal
        isOpen={true}
        onClose={() => {}}
        instance={mockInstance}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText(/Loading Minecraft 1.21.1/)).toBeTruthy();
    });

    const clearBtn = screen.getByTestId("console-clear-btn");
    fireEvent.click(clearBtn);

    await waitFor(() => {
      expect(screen.getByText(/Журнал пуст/)).toBeTruthy();
      expect(screen.queryByText(/Loading Minecraft 1.21.1/)).toBeNull();
    });
  });

  it("saves logs when save button is clicked", async () => {
    render(() => (
      <GameConsoleModal
        isOpen={true}
        onClose={() => {}}
        instance={mockInstance}
      />
    ));

    await waitFor(() => {
      expect(screen.getByText(/Loading Minecraft 1.21.1/)).toBeTruthy();
    });

    const saveBtn = screen.getByTestId("console-save-btn");
    fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(launcherAPI.saveGameLog).toHaveBeenCalledWith({
        instance_id: mockInstance.id,
      });
      expect(screen.getByText(/Сохранено: C:\\Nord\\instances\\test-console-inst\\logs\\saved.txt/)).toBeTruthy();
    });
  });

  it("shows crash diagnostic button and banner when instance is crashed", () => {
    const crashedInstance = { ...mockInstance, state: "crashed" as const };
    const onOpenCrash = vi.fn();

    render(() => (
      <GameConsoleModal
        isOpen={true}
        onClose={() => {}}
        instance={crashedInstance}
        onOpenCrash={onOpenCrash}
      />
    ));

    expect(screen.getByText(/Игра аварийно завершилась/)).toBeTruthy();
    const diagBtn = screen.getByTestId("console-crash-diag-btn");
    fireEvent.click(diagBtn);
    expect(onOpenCrash).toHaveBeenCalledTimes(1);
  });
});
