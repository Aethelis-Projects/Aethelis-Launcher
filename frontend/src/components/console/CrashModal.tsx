import { Component, createSignal, Show, For } from "solid-js";
import { AlertOctagon, Copy, Check, X, Terminal } from "lucide-solid";
import type { CrashReportDTO } from "../../bindings/ipc_types";

interface CrashModalProps {
  report: CrashReportDTO | null;
  onClose: () => void;
}

export const CrashModal: Component<CrashModalProps> = (props) => {
  const [copied, setCopied] = createSignal(false);

  const handleCopy = () => {
    if (!props.report) return;
    const text = `Nord Launcher Crash Report
Category: ${props.report.category}
Exit Code: ${props.report.exit_code}
Summary: ${props.report.summary}
Remedy: ${props.report.remedy}

Logs:
${props.report.relevant_lines.join("\n")}`;

    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Show when={props.report}>
      <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4">
        <div class="w-full max-w-2xl bg-zinc-950 border border-red-500/40 rounded-lg shadow-2xl overflow-hidden flex flex-col max-h-[85vh]">
          {/* Header */}
          <div class="bg-red-950/40 border-b border-red-500/30 px-5 py-3.5 flex items-center justify-between">
            <div class="flex items-center gap-2.5">
              <AlertOctagon class="w-5 h-5 text-red-400" />
              <h2 class="text-sm font-semibold tracking-wider uppercase text-red-200">
                Сбой выполнения игры (Код: {props.report!.exit_code})
              </h2>
            </div>
            <button
              type="button"
              onClick={props.onClose}
              class="text-zinc-400 hover:text-zinc-100 p-1 rounded hover:bg-zinc-800 transition-colors"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          {/* Body */}
          <div class="p-5 overflow-y-auto space-y-4 text-xs">
            {/* Category & Summary */}
            <div class="bg-zinc-900 border border-zinc-800 rounded p-3.5 space-y-2">
              <div class="flex items-center gap-2">
                <span class="font-mono text-[10px] uppercase tracking-wider px-2 py-0.5 rounded bg-red-500/10 border border-red-500/30 text-red-400">
                  {props.report!.category}
                </span>
                <span class="font-medium text-zinc-200">
                  {props.report!.summary}
                </span>
              </div>
              <p class="text-zinc-400 leading-relaxed pl-1 border-l-2 border-[#00D4B2]">
                <strong class="text-zinc-200">Рекомендация:</strong> {props.report!.remedy}
              </p>
            </div>

            {/* Log Snippet */}
            <div class="space-y-1.5">
              <span class="text-[11px] font-mono text-zinc-400 flex items-center gap-1.5">
                <Terminal class="w-3.5 h-3.5 text-zinc-500" />
                Журнал последних сообщений (Log4j):
              </span>
              <div class="bg-zinc-950 border border-zinc-900 rounded p-3 font-mono text-[11px] text-zinc-400 overflow-x-auto max-h-48 space-y-1">
                <For each={props.report!.relevant_lines}>
                  {(line) => (
                    <div class={`break-all ${line.includes("ERROR") || line.includes("FATAL") ? "text-red-400" : ""}`}>
                      {line}
                    </div>
                  )}
                </For>
              </div>
            </div>
          </div>

          {/* Footer */}
          <div class="bg-zinc-900/60 border-t border-zinc-800 px-5 py-3 flex items-center justify-between">
            <button
              type="button"
              onClick={handleCopy}
              class="px-3 py-1.5 rounded bg-zinc-800 hover:bg-zinc-700 text-zinc-200 font-mono text-xs flex items-center gap-1.5 transition-colors"
            >
              <Show when={copied()} fallback={<Copy class="w-3.5 h-3.5" />}>
                <Check class="w-3.5 h-3.5 text-[#00D4B2]" />
              </Show>
              {copied() ? "Скопировано в буфер" : "Скопировать диагностику"}
            </button>

            <button
              type="button"
              onClick={props.onClose}
              class="px-4 py-1.5 rounded bg-zinc-800 hover:bg-zinc-700 text-zinc-100 font-medium text-xs transition-colors"
            >
              Закрыть
            </button>
          </div>
        </div>
      </div>
    </Show>
  );
};