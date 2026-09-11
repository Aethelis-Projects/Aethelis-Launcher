import { render, screen, fireEvent } from "@solidjs/testing-library";
import { describe, it, expect, vi } from "vitest";
import { CrashModal } from "./CrashModal";
import type { CrashReportDTO } from "../../bindings/ipc_types";

describe("CrashModal Component", () => {
  const mockReport: CrashReportDTO = {
    category: "out_of_memory",
    summary: "Нехватка оперативной памяти Java Heap",
    remedy: "Увеличьте RAM в настройках инстанса",
    details: "java.lang.OutOfMemoryError",
    relevant_lines: [
      "[INFO] Starting game...",
      "[ERROR] java.lang.OutOfMemoryError: Java heap space",
    ],
    exit_code: 1,
  };

  it("renders crash report details when open", () => {
    const onClose = vi.fn();
    render(() => <CrashModal report={mockReport} onClose={onClose} />);

    expect(screen.getByText("Сбой выполнения игры (Код: 1)")).toBeTruthy();
    expect(screen.getByText("out_of_memory")).toBeTruthy();
    expect(screen.getByText("Нехватка оперативной памяти Java Heap")).toBeTruthy();
    expect(screen.getByText(/Увеличьте RAM/)).toBeTruthy();
  });

  it("calls onClose when close button clicked", () => {
    const onClose = vi.fn();
    render(() => <CrashModal report={mockReport} onClose={onClose} />);

    const closeBtn = screen.getByText("Закрыть");
    fireEvent.click(closeBtn);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not render when report is null", () => {
    render(() => <CrashModal report={null} onClose={() => {}} />);
    expect(screen.queryByText("Сбой выполнения игры")).toBeNull();
  });
});