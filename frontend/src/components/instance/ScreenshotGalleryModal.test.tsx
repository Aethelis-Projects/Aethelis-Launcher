import { render, screen, fireEvent, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ScreenshotGalleryModal } from "./ScreenshotGalleryModal";
import { launcherAPI } from "../../services/api";
import { ScreenshotDTO } from "../../bindings/ipc_types";

describe("ScreenshotGalleryModal", () => {
  const mockScreenshots: ScreenshotDTO[] = [
    {
      file_name: "2026-09-20_15.30.00.png",
      path: "instances/test-inst/screenshots/2026-09-20_15.30.00.png",
      size: 1048576,
      created_at: "2026-09-20T15:30:00Z",
    },
    {
      file_name: "2026-09-21_12.00.00.png",
      path: "instances/test-inst/screenshots/2026-09-21_12.00.00.png",
      size: 2097152,
      created_at: "2026-09-21T12:00:00Z",
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(launcherAPI, "listScreenshots").mockResolvedValue(mockScreenshots);
    vi.spyOn(launcherAPI, "getScreenshotData").mockResolvedValue({
      data_url: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
    });
    vi.spyOn(launcherAPI, "deleteScreenshot").mockResolvedValue();
    vi.spyOn(launcherAPI, "openPath").mockResolvedValue();
  });

  it("renders screenshots grid with files metadata", async () => {
    render(() => (
      <ScreenshotGalleryModal
        isOpen={true}
        instanceId="test-inst"
        instanceName="Test Instance"
        onClose={vi.fn()}
      />
    ));

    expect(await screen.findByText("2026-09-20_15.30.00.png")).toBeTruthy();
    expect(screen.getByText("2026-09-21_12.00.00.png")).toBeTruthy();
    expect(screen.getByText("1.0 МБ")).toBeTruthy();
    expect(screen.getByText("2.0 МБ")).toBeTruthy();
  });

  it("copies to clipboard using ClipboardItem when available", async () => {
    const writeMock = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, {
      clipboard: {
        write: writeMock,
      },
    });

    // Mock ClipboardItem constructor
    class MockClipboardItem {
      data: Record<string, Blob>;
      constructor(data: Record<string, Blob>) {
        this.data = data;
      }
    }
    (window as unknown as { ClipboardItem: unknown }).ClipboardItem = MockClipboardItem;

    render(() => (
      <ScreenshotGalleryModal
        isOpen={true}
        instanceId="test-inst"
        instanceName="Test Instance"
        onClose={vi.fn()}
      />
    ));

    const copyBtn = await screen.findByTestId("copy-btn-2026-09-20_15.30.00.png");
    fireEvent.click(copyBtn);

    await waitFor(() => {
      expect(writeMock).toHaveBeenCalled();
      expect(screen.getByTestId("gallery-toast").textContent).toContain("скопирован в буфер");
    });
  });

  it("falls back to openPath when clipboard fails", async () => {
    const openPathSpy = vi.spyOn(launcherAPI, "openPath").mockResolvedValue();
    // Simulate failing clipboard
    Object.assign(navigator, {
      clipboard: {
        write: vi.fn().mockRejectedValue(new Error("Permission denied")),
      },
    });

    render(() => (
      <ScreenshotGalleryModal
        isOpen={true}
        instanceId="test-inst"
        instanceName="Test Instance"
        onClose={vi.fn()}
      />
    ));

    const copyBtn = await screen.findByTestId("copy-btn-2026-09-20_15.30.00.png");
    fireEvent.click(copyBtn);

    await waitFor(() => {
      expect(openPathSpy).toHaveBeenCalledWith("instances/test-inst/screenshots/2026-09-20_15.30.00.png");
      expect(screen.getByTestId("gallery-toast").textContent).toContain("Файл открыт в папке");
    });
  });

  it("deletes screenshot and removes it from the list", async () => {
    const deleteSpy = vi.spyOn(launcherAPI, "deleteScreenshot").mockResolvedValue();

    render(() => (
      <ScreenshotGalleryModal
        isOpen={true}
        instanceId="test-inst"
        instanceName="Test Instance"
        onClose={vi.fn()}
      />
    ));

    const deleteBtn = await screen.findByTestId("delete-btn-2026-09-20_15.30.00.png");
    fireEvent.click(deleteBtn);

    await waitFor(() => {
      expect(deleteSpy).toHaveBeenCalledWith({
        instance_id: "test-inst",
        file_name: "2026-09-20_15.30.00.png",
      });
      expect(screen.queryByText("2026-09-20_15.30.00.png")).toBeNull();
    });
  });

  it("calls openPath when folder button in header is clicked", async () => {
    const openPathSpy = vi.spyOn(launcherAPI, "openPath").mockResolvedValue();

    render(() => (
      <ScreenshotGalleryModal
        isOpen={true}
        instanceId="test-inst"
        instanceName="Test Instance"
        onClose={vi.fn()}
      />
    ));

    const folderBtn = await screen.findByTestId("gallery-open-folder-btn");
    fireEvent.click(folderBtn);

    expect(openPathSpy).toHaveBeenCalledWith("test-inst");
  });
});
