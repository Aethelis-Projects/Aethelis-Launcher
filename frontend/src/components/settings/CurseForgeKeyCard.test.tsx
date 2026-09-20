import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { CurseForgeKeyCard } from "./CurseForgeKeyCard";
import { launcherAPI } from "../../services/api";

describe("CurseForgeKeyCard Component (B5, N6, R2)", () => {
  beforeEach(() => {
    launcherAPI.setMockSettings({});
    launcherAPI.setMockHasBuiltinCurseForgeKey(false);
    vi.restoreAllMocks();
  });

  it("renders with loaded key from settings and 32-hex placeholder", async () => {
    launcherAPI.setMockSettings({ curseforge_api_key: "$2a$10$existingkey" });

    render(() => <CurseForgeKeyCard />);

    const input = (await screen.findByTestId("cf-key-input")) as HTMLInputElement;
    await vi.waitFor(() => {
      expect(input.value).toBe("$2a$10$existingkey");
    });
    expect(input.type).toBe("password");
    expect(input.placeholder).toBe("ключ вида $2a$10… с console.curseforge.com (или оставьте пустым для встроенного)");
    const helper = screen.getByTestId("cf-key-helper-text");
    expect(helper.textContent).toContain("Сохранение пустого поля возвращает использование встроенного/сайдкар-ключа.");
  });

  it("toggles password visibility mask", async () => {
    render(() => <CurseForgeKeyCard />);

    const input = (await screen.findByTestId("cf-key-input")) as HTMLInputElement;
    expect(input.type).toBe("password");

    const toggleBtn = screen.getByTestId("cf-key-toggle-visibility");
    fireEvent.click(toggleBtn);
    expect(input.type).toBe("text");

    fireEvent.click(toggleBtn);
    expect(input.type).toBe("password");
  });

  it("saves entered key successfully", async () => {
    const setSettingSpy = vi.spyOn(launcherAPI, "setSetting").mockResolvedValue(undefined);

    render(() => <CurseForgeKeyCard />);

    const input = (await screen.findByTestId("cf-key-input")) as HTMLInputElement;
    fireEvent.input(input, { target: { value: "$2a$10$newcustomkey" } });

    const saveBtn = screen.getByTestId("cf-key-save-btn");
    fireEvent.click(saveBtn);

    const status = await screen.findByTestId("cf-key-status");
    expect(status.textContent).toContain("успешно сохранен");
    expect(setSettingSpy).toHaveBeenCalledWith("curseforge_api_key", "$2a$10$newcustomkey");
  });

  it("handles save error gracefully", async () => {
    vi.spyOn(launcherAPI, "setSetting").mockRejectedValue(new Error("disk full"));

    render(() => <CurseForgeKeyCard />);

    const saveBtn = screen.getByTestId("cf-key-save-btn");
    fireEvent.click(saveBtn);

    const status = await screen.findByTestId("cf-key-status");
    expect(status.textContent).toContain("Не удалось сохранить");
  });

  it("shows builtin key badge and collapses spoiler by default when builtin key exists (R2)", async () => {
    launcherAPI.setMockHasBuiltinCurseForgeKey(true);

    render(() => <CurseForgeKeyCard />);

    const badge = await screen.findByTestId("cf-builtin-badge");
    expect(badge.textContent).toContain("Встроенный ключ активен");

    // Input should be hidden initially in spoiler
    expect(screen.queryByTestId("cf-key-input")).toBeNull();

    // Toggle spoiler open
    const spoilerToggle = screen.getByTestId("cf-custom-key-spoiler-toggle");
    expect(spoilerToggle.textContent).toContain("Использовать свой ключ");
    fireEvent.click(spoilerToggle);

    const input = (await screen.findByTestId("cf-key-input")) as HTMLInputElement;
    expect(input).toBeTruthy();
    expect(input.placeholder).toBe("ключ вида $2a$10… с console.curseforge.com (или оставьте пустым для встроенного)");
  });

  it("opens spoiler automatically when builtin key is active but custom key is configured (R2)", async () => {
    launcherAPI.setMockHasBuiltinCurseForgeKey(true);
    launcherAPI.setMockSettings({ curseforge_api_key: "custom-overridden-key" });

    render(() => <CurseForgeKeyCard />);

    const badge = await screen.findByTestId("cf-builtin-badge");
    expect(badge.textContent).toContain("Встроенный ключ активен");

    const input = (await screen.findByTestId("cf-key-input")) as HTMLInputElement;
    await vi.waitFor(() => {
      expect(input.value).toBe("custom-overridden-key");
    });
  });

  it("collects diagnostic report and copies to clipboard", async () => {
    const getReportSpy = vi.spyOn(launcherAPI, "getDiagnosticReport").mockResolvedValue("=== Mock Diag Report ===");
    Object.assign(navigator, {
      clipboard: {
        writeText: vi.fn().mockResolvedValue(undefined),
      },
    });

    render(() => <CurseForgeKeyCard />);

    const diagBtn = await screen.findByTestId("diag-pack-btn");
    expect(diagBtn.textContent).toContain("Собрать диаг-пак");
    fireEvent.click(diagBtn);

    const status = await screen.findByTestId("diag-pack-status");
    expect(status.textContent).toContain("Диагностический отчёт скопирован в буфер обмена");
    expect(getReportSpy).toHaveBeenCalledWith("");
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("=== Mock Diag Report ===");
  });
});
