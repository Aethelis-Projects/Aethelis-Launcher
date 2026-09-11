import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi } from "vitest";
import { LaunchButton } from "./LaunchButton";

describe("LaunchButton", () => {
  it("renders default state with active verb", () => {
    render(() => <LaunchButton />);
    const btn = screen.getByTestId("launch-button");
    expect(btn.textContent).toContain("Запустить игру");
    expect(btn.getAttribute("disabled")).toBeNull();
  });

  it("handles loading state without layout shift", () => {
    render(() => <LaunchButton state="loading" />);
    const btn = screen.getByTestId("launch-button");
    expect(btn.textContent).toContain("Запуск игры...");
    expect(btn.getAttribute("aria-busy")).toBe("true");
  });

  it("handles success state", () => {
    render(() => <LaunchButton state="success" />);
    const btn = screen.getByTestId("launch-button");
    expect(btn.textContent).toContain("Игра запущена");
  });

  it("handles error state with action and message", () => {
    const onRetry = vi.fn();
    render(() => (
      <LaunchButton
        state="error"
        errorMessage="Java runtime exit code 1"
        onRetry={onRetry}
      />
    ));
    const btn = screen.getByTestId("launch-button");
    expect(btn.textContent).toContain("Повторить запуск");
    expect(screen.getByTestId("launch-error-msg").textContent).toContain("Java runtime exit code 1");

    fireEvent.click(btn);
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("handles disabled state", () => {
    const onClick = vi.fn();
    render(() => <LaunchButton disabled={true} onClick={onClick} />);
    const btn = screen.getByTestId("launch-button");
    expect(btn.hasAttribute("disabled")).toBe(true);

    fireEvent.click(btn);
    expect(onClick).not.toHaveBeenCalled();
  });
});