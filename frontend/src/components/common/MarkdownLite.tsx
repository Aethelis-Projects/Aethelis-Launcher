import { For, type JSX } from "solid-js";

export interface MarkdownLiteProps {
  content?: string;
  fallback?: string;
  class?: string;
}

export function renderMarkdownLite(
  text?: string,
  fallbackText: string = "Regular maintenance release with security and performance improvements."
): JSX.Element {
  if (!text || text.trim() === "") {
    return <p class="text-zinc-400">{fallbackText}</p>;
  }
  const lines = text.split("\n");
  return (
    <div class="space-y-1">
      <For each={lines}>
        {(line) => {
          const trimmed = line.trim();
          if (trimmed.startsWith("### ")) {
            return (
              <div class="font-bold text-zinc-200 text-[11px] uppercase tracking-wider mt-2.5 mb-1 text-[#00D4B2]/90">
                {trimmed.substring(4)}
              </div>
            );
          }
          if (trimmed.startsWith("## ")) {
            return (
              <div class="font-bold text-white text-xs mt-3 mb-1">
                {trimmed.substring(3)}
              </div>
            );
          }
          if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
            return (
              <div class="flex items-start gap-2 pl-1.5 py-0.5 text-zinc-300">
                <span class="text-[#00D4B2] select-none leading-none mt-1 text-[10px]">•</span>
                <span class="leading-snug">{trimmed.substring(2)}</span>
              </div>
            );
          }
          if (trimmed === "") {
            return <div class="h-1" />;
          }
          return <div class="leading-relaxed text-zinc-300">{line}</div>;
        }}
      </For>
    </div>
  );
}

export const MarkdownLite = (props: MarkdownLiteProps) => {
  return (
    <div class={props.class}>
      {renderMarkdownLite(props.content, props.fallback)}
    </div>
  );
};
