import { openExternal } from "./api";

type GuardDeps = {
  open: (url: string) => Promise<void> | void;
};

const DEFAULT_DEPS: GuardDeps = { open: openExternal };

function isExternalHttpHref(anchor: HTMLAnchorElement): boolean {
  const raw = anchor.getAttribute("href");
  if (!raw) return false;
  try {
    const u = new URL(raw, window.location.href);
    return (u.protocol === "https:" || u.protocol === "http:") && u.origin !== window.location.origin;
  } catch {
    return false;
  }
}

function httpUrlFromDataTransfer(dt: DataTransfer | null): string | null {
  if (!dt) return null;
  const candidates: string[] = [];
  const uriList = dt.getData("text/uri-list");
  if (uriList) candidates.push(...uriList.split(/\r?\n/).filter((l) => l && !l.startsWith("#")));
  const plain = dt.getData("text/plain");
  if (plain) candidates.push(plain.trim());
  const url = dt.getData("text/url");
  if (url) candidates.push(url.trim());
  for (const candidate of candidates) {
    try {
      const u = new URL(candidate, window.location.href);
      if ((u.protocol === "https:" || u.protocol === "http:") && u.origin !== window.location.origin) {
        return u.href;
      }
    } catch {
      /* not a URL - ignore */
    }
  }
  return null;
}

/**
 * Installs the document-level link/drop guard and returns a detach function.
 * - any click inside an external <a> (including modifier-click / middle-click
 *   where the browser would open an embedded popup or navigate) is neutralised
 *   and re-routed to the system browser;
 * - drops onto the window are neutralised: a webview navigates itself to a
 *   dropped file or link by default, and a click handler cannot catch that
 *   (owner p3). http(s) payloads are routed to the system browser, anything
 *   else (files, text) is simply swallowed - the launcher has no drop targets
 *   today, and if one appears it must call preventDefault() itself, which this
 *   guard honours (it only acts on unhandled events);
 * - window.open is neutered so page scripts cannot create embedded webviews.
 */
export function installExternalLinkGuard(deps: GuardDeps = DEFAULT_DEPS): () => void {
  const onClick = (event: MouseEvent) => {
    const target = event.target as Element | null;
    const anchor = target?.closest?.("a[href]") as HTMLAnchorElement | null;
    if (!anchor || !isExternalHttpHref(anchor)) return;
    // Blocks navigation for left click and suppresses the popup for
    // ctrl/shift/alt/meta + middle button (those are the WebView2/WebKitGTK
    // "new window" paths the owner asked to eliminate).
    event.preventDefault();
    event.stopPropagation();
    void deps.open(anchor.href);
  };
  const onAuxClick = (event: MouseEvent) => {
    if (event.button === 1) onClick(event);
  };

  // Bubble phase on purpose: an app drop zone that handles the event calls
  // preventDefault itself, and `defaultPrevented` lets us stand down instead of
  // stealing its payload. Dragging INTO a webview always ends in navigation
  // unless the drop is prevented, so the fallback has to be here.
  const onDragOver = (event: DragEvent) => {
    if (event.defaultPrevented) return;
    event.preventDefault();
  };
  const onDrop = (event: DragEvent) => {
    if (event.defaultPrevented) return;
    event.preventDefault();
    const url = httpUrlFromDataTransfer(event.dataTransfer);
    if (url) void deps.open(url);
  };

  const nativeOpen = window.open.bind(window);
  window.open = ((...args: Parameters<typeof window.open>) => {
    const url = args[0];
    if (typeof url === "string" && /^https?:/i.test(url)) {
      void deps.open(url);
    }
    return null;
  }) as typeof window.open;

  document.addEventListener("click", onClick, true);
  document.addEventListener("auxclick", onAuxClick, true);
  window.addEventListener("dragover", onDragOver);
  window.addEventListener("drop", onDrop);
  return () => {
    document.removeEventListener("click", onClick, true);
    document.removeEventListener("auxclick", onAuxClick, true);
    window.removeEventListener("dragover", onDragOver);
    window.removeEventListener("drop", onDrop);
    window.open = nativeOpen;
  };
}
