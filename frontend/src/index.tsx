/* @refresh reload */
import { render } from "solid-js/web";
import "./index.css";
import { App } from "./App";

const root = document.getElementById("root");

if (import.meta.env.DEV && !(root instanceof HTMLElement)) {
  throw new Error(
    "Root element not found. Did you forget to add it to your index.html? Or maybe the id attribute got misspelled?"
  );
}

render(() => <App />, root!);

// Instrument first UI frame paint for NFR measurement
requestAnimationFrame(() => {
  performance.mark("first-ui-frame");
  const entries = performance.getEntriesByName("first-ui-frame");
  if (entries.length > 0) {
    console.log(`[PERF] First UI Frame rendered at: ${entries[0].startTime.toFixed(2)}ms`);
  }
});