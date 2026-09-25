import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi } from "vitest";
import { ModUpdatesDiffModal } from "./ModUpdatesDiffModal";
import type { InstalledModDTO, ModUpdateItemDTO } from "../../bindings/ipc_types";

describe("ModUpdatesDiffModal Component (G1, G2, G3)", () => {
  const mockInstalledMods: InstalledModDTO[] = [
    {
      file_name: "sodium-fabric-0.5.8.jar",
      mod_id: "sodium",
      name: "Sodium",
      version: "0.5.8",
      source: "modrinth",
      release_type: "release",
      enabled: true,
      size_bytes: 1048576,
    },
    {
      file_name: "appleskin-forge-mc1.21-3.0.4.jar",
      mod_id: "248787",
      name: "AppleSkin",
      version: "3.0.4",
      source: "curseforge",
      release_type: "beta",
      enabled: true,
      size_bytes: 524288,
    },
  ];

  const mockUpdates: ModUpdateItemDTO[] = [
    {
      file_name: "sodium-fabric-0.5.8.jar",
      mod_id: "sodium",
      source: "modrinth",
      current_version: "0.5.8",
      latest_version: "0.5.9",
      latest_version_id: "sodium-0.5.9-id",
      release_type: "release",
      dependencies: ["fabric-api", "cloth-config"],
      changelog: "### Changes\n- Improved chunk rendering\n- Fixed crash on launch",
    },
    {
      file_name: "appleskin-forge-mc1.21-3.0.4.jar",
      mod_id: "248787",
      source: "curseforge",
      current_version: "3.0.4",
      latest_version: "3.0.5",
      latest_version_id: "cf-file-305",
      release_type: "release",
      dependencies: [],
      changelog: "Performance fixes for tooltips",
    },
  ];

  it("does not render modal when isOpen is false", () => {
    render(() => (
      <ModUpdatesDiffModal
        isOpen={false}
        onClose={vi.fn()}
        installedMods={mockInstalledMods}
        updates={mockUpdates}
      />
    ));

    expect(screen.queryByTestId("mod-updates-diff-modal")).toBeNull();
  });

  it("renders paired installed-to-candidate diff with versions and source badges (G1, G3)", () => {
    render(() => (
      <ModUpdatesDiffModal
        isOpen={true}
        onClose={vi.fn()}
        installedMods={mockInstalledMods}
        updates={mockUpdates}
      />
    ));

    expect(screen.getByTestId("mod-updates-diff-modal")).toBeTruthy();
    expect(screen.getByText("Доступные обновления модов")).toBeTruthy();
    expect(screen.getByText("Обнаружено обновлений: 2")).toBeTruthy();

    // Sodium pairing (Modrinth)
    const sodiumCard = screen.getByTestId("update-diff-item-sodium-fabric-0.5.8.jar");
    expect(sodiumCard.textContent).toContain("Sodium");
    expect(sodiumCard.textContent).toContain("modrinth");
    expect(sodiumCard.textContent).toContain("v0.5.8");
    expect(sodiumCard.textContent).toContain("v0.5.9");

    // AppleSkin pairing (CurseForge)
    const appleCard = screen.getByTestId("update-diff-item-appleskin-forge-mc1.21-3.0.4.jar");
    expect(appleCard.textContent).toContain("AppleSkin");
    expect(appleCard.textContent).toContain("curseforge");
    expect(appleCard.textContent).toContain("v3.0.4");
    expect(appleCard.textContent).toContain("v3.0.5");
  });

  it("renders dependency delta when dependencies exist on candidate update (G1)", () => {
    render(() => (
      <ModUpdatesDiffModal
        isOpen={true}
        onClose={vi.fn()}
        installedMods={mockInstalledMods}
        updates={mockUpdates}
      />
    ));

    const sodiumCard = screen.getByTestId("update-diff-item-sodium-fabric-0.5.8.jar");
    expect(sodiumCard.textContent).toContain("Дельта зависимостей (2)");
    expect(sodiumCard.textContent).toContain("fabric-api");
    expect(sodiumCard.textContent).toContain("cloth-config");

    const appleCard = screen.getByTestId("update-diff-item-appleskin-forge-mc1.21-3.0.4.jar");
    expect(appleCard.textContent).toContain("без внешних зависимостей");
  });

  it("renders safe markdown-lite changelog for candidate update (G2)", () => {
    render(() => (
      <ModUpdatesDiffModal
        isOpen={true}
        onClose={vi.fn()}
        installedMods={mockInstalledMods}
        updates={mockUpdates}
      />
    ));

    const sodiumCard = screen.getByTestId("update-diff-item-sodium-fabric-0.5.8.jar");
    expect(sodiumCard.textContent).toContain("Changes");
    expect(sodiumCard.textContent).toContain("Improved chunk rendering");
    expect(sodiumCard.textContent).toContain("Fixed crash on launch");
  });

  it("triggers onUpdateMod when individual update button is clicked", () => {
    const onUpdateMod = vi.fn();
    render(() => (
      <ModUpdatesDiffModal
        isOpen={true}
        onClose={vi.fn()}
        installedMods={mockInstalledMods}
        updates={mockUpdates}
        onUpdateMod={onUpdateMod}
      />
    ));

    const updateBtn = screen.getByTestId("apply-update-btn-sodium-fabric-0.5.8.jar");
    fireEvent.click(updateBtn);

    expect(onUpdateMod).toHaveBeenCalledTimes(1);
    expect(onUpdateMod).toHaveBeenCalledWith(mockUpdates[0]);
  });

  it("triggers onUpdateAll when 'Обновить все' button is clicked", () => {
    const onUpdateAll = vi.fn();
    render(() => (
      <ModUpdatesDiffModal
        isOpen={true}
        onClose={vi.fn()}
        installedMods={mockInstalledMods}
        updates={mockUpdates}
        onUpdateAll={onUpdateAll}
      />
    ));

    const updateAllBtn = screen.getByTestId("diff-update-all-btn");
    expect(updateAllBtn.textContent).toContain("Обновить все (2)");
    fireEvent.click(updateAllBtn);

    expect(onUpdateAll).toHaveBeenCalledTimes(1);
  });

  it("triggers onClose when close button is clicked", () => {
    const onClose = vi.fn();
    render(() => (
      <ModUpdatesDiffModal
        isOpen={true}
        onClose={onClose}
        installedMods={mockInstalledMods}
        updates={mockUpdates}
      />
    ));

    const closeBtn = screen.getByTestId("close-mod-updates-diff-modal-btn");
    fireEvent.click(closeBtn);
    expect(onClose).toHaveBeenCalledTimes(1);

    const footerCloseBtn = screen.getByTestId("close-diff-modal-footer-btn");
    fireEvent.click(footerCloseBtn);
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
