import { render, fireEvent, screen, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { CreateInstanceModal } from "./CreateInstanceModal";
import { launcherAPI } from "../../services/api";
import type { GameVersionDTO, InstanceDTO, LoaderResolutionDTO } from "../../bindings/ipc_types";

const versions: GameVersionDTO[] = [
  { id: "26.3", type: "release", release_time: "2026-09-01T00:00:00Z" },
  { id: "1.21.4", type: "release", release_time: "2024-12-03T00:00:00Z" },
];

const fabricRes: LoaderResolutionDTO = {
  loader: "fabric",
  default: "0.16.4",
  options: ["0.16.9", "0.16.4"],
  source: "fabric",
};

const created: InstanceDTO = {
  id: "pack-1",
  name: "pack",
  game_version: "26.3",
  loader: "fabric",
  loader_version: "0.16.4",
  min_ram_mb: 2048,
  max_ram_mb: 4096,
  jvm_args: [],
  skip_java_check: false,
  state: "idle",
  total_play_seconds: 0,
};

describe("CreateInstanceModal wizard (v0.7.2 G10)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "listMinecraftVersions").mockResolvedValue(versions);
    vi.spyOn(launcherAPI, "listLoaderVersions").mockResolvedValue(fabricRes);
  });

  it("loads the release catalog and preselects the newest version", async () => {
    render(() => <CreateInstanceModal isOpen onClose={() => {}} />);
    expect(launcherAPI.listMinecraftVersions).toHaveBeenCalledWith({ channel: "release" });
    await waitFor(() => {
      const sel = screen.getByTestId("wizard-version-select") as HTMLSelectElement;
      expect(sel.value).toBe("26.3");
    });
  });

  it("creates an instance with the recommended loader version", async () => {
    const createSpy = vi.spyOn(launcherAPI, "createInstanceWithLoader").mockResolvedValue(created);
    const onCreated = vi.fn();
    render(() => <CreateInstanceModal isOpen onClose={() => {}} onCreated={onCreated} />);

    fireEvent.input(screen.getByTestId("wizard-name-input"), { target: { value: "pack" } });
    await waitFor(() => expect(launcherAPI.listLoaderVersions).toHaveBeenCalled());
    await waitFor(() => {
      const sel = screen.getByTestId("wizard-loader-version-select") as HTMLSelectElement;
      expect(sel.value).toBe("0.16.4");
    });

    fireEvent.click(screen.getByTestId("wizard-create-btn"));
    await waitFor(() => {
      expect(createSpy).toHaveBeenCalledWith({
        name: "pack",
        game_version: "26.3",
        loader: "fabric",
        loader_version: "0.16.4",
      });
    });
    expect(onCreated).toHaveBeenCalledWith(created);
  });

  it("vanilla skips loader resolution entirely", async () => {
    const createSpy = vi.spyOn(launcherAPI, "createInstanceWithLoader").mockResolvedValue({ ...created, loader: "vanilla", loader_version: undefined });
    render(() => <CreateInstanceModal isOpen onClose={() => {}} />);
    await waitFor(() => {
      const sel = screen.getByTestId("wizard-version-select") as HTMLSelectElement;
      expect(sel.value).toBe("26.3");
    });
    fireEvent.input(screen.getByTestId("wizard-name-input"), { target: { value: "plain" } });
    fireEvent.click(screen.getByTestId("wizard-loader-vanilla"));
    expect(screen.queryByTestId("wizard-loader-version-select")).toBeNull();

    fireEvent.click(screen.getByTestId("wizard-create-btn"));
    await waitFor(() => expect(createSpy).toHaveBeenCalled());
    expect(createSpy.mock.calls[0][0].loader_version).toBeUndefined();
  });

  it("disables create until a name is typed", async () => {
    render(() => <CreateInstanceModal isOpen onClose={() => {}} />);
    await waitFor(() => expect(launcherAPI.listMinecraftVersions).toHaveBeenCalled());
    expect((screen.getByTestId("wizard-create-btn") as HTMLButtonElement).disabled).toBe(true);
  });
});
