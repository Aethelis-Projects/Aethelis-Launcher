import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi } from "vitest";
import { InstanceCard } from "./InstanceCard";
import { InstanceDTO } from "../../bindings/ipc_types";

describe("InstanceCard", () => {
  const mockInstance: InstanceDTO = {
    id: "test-inst-1",
    name: "Nord Test",
    game_version: "1.21.1",
    loader: "fabric",
    loader_version: "0.16.5",
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
});