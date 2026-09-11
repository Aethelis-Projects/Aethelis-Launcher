import { render, screen, fireEvent } from "@solidjs/testing-library";
import { describe, it, expect } from "vitest";
import { AccountManager } from "./AccountManager";

describe("AccountManager Component", () => {
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
});