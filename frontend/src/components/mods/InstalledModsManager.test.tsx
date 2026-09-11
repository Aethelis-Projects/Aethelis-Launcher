import { render, screen } from "@solidjs/testing-library";
import { describe, it, expect } from "vitest";
import { InstalledModsManager } from "./InstalledModsManager";

describe("InstalledModsManager Component", () => {
  it("renders installed mods manager and search input", () => {
    render(() => <InstalledModsManager instanceId="nord-opti-1" />);
    expect(screen.getByText(/Установленные модификации/)).toBeTruthy();
    expect(screen.getByPlaceholderText("Фильтр модов...")).toBeTruthy();
  });
});