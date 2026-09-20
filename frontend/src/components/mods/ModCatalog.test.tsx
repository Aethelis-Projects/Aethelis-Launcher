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
    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({ items: [], total_count: 0 });

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
      .mockResolvedValueOnce({
        items: [
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
        ],
        total_count: 1,
      });

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

    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({ items: [mockMod], total_count: 1 });
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

    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({ items: [mockMod], total_count: 1 });

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

  it("displays rate-limit banner when searchMods fails with CF_RATE_LIMITED (D6)", async () => {
    vi.spyOn(launcherAPI, "searchMods").mockRejectedValue(
      new Error("CF_RATE_LIMITED: curseforge api rate limit exceeded (status 403)")
    );

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const errorBanner = await screen.findByTestId("mods-search-error-banner");
    expect(errorBanner.textContent).toContain("Встроенный ключ каталога временно недоступен");
  });

  it("displays missing sidecar message on 401/403 when no builtin key exists (A-остатки)", async () => {
    vi.spyOn(launcherAPI, "hasBuiltinCurseForgeKey").mockResolvedValue(false);
    vi.spyOn(launcherAPI, "searchMods").mockRejectedValue(
      new Error("curseforge: request returned status 403 Forbidden")
    );

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const errorBanner = await screen.findByTestId("mods-search-error-banner");
    expect(errorBanner.textContent).toContain("Встроенный ключ каталога временно недоступен");
  });

  it("displays invalid/exhausted message on 401/403 when builtin key exists (A-остатки)", async () => {
    vi.spyOn(launcherAPI, "hasBuiltinCurseForgeKey").mockResolvedValue(true);
    vi.spyOn(launcherAPI, "searchMods").mockRejectedValue(
      new Error("curseforge: request returned status 401 Unauthorized")
    );

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const errorBanner = await screen.findByTestId("mods-search-error-banner");
    expect(errorBanner.textContent).toContain("CurseForge временно ограничил запросы — попробуйте позже");
  });

  it("renders reactive sort options for Modrinth vs CurseForge and resets sort on switch (B5)", async () => {
    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({ items: [], total_count: 0 });

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const sortSelect = (await screen.findByTestId("mods-sort-select")) as HTMLSelectElement;
    // On Modrinth, should have "newest" option
    const optionsModrinth = Array.from(sortSelect.options).map((o) => o.value);
    expect(optionsModrinth).toContain("relevance");
    expect(optionsModrinth).toContain("downloads");
    expect(optionsModrinth).toContain("updated");
    expect(optionsModrinth).toContain("newest");
    expect(optionsModrinth).not.toContain("popularity");

    // Select "newest"
    fireEvent.change(sortSelect, { target: { value: "newest" } });
    expect(sortSelect.value).toBe("newest");

    // Switch to CurseForge
    const cfBtn = screen.getByText("CurseForge");
    fireEvent.click(cfBtn);

    // On CurseForge, options should not have "newest", should have "popularity"
    const optionsCF = Array.from(sortSelect.options).map((o) => o.value);
    expect(optionsCF).toContain("relevance");
    expect(optionsCF).toContain("popularity");
    expect(optionsCF).toContain("updated");
    expect(optionsCF).toContain("downloads");
    expect(optionsCF).not.toContain("newest");

    // Should have reset from "newest" to "relevance"
    expect(sortSelect.value).toBe("relevance");
  });

  it("displays rate limit banner with active countdown and fallback button on rate_limited reason (B4, C3)", async () => {
    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({
      items: [],
      total_count: 0,
      reason: "rate_limited",
      retry_after_seconds: 15,
    });

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    // Switch to CurseForge
    const cfBtn = screen.getByText("CurseForge");
    fireEvent.click(cfBtn);

    const banner = await screen.findByTestId("mods-rate-limit-banner");
    expect(banner.textContent).toContain("CurseForge ограничил частоту запросов — повторим через 15 с");

    const fallbackBtn = await screen.findByTestId("fallback-to-modrinth-btn");
    expect(fallbackBtn.textContent).toContain("Искать это же на Modrinth");
  });

  it("displays key invalid banner with fallback button on key_invalid reason (C3)", async () => {
    vi.spyOn(launcherAPI, "searchMods").mockResolvedValue({
      items: [],
      total_count: 0,
      reason: "key_invalid",
    });

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    // Switch to CurseForge
    const cfBtn = screen.getByText("CurseForge");
    fireEvent.click(cfBtn);

    const banner = await screen.findByTestId("mods-key-invalid-banner");
    expect(banner.textContent).toContain("Ключ каталога отклонён сервером — это внутренняя проблема, обновите лаунчер");

    const fallbackBtn = await screen.findByTestId("fallback-to-modrinth-btn");
    expect(fallbackBtn.textContent).toContain("Искать это же на Modrinth");
  });

  it("clicking fallback button switches to Modrinth and triggers search (C3)", async () => {
    const searchSpy = vi.spyOn(launcherAPI, "searchMods")
      .mockResolvedValueOnce({ items: [], total_count: 0 }) // initial Modrinth
      .mockResolvedValueOnce({
        items: [],
        total_count: 0,
        reason: "rate_limited",
        retry_after_seconds: 10,
      }) // CurseForge
      .mockResolvedValueOnce({
        items: [
          {
            id: "mod-mr",
            slug: "sodium",
            source: "modrinth",
            name: "Sodium",
            author: "jellysquid",
            summary: "Optimization mod",
            downloads: 100000,
            categories: ["optimization"],
          },
        ],
        total_count: 1,
      }); // Back to Modrinth via fallback

    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    // Switch to CurseForge
    const cfBtn = screen.getByText("CurseForge");
    fireEvent.click(cfBtn);

    const fallbackBtn = await screen.findByTestId("fallback-to-modrinth-btn");
    fireEvent.click(fallbackBtn);

    const modCard = await screen.findByText("Sodium");
    expect(modCard).toBeTruthy();
    expect(screen.queryByTestId("mods-rate-limit-banner")).toBeNull();
    expect(searchSpy).toHaveBeenCalledTimes(3);
  });
});