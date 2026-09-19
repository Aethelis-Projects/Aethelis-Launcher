import { render, screen, fireEvent } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { AccountManager } from "./AccountManager";
import { launcherAPI } from "../../services/api";

describe("AccountManager Component (F4, R1)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("renders accounts header and login buttons", () => {
    render(() => <AccountManager />);
    expect(screen.getByText("Управление аккаунтами")).toBeTruthy();
    expect(screen.getByText("Офлайн аккаунт")).toBeTruthy();
    expect(screen.getByText("Войти через Microsoft")).toBeTruthy();
  });

  it("opens offline modal when offline button clicked", () => {
    render(() => <AccountManager />);
    const offlineBtn = screen.getByText("Офлайн аккаунт");
    fireEvent.click(offlineBtn);

    expect(screen.getByText("Добавить офлайн-аккаунт")).toBeTruthy();
    expect(screen.getByPlaceholderText("Введите никнейм игрока...")).toBeTruthy();
  });

  it("displays honest capability badge for offline accounts", async () => {
    vi.spyOn(launcherAPI, "listAccounts").mockResolvedValue([
      {
        uuid: "offline-1",
        username: "LocalMiner",
        type: "offline",
        is_active: true,
      },
    ]);

    render(() => <AccountManager />);

    const badge = await screen.findByTestId("offline-capability-badge");
    expect(badge.textContent).toContain("Одиночная игра и серверы без проверки лицензии");
  });

  it("renders 1-click fallback button on Microsoft login error when no offline account exists", async () => {
    vi.spyOn(launcherAPI, "listAccounts").mockResolvedValue([
      {
        uuid: "ms-1",
        username: "MsUser",
        type: "microsoft",
        is_active: true,
      },
    ]);
    vi.spyOn(launcherAPI, "loginMicrosoft").mockRejectedValue(
      new Error("User does not have an Xbox profile")
    );

    render(() => <AccountManager />);
    const msBtn = screen.getByText("Войти через Microsoft");
    fireEvent.click(msBtn);

    const banner = await screen.findByTestId("ms-login-error-banner");
    expect(banner.textContent).toContain("does not have an Xbox profile");

    const fallbackBtn = screen.getByTestId("create-offline-fallback-btn");
    expect(fallbackBtn).toBeTruthy();

    fireEvent.click(fallbackBtn);
    expect(screen.getByText("Добавить офлайн-аккаунт")).toBeTruthy();
  });

  it("hides fallback button on Microsoft login error if offline account already exists", async () => {
    vi.spyOn(launcherAPI, "listAccounts").mockResolvedValue([
      {
        uuid: "offline-1",
        username: "OfflineSteve",
        type: "offline",
        is_active: false,
      },
    ]);
    vi.spyOn(launcherAPI, "loginMicrosoft").mockRejectedValue(
      new Error("Network timeout")
    );

    render(() => <AccountManager />);
    const msBtn = screen.getByText("Войти через Microsoft");
    fireEvent.click(msBtn);

    const banner = await screen.findByTestId("ms-login-error-banner");
    expect(banner.textContent).toContain("Network timeout");
    expect(screen.queryByTestId("create-offline-fallback-btn")).toBeNull();
  });

  it("enforces zero hardcoded version strings in text (anti-F4)", () => {
    const { container } = render(() => <AccountManager />);
    expect(container.textContent).not.toMatch(/v0\.[0-9]+/);
  });
});