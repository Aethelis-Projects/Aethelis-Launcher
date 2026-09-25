import { render, screen } from "@solidjs/testing-library";
import { describe, it, expect } from "vitest";
import { MarkdownLite, renderMarkdownLite } from "./MarkdownLite";

describe("MarkdownLite Component (G2)", () => {
  it("renders default fallback text when content is empty", () => {
    render(() => <MarkdownLite content="" />);
    expect(
      screen.getByText("Regular maintenance release with security and performance improvements.")
    ).toBeTruthy();
  });

  it("renders custom fallback text when provided and content is empty", () => {
    render(() => <MarkdownLite content="" fallback="No release notes provided." />);
    expect(screen.getByText("No release notes provided.")).toBeTruthy();
  });

  it("renders h2, h3, bullet items, and regular text without innerHTML", () => {
    const markdown = [
      "## Major Update",
      "### New Features",
      "- Add support for .mrpack files",
      "* Fix crash on startup",
      "Please report any bugs to the repository.",
    ].join("\n");

    render(() => <MarkdownLite content={markdown} />);

    expect(screen.getByText("Major Update")).toBeTruthy();
    expect(screen.getByText("New Features")).toBeTruthy();
    expect(screen.getByText("Add support for .mrpack files")).toBeTruthy();
    expect(screen.getByText("Fix crash on startup")).toBeTruthy();
    expect(screen.getByText("Please report any bugs to the repository.")).toBeTruthy();
  });

  it("renderMarkdownLite function returns JSX element directly", () => {
    render(() => <div>{renderMarkdownLite("- Test item")}</div>);
    expect(screen.getByText("Test item")).toBeTruthy();
  });
});
