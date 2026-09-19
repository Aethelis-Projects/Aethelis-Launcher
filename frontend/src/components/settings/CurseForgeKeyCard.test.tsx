import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { CurseForgeKeyCard } from "./CurseForgeKeyCard";
import { launcherAPI } from "../../services/api";

describe("CurseForgeKeyCard Component (B5, N6)", () => {
  beforeEach(() => {
    launcherAPI.setMockSettings({});
    vi.restoreAllMocks();
  });

  it("renders with loaded key from settings", async () => {
    launcherAPI.setMockSettings({ curseforge_api_key: "$2a$10$existingkey" });

    render(() => <CurseForgeKeyCard />);

    const input = (await screen.findByTestId("cf-key-input")) as HTMLInputElement;
    await vi.waitFor(() => {
      expect(input.value).toBe("$2a$10$existingkey");
    });
    expect(input.type).toBe("password");
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
});
