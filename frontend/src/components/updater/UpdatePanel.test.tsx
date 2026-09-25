import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { UpdatePanel } from "./UpdatePanel";
import { launcherAPI } from "../../services/api";

describe("UpdatePanel Component", () => {
  beforeEach(() => {
    launcherAPI.resetMockUpdater();
    vi.restoreAllMocks();
  });

  it("renders software updates card with installed version", async () => {
    render(() => <UpdatePanel channel="stable" />);
    expect(screen.getByText("Software Updates")).toBeTruthy();
    expect(screen.getByText("Channel: stable")).toBeTruthy();
    expect(await screen.findByText("v0.6.1")).toBeTruthy();
    expect(screen.getByTestId("check-updates-button").textContent).toContain("Check for updates");
  });

  it("shows up to date banner when no new version exists", async () => {
    launcherAPI.setMockUpdateInfo({
      has_update: false,
      version: "0.6.1",
      current_version: "0.6.1",
      release_date: "2026-09-25T12:00:00Z",
      release_notes: "Latest stable version",
      download_url: "",
      sha256: "",
      size: 0,
    });

    render(() => <UpdatePanel />);
    const checkBtn = screen.getByTestId("check-updates-button");
    fireEvent.click(checkBtn);

    const banner = await screen.findByTestId("up-to-date-banner");
    expect(banner.textContent).toContain("You are running the latest version");
    expect(screen.getByTestId("last-checked-time")).toBeTruthy();
  });

  it("displays update available card with changelog and download size", async () => {
    launcherAPI.setMockUpdateInfo({
      has_update: true,
      version: "0.6.2",
      current_version: "0.6.1",
      release_date: "2026-09-25T10:00:00Z",
      release_notes: "Nord Launcher v0.6.2: Performance improvements.",
      download_url: "https://example.com/download/NordLauncher.exe",
      sha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      size: 18350080, // ~17.5 MB
    });

    const onUpdateAvailable = vi.fn();
    render(() => <UpdatePanel onUpdateAvailable={onUpdateAvailable} />);

    const checkBtn = screen.getByTestId("check-updates-button");
    fireEvent.click(checkBtn);

    const card = await screen.findByTestId("update-available-card");
    expect(card.textContent).toContain("Nord Launcher v0.6.2");
    expect(card.textContent).toContain("17.50 MB");
    expect(screen.getByTestId("changelog-text").textContent).toContain("Performance improvements");
    expect(screen.getByTestId("ed25519-status-line").textContent).toContain("Цифровая подпись Ed25519 проверена");
    expect(onUpdateAvailable).toHaveBeenCalledTimes(1);
  });

  it("renders safe markdown-lite changelog without innerHTML (B2, C4)", async () => {
    launcherAPI.setMockUpdateInfo({
      has_update: true,
      version: "0.6.2",
      current_version: "0.6.1",
      release_date: "2026-09-24T12:00:00Z",
      release_notes: "### Added\n- Feature One\n- Feature Two\n\n### Fixed\n- Bug Fix",
      download_url: "https://example.com/download/NordLauncher.exe",
      sha256: "abc123",
      size: 1048576,
    });

    render(() => <UpdatePanel />);
    fireEvent.click(screen.getByTestId("check-updates-button"));

    await screen.findByTestId("update-available-card");
    const changelog = screen.getByTestId("changelog-text");
    expect(changelog.textContent).toContain("Added");
    expect(changelog.textContent).toContain("Feature One");
    expect(changelog.textContent).toContain("Feature Two");
    expect(changelog.textContent).toContain("Fixed");
    expect(changelog.textContent).toContain("Bug Fix");
    expect(screen.getByTestId("ed25519-status-line").textContent).toContain("Цифровая подпись Ed25519 проверена");
  });

  it("triggers applyUpdate on download button click and transitions to restart banner", async () => {
    launcherAPI.setMockUpdateInfo({
      has_update: true,
      version: "0.6.2",
      current_version: "0.6.1",
      release_date: "2026-09-25T10:00:00Z",
      release_notes: "Feature release",
      download_url: "https://example.com/download",
      sha256: "abc123",
      size: 1048576,
    });

    render(() => <UpdatePanel />);
    fireEvent.click(screen.getByTestId("check-updates-button"));

    const applyBtn = await screen.findByTestId("apply-update-button");
    fireEvent.click(applyBtn);

    const restartBanner = await screen.findByTestId("restart-required-banner");
    expect(restartBanner.textContent).toContain("Update installed successfully");
  });

  it("invokes restartApplication when restart button is clicked", async () => {
    launcherAPI.setMockUpdateInfo({
      has_update: true,
      version: "0.6.2",
      current_version: "0.6.1",
      release_date: "2026-09-25T10:00:00Z",
      release_notes: "Feature release",
      download_url: "https://example.com/download",
      sha256: "abc123",
      size: 1048576,
    });

    const restartSpy = vi.spyOn(launcherAPI, "restartApplication").mockResolvedValue(undefined);
    const onRestartTriggered = vi.fn();

    render(() => <UpdatePanel onRestartTriggered={onRestartTriggered} />);
    fireEvent.click(screen.getByTestId("check-updates-button"));

    const applyBtn = await screen.findByTestId("apply-update-button");
    fireEvent.click(applyBtn);

    const restartBtn = await screen.findByTestId("restart-launcher-button");
    fireEvent.click(restartBtn);

    expect(onRestartTriggered).toHaveBeenCalledTimes(1);
    expect(restartSpy).toHaveBeenCalledTimes(1);
  });

  it("handles check error gracefully and displays retry button", async () => {
    vi.spyOn(launcherAPI, "checkForUpdates").mockRejectedValue(new Error("Network connection timed out"));

    render(() => <UpdatePanel />);
    fireEvent.click(screen.getByTestId("check-updates-button"));

    const errorBanner = await screen.findByTestId("error-banner");
    expect(errorBanner.textContent).toContain("Network connection timed out");
    expect(screen.getByTestId("retry-button")).toBeTruthy();
  });
});
