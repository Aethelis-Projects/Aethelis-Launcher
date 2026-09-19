import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { JavaManager } from "./JavaManager";
import { launcherAPI } from "../../services/api";
import type { JavaInstallationDTO } from "../../bindings/ipc_types";

describe("JavaManager Component (J1)", () => {
  const mockRuntimes: JavaInstallationDTO[] = [
    {
      path: "C:\\Nord\\runtimes\\adoptium-21-21.0.2\\bin\\java.exe",
      home_dir: "C:\\Nord\\runtimes\\adoptium-21-21.0.2",
      major_version: 21,
      full_version: "21.0.2",
      vendor: "Eclipse Adoptium",
      kind: "managed",
      used_by: ["Nordic 1.21"],
    },
    {
      path: "C:\\Nord\\runtimes\\adoptium-17-17.0.10\\bin\\java.exe",
      home_dir: "C:\\Nord\\runtimes\\adoptium-17-17.0.10",
      major_version: 17,
      full_version: "17.0.10",
      vendor: "Eclipse Adoptium",
      kind: "managed",
      used_by: [],
    },
    {
      path: "C:\\Program Files\\Java\\jdk-8\\bin\\java.exe",
      home_dir: "C:\\Program Files\\Java\\jdk-8",
      major_version: 8,
      full_version: "1.8.0_391",
      vendor: "Oracle Corporation",
      kind: "detected",
      used_by: [],
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "listJavaRuntimes").mockResolvedValue(mockRuntimes);
    vi.spyOn(launcherAPI, "getJavaDownloadStatus").mockResolvedValue({
      task_id: "",
      major: 0,
      status: "idle",
      bytes_read: 0,
      total_bytes: 0,
      percentage: 0,
    });
  });

  it("loads and displays discovered and managed Java runtimes on mount", async () => {
    render(() => <JavaManager />);

    await vi.waitFor(() => {
      expect(screen.getByTestId("java-runtimes-list")).toBeTruthy();
    });

    expect(screen.getByText(/Eclipse Adoptium 21.0.2/)).toBeTruthy();
    expect(screen.getByText(/Oracle Corporation 1.8.0_391/)).toBeTruthy();
    expect(screen.getByText("Nordic 1.21")).toBeTruthy();
  });

  it("triggers Adoptium JDK download when download button is clicked", async () => {
    const downloadSpy = vi.spyOn(launcherAPI, "downloadJavaRuntime").mockResolvedValue();
    vi.spyOn(launcherAPI, "getJavaDownloadStatus").mockResolvedValue({
      task_id: "task-test-21",
      major: 21,
      status: "downloading",
      bytes_read: 50 * 1024 * 1024,
      total_bytes: 100 * 1024 * 1024,
      percentage: 50,
    });

    render(() => <JavaManager />);

    const dlBtn = await screen.findByTestId("download-java-21-button");
    fireEvent.click(dlBtn);

    await vi.waitFor(() => {
      expect(downloadSpy).toHaveBeenCalledWith(21);
    });

    const progress = await screen.findByTestId("java-download-progress");
    expect(progress.textContent).toContain("50%");
  });

  it("adds external custom Java runtime path", async () => {
    const addSpy = vi.spyOn(launcherAPI, "addJavaRuntime").mockResolvedValue({
      path: "C:\\Custom\\Java\\bin\\java.exe",
      home_dir: "C:\\Custom\\Java",
      major_version: 21,
      full_version: "21.0.2",
      vendor: "Custom",
      kind: "detected",
      used_by: [],
    });

    render(() => <JavaManager />);

    const input = await screen.findByTestId("add-java-path-input");
    const addBtn = await screen.findByTestId("add-java-path-button");

    fireEvent.input(input, { target: { value: "C:\\Custom\\Java" } });
    fireEvent.click(addBtn);

    await vi.waitFor(() => {
      expect(addSpy).toHaveBeenCalledWith("C:\\Custom\\Java");
    });
  });

  it("blocks deletion of runtime when used by an instance", async () => {
    render(() => <JavaManager />);

    await vi.waitFor(() => {
      expect(screen.getByTestId("delete-runtime-21")).toBeTruthy();
    });

    const del21 = screen.getByTestId("delete-runtime-21") as HTMLButtonElement;
    expect(del21.disabled).toBe(true);

    const del17 = screen.getByTestId("delete-runtime-17") as HTMLButtonElement;
    expect(del17.disabled).toBe(false);
  });

  it("removes unused managed runtime when delete button is clicked", async () => {
    const removeSpy = vi.spyOn(launcherAPI, "removeJavaRuntime").mockResolvedValue();

    render(() => <JavaManager />);

    const del17 = await screen.findByTestId("delete-runtime-17");
    fireEvent.click(del17);

    await vi.waitFor(() => {
      expect(removeSpy).toHaveBeenCalledWith("C:\\Nord\\runtimes\\adoptium-17-17.0.10\\bin\\java.exe");
    });
  });
});
