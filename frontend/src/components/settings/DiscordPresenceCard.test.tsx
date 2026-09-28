import { render, screen, fireEvent, waitFor } from "@solidjs/testing-library";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { DiscordPresenceCard } from "./DiscordPresenceCard";
import { launcherAPI } from "../../services/api";

function baseMocks() {
  vi.spyOn(launcherAPI, "getSettings").mockResolvedValue({ settings: {} });
  vi.spyOn(launcherAPI, "listInstances").mockResolvedValue([
    {
      id: "inst-1", name: "Vault", game_version: "1.20.1", loader: "forge",
      min_ram_mb: 2048, max_ram_mb: 4096, java_path: "", jvm_args: "", game_args: "",
      version_type: "release", onboarding_done: true, is_favorite: false, group_name: "",
      created_at: "", updated_at: "", last_played_at: "",
    } as never,
  ]);
  vi.spyOn(launcherAPI, "getDiscordRpcPreview").mockResolvedValue({
    details: "Minecraft 1.20.1", state: "Vault",
  });
}

describe("DiscordPresenceCard (D'5)", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("renders opt-in default off and honest preview", async () => {
    baseMocks();
    vi.spyOn(launcherAPI, "getDiscordRpcStatus").mockResolvedValue({
      enabled: false, connected: false, app_id_set: false, has_activity: false,
    });
    render(() => <DiscordPresenceCard />);
    const toggle = (await screen.findByTestId("discord-rpc-toggle")) as HTMLInputElement;
    expect(toggle.checked).toBe(false);
    await waitFor(() =>
      expect(screen.getByTestId("discord-rpc-preview").textContent).toContain("Minecraft 1.20.1")
    );
    expect(screen.getByTestId("discord-rpc-preview").textContent).toContain("Vault");
    expect(screen.getByTestId("discord-rpc-preview").textContent).toContain("Никаких аккаунтных данных");
  });

  it("enabling calls backend and shows missing-app-id guidance", async () => {
    baseMocks();
    const enable = vi.spyOn(launcherAPI, "setDiscordRpcEnabled").mockResolvedValue(undefined);
    vi.spyOn(launcherAPI, "getDiscordRpcStatus")
      .mockResolvedValueOnce({ enabled: false, connected: false, app_id_set: false, has_activity: false })
      .mockResolvedValue({ enabled: true, connected: false, app_id_set: false, has_activity: false });
    render(() => <DiscordPresenceCard />);
    const toggle = (await screen.findByTestId("discord-rpc-toggle")) as HTMLInputElement;
    fireEvent.click(toggle);
    await waitFor(() => expect(enable).toHaveBeenCalledWith(true));
    await screen.findByTestId("discord-rpc-offline");
    expect(screen.getByTestId("discord-rpc-offline").textContent).toContain("Нужен Application ID");
  });

  it("saves application id through the dedicated method", async () => {
    baseMocks();
    vi.spyOn(launcherAPI, "getDiscordRpcStatus").mockResolvedValue({
      enabled: true, connected: false, app_id_set: false, has_activity: false,
    });
    const save = vi.spyOn(launcherAPI, "setDiscordAppID").mockResolvedValue(undefined);
    render(() => <DiscordPresenceCard />);
    const input = (await screen.findByTestId("discord-appid-input")) as HTMLInputElement;
    fireEvent.input(input, { target: { value: "1234567890" } });
    fireEvent.click(screen.getByTestId("discord-appid-save"));
    await waitFor(() => expect(save).toHaveBeenCalledWith("1234567890"));
  });
});
