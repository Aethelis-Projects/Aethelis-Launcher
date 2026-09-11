import { render, fireEvent, screen } from "@solidjs/testing-library";
import { describe, it, expect, vi } from "vitest";
import { ModSearchInput } from "./ModSearchInput";

describe("ModSearchInput", () => {
  it("renders with placeholder and triggers search on typing", () => {
    const onSearch = vi.fn();
    render(() => <ModSearchInput placeholder="Поиск..." onSearch={onSearch} />);
    const input = screen.getByTestId("mod-search-input") as HTMLInputElement;

    expect(input.placeholder).toBe("Поиск...");
    fireEvent.input(input, { target: { value: "sodium" } });

    expect(onSearch).toHaveBeenCalledWith("sodium");
  });

  it("clears value when clear button is clicked", () => {
    const onClear = vi.fn();
    const onSearch = vi.fn();
    render(() => <ModSearchInput value="iris" onClear={onClear} onSearch={onSearch} />);

    const clearBtn = screen.getByTestId("mod-search-clear");
    fireEvent.click(clearBtn);

    expect(onClear).toHaveBeenCalledTimes(1);
    expect(onSearch).toHaveBeenCalledWith("");
  });
});