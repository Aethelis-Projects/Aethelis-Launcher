import { render, screen, fireEvent } from "@solidjs/testing-library";
import { describe, it, expect } from "vitest";
import { ModCatalog } from "./ModCatalog";

describe("ModCatalog Component", () => {
  it("renders catalog header and source toggle buttons", async () => {
    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    expect(screen.getByText("Каталог модификаций")).toBeTruthy();
    expect(screen.getByText("Modrinth")).toBeTruthy();
    expect(screen.getByText("CurseForge")).toBeTruthy();
  });

  it("switches source tab on click", async () => {
    render(() => (
      <ModCatalog
        activeInstanceId="test-inst"
        gameVersion="1.21.1"
        loader="fabric"
      />
    ));

    const cfBtn = screen.getByText("CurseForge");
    fireEvent.click(cfBtn);
    expect(cfBtn.className).toContain("bg-[#00D4B2]");
  });
});