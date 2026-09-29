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

/**
 * Installs the document-level link guard and returns a detach function.
 * - any click inside an external <a> (including modifier-click / middle-click
 *   where the browser would open an embedded popup or navigate) is neutralised
 *   and re-routed to the system browser;
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
  return () => {
    document.removeEventListener("click", onClick, true);
    document.removeEventListener("auxclick", onAuxClick, true);
    window.open = nativeOpen;
  };
}
