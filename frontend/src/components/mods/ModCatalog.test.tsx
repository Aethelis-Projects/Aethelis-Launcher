import { render, screen, fireEvent } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ModCatalog } from "./ModCatalog";
import { launcherAPI } from "../../services/api";
import type { ModItemDTO } from "../../bindings/ipc_types";

describe("ModCatalog Component", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders catalog header and source toggle buttons", async () => {
    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    expect(screen.getByText("Каталог модификаций")).toBeTruthy();
    expect(screen.getByText("Modrinth")).toBeTruthy();
    expect(screen.getByText("CurseForge")).toBeTruthy();
  });

  it("switches source tab on click", async () => {
    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const cfBtn = screen.getByText("CurseForge");
    fireEvent.click(cfBtn);
    expect(cfBtn.className).toContain("bg-[#00D4B2]");
  });

  it("displays search error banner when searchMods fails (C1)", async () => {
    vi.spyOn(launcherAPI, "searchMods").mockRejectedValue(
      new Error("curseforge: API key is not configured (set CURSEFORGE_API_KEY environment variable)")
    );

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const errorBanner = await screen.findByTestId("mods-search-error-banner");
    expect(errorBanner.textContent).toContain("API key is not configured");
    expect(screen.queryByTestId("mods-empty-state")).toBeNull();
    expect(screen.queryByTestId("mods-loading-indicator")).toBeNull();
  });

  it("displays empty state when searchMods returns empty list", async () => {
    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue([]);

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const emptyState = await screen.findByTestId("mods-empty-state");
    expect(emptyState.textContent).toContain("Модификации по запросу не найдены.");
    expect(screen.queryByTestId("mods-search-error-banner")).toBeNull();
  });

  it("clears search error when switching source tab to successful source", async () => {
    const searchSpy = vi.spyOn(launcherAPI, "searchMods")
      .mockRejectedValueOnce(new Error("CF network failure"))
      .mockResolvedValueOnce([
        {
          id: "mod-1",
          slug: "sodium",
          source: "modrinth",
          name: "Sodium",
          author: "jellysquid",
          summary: "Modern rendering engine",
          downloads: 5000000,
          categories: ["optimization"],
        },
      ]);

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const errorBanner = await screen.findByTestId("mods-search-error-banner");
    expect(errorBanner.textContent).toContain("CF network failure");

    // Switch source to CurseForge or Modrinth
    const cfBtn = screen.getByText("CurseForge");
    fireEvent.click(cfBtn);

    const modCard = await screen.findByText("Sodium");
    expect(modCard).toBeTruthy();
    expect(screen.queryByTestId("mods-search-error-banner")).toBeNull();
    expect(searchSpy).toHaveBeenCalledTimes(2);
  });

  it("handles installMod failure gracefully and displays install error banner (C3)", async () => {
    const mockMod: ModItemDTO = {
      id: "mod-test-1",
      slug: "test-mod",
      source: "modrinth",
      name: "Test Mod",
      author: "tester",
      summary: "Test mod summary",
      downloads: 100,
      categories: ["utility"],
    };

    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue([mockMod]);
    vi.spyOn(launcherAPI, "installMod").mockRejectedValue(
      new Error("Demo mode: mod installation backend ships in v0.2.0")
    );

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const installBtn = await screen.findByText("Установить");
    fireEvent.click(installBtn);

    const installBanner = await screen.findByTestId("mods-install-error-banner");
    expect(installBanner.textContent).toContain("Demo mode: mod installation backend ships in v0.2.0");
    // Ensure button did not transition to fake "Установлен"
    expect(screen.queryByText("Установлен")).toBeNull();
  });

  it("installs mod successfully and triggers onModInstalled callback", async () => {
    const mockMod: ModItemDTO = {
      id: "mod-test-success",
      slug: "success-mod",
      source: "modrinth",
      name: "Success Mod",
      author: "tester",
      summary: "Success test mod summary",
      downloads: 200,
      categories: ["optimization"],
    };

    const installSpy = vi.spyOn(launcherAPI, "installMod").mockResolvedValue({
      success: true,
      file_name: "success-mod-1.0.0.jar",
      message: "Mod success-mod-1.0.0.jar installed successfully",
    });
    const onModInstalled = vi.fn();

    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue([mockMod]);

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
        onModInstalled={onModInstalled}
      />
    ));

    const installBtn = await screen.findByText("Установить");
    fireEvent.click(installBtn);

    await screen.findByText("Установлен");
    expect(installSpy).toHaveBeenCalledWith("test-inst", mockMod);
    expect(onModInstalled).toHaveBeenCalledWith(mockMod);
    expect(screen.queryByTestId("mods-install-error-banner")).toBeNull();
  });
});