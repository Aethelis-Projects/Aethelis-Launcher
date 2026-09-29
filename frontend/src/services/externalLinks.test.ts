import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { installExternalLinkGuard } from "./externalLinks";

// v0.7.2 round-5 (owner p2): a clicked link in user-authored Modrinth/CurseForge
// content must not navigate the app webview and must not create an embedded
// webview window - it is routed to the system browser (or dropped).
describe("installExternalLinkGuard", () => {
  let detach: () => void;
  let open: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    open = vi.fn(async () => undefined);
    detach = installExternalLinkGuard({ open });
  });
  afterEach(() => detach());

  const anchor = (href: string) => {
    const a = document.createElement("a");
    a.setAttribute("href", href);
    const span = document.createElement("span");
    a.appendChild(span);
    document.body.appendChild(a);
    return { a, span };
  };

  it("routes an external http link to the browser and cancels navigation", () => {
    const { span } = anchor("https://modrinth.com/mod/sodium");
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
    span.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    expect(open).toHaveBeenCalledWith("https://modrinth.com/mod/sodium");
  });

  it("covers modifier-click (embedded-popup path)", () => {
    const { a } = anchor("https://example.com/x");
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true, ctrlKey: true });
    a.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    expect(open).toHaveBeenCalledTimes(1);
  });

  it("covers middle-click auxclick", () => {
    const { a } = anchor("https://example.com/y");
    const ev = new MouseEvent("auxclick", { bubbles: true, cancelable: true, button: 1 });
    a.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    expect(open).toHaveBeenCalledTimes(1);
  });

  it("leaves app-internal links alone", () => {
    const { a } = anchor("/instances");
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
    a.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(false);
    expect(open).not.toHaveBeenCalled();
  });

  it("drops javascript: and data: hrefs without opening anything", () => {
    for (const href of ["javascript:alert(1)", "data:text/html,<script>1</script>", "file:///etc/passwd"]) {
      const { a } = anchor(href);
      const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
      a.dispatchEvent(ev);
      expect(open).not.toHaveBeenCalled();
    }
  });

  it("neuters window.open so page scripts cannot spawn an embedded webview", async () => {
    const w = window.open("https://evil.test/popup");
    expect(w).toBeNull();
    expect(open).toHaveBeenCalledWith("https://evil.test/popup");
  });

  it("detach restores native window.open", () => {
    detach();
    const native = window.open;
    expect(native).not.toBe(open);
    detach = installExternalLinkGuard({ open });
  });
});

// v0.7.2 round-6 (owner p3): dropping a file or a link onto the window makes a
// webview navigate away from the app, and a click handler cannot see that at
// all. The guard therefore owns dragover/drop too - while standing down when an
// in-app drop zone handled the event first.
describe("installExternalLinkGuard - drag & drop onto the window", () => {
  type DropFixture = Event & { dataTransfer: { getData: (kind: string) => string } };

  const makeDrop = (payloads: Record<string, string>): DropFixture => {
    const ev = new Event("drop", { bubbles: true, cancelable: true }) as DropFixture;
    ev.dataTransfer = { getData: (kind: string) => payloads[kind] ?? "" };
    return ev;
  };

  it("routes a dropped http link to the system browser and cancels navigation", () => {
    const open = vi.fn(async () => undefined);
    const detach = installExternalLinkGuard({ open });
    const ev = makeDrop({ "text/uri-list": "https://modrinth.com/mod/sodium" });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    expect(open).toHaveBeenCalledWith("https://modrinth.com/mod/sodium");
    detach();
  });

  it("swallows a dropped file instead of navigating the app away", () => {
    const open = vi.fn(async () => undefined);
    const detach = installExternalLinkGuard({ open });
    const ev = makeDrop({ "text/plain": "" });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    expect(open).not.toHaveBeenCalled();
    detach();
  });

  it("prevents the dragover default (WebKitGTK/WebView2 start navigation there)", () => {
    const detach = installExternalLinkGuard({ open: vi.fn() });
    const ev = new Event("dragover", { bubbles: true, cancelable: true });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    detach();
  });

  it("stands down when an in-app drop zone already handled the event", () => {
    const open = vi.fn(async () => undefined);
    const detach = installExternalLinkGuard({ open });
    const ev = makeDrop({ "text/uri-list": "https://modrinth.com/mod/lithium" });
    ev.preventDefault(); // a future drop zone marks the event handled
    window.dispatchEvent(ev);
    expect(open).not.toHaveBeenCalled();
    detach();
  });

  it("does not open non-http dropped payloads", () => {
    const open = vi.fn(async () => undefined);
    const detach = installExternalLinkGuard({ open });
    const hostile: Record<string, string>[] = [
      { "text/uri-list": "javascript:alert(1)" },
      { "text/uri-list": "file:///home/user/.ssh/id_ed25519" },
      { "text/plain": "data:text/html,<script>alert(1)</script>" },
    ];
    for (const payloads of hostile) {
      const ev = makeDrop(payloads);
      window.dispatchEvent(ev);
      expect(ev.defaultPrevented).toBe(true);
    }
    expect(open).not.toHaveBeenCalled();
    detach();
  });

  it("detach removes the drop guard again", () => {
    const open = vi.fn(async () => undefined);
    const detach = installExternalLinkGuard({ open });
    detach();
    const ev = makeDrop({ "text/uri-list": "https://example.com/after-detach" });
    window.dispatchEvent(ev);
    expect(open).not.toHaveBeenCalled();
  });
});
