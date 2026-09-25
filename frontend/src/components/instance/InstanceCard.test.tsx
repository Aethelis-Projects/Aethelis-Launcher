import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi } from "vitest";
import { InstanceCard } from "./InstanceCard";
import { launcherAPI } from "../../services/api";
import { InstanceDTO } from "../../bindings/ipc_types";

describe("InstanceCard", () => {
  const mockInstance: InstanceDTO = {
    id: "test-inst-1",
    name: "Nord Test",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
    min_ram_mb: 2048,
    max_ram_mb: 4096,
    jvm_args: [],
    skip_java_check: false,
    state: "idle",
    total_play_seconds: 3600,
  };

  it("renders instance metadata correctly", () => {
    render(() => <InstanceCard instance={mockInstance} />);
    expect(screen.getByText("Nord Test")).toBeTruthy();
    expect(screen.getByText("1.21.1")).toBeTruthy();
    expect(screen.getByText("fabric")).toBeTruthy();
    expect(screen.getByText("1 ч 0 м")).toBeTruthy();
  });

  it("handles selection and keyboard navigation", () => {
    const onSelect = vi.fn();
    render(() => <InstanceCard instance={mockInstance} onSelect={onSelect} />);

    const card = screen.getByTestId("instance-card");
    fireEvent.click(card);
    expect(onSelect).toHaveBeenCalledWith(mockInstance);

    fireEvent.keyDown(card, { key: "Enter" });
    expect(onSelect).toHaveBeenCalledTimes(2);
  });

  it("calls openPath when folder button is clicked without selecting card", () => {
    const onSelect = vi.fn();
    const openPathSpy = vi.spyOn(launcherAPI, "openPath").mockResolvedValue();

    render(() => <InstanceCard instance={mockInstance} onSelect={onSelect} />);

    const folderBtn = screen.getByTestId("instance-open-folder-btn");
    fireEvent.click(folderBtn);

    expect(openPathSpy).toHaveBeenCalledWith("test-inst-1");
    expect(onSelect).not.toHaveBeenCalled();
  });
});