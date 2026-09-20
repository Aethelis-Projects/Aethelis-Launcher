import { Component, Show } from "solid-js";
import { Download, X, Loader2, Sparkles } from "lucide-solid";
import type { UpdateInfoDTO } from "../../bindings/ipc_types";
import { renderMarkdownLite, formatVersion } from "./UpdatePanel";

export interface StartupUpdateModalProps {
  isOpen: boolean;
  updateInfo: UpdateInfoDTO | null;
  isApplying: boolean;
  onInstallAndRestart: () => void;
  onSnooze: () => void;
  onClose: () => void;
}

export const StartupUpdateModal: Component<StartupUpdateModalProps> = (props) => {
  const formatBytes = (bytes: number): string => {
    if (!bytes || bytes <= 0) return "0 MB";
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  return (
    <Show when={props.isOpen && props.updateInfo && props.updateInfo.has_update}>
      <div
        class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4 animate-in fade-in duration-200"
        data-testid="startup-update-modal"
      >
        <div class="w-full max-w-lg bg-nord-surface border border-nord-cyan/30 rounded-2xl shadow-2xl p-6 flex flex-col gap-5 relative overflow-hidden select-none">
          {/* Header */}
          <div class="flex items-start justify-between gap-4">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-xl bg-nord-cyan/10 border border-nord-cyan/30 flex items-center justify-center text-nord-cyan shrink-0">
                <Sparkles class="w-5 h-5" />
              </div>
              <div>
                <h3 class="text-base font-bold text-white tracking-tight flex items-center gap-2">
                  Доступно обновление Nord Launcher {formatVersion(props.updateInfo?.version)}
                </h3>
                <p class="text-xs text-zinc-400 mt-0.5">
                  Рекомендуется установить последнюю стабильную версию
                </p>
              </div>
            </div>

            <button
              type="button"
              onClick={props.onClose}
              disabled={props.isApplying}
              class="w-8 h-8 rounded-lg flex items-center justify-center text-zinc-400 hover:text-white hover:bg-white/5 transition-colors cursor-pointer disabled:opacity-50"
              title="Закрыть"
              data-testid="startup-modal-close-btn"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          {/* Size & Info */}
          <div class="flex items-center justify-between px-3.5 py-2 rounded-xl bg-zinc-950/60 border border-white/5 text-xs font-mono">
            <span class="text-zinc-400">Размер загрузки:</span>
            <span class="font-bold text-zinc-200">{formatBytes(props.updateInfo?.size || 0)}</span>
          </div>

          {/* Changelog snippet */}
          <div class="flex flex-col gap-2">
            <span class="text-xs font-mono font-medium uppercase tracking-wider text-zinc-400">
              Список изменений
            </span>
            <div
              class="p-3.5 rounded-xl bg-zinc-950/70 border border-white/5 text-xs font-mono leading-relaxed max-h-48 overflow-y-auto"
              data-testid="startup-modal-changelog"
            >
              {renderMarkdownLite(props.updateInfo?.release_notes)}
            </div>
          </div>

          {/* Action buttons */}
          <div class="flex items-center justify-end gap-3 pt-2 border-t border-white/5">
            <button
              type="button"
              onClick={props.onSnooze}
              disabled={props.isApplying}
              class="px-4 py-2 rounded-lg text-xs font-mono text-zinc-400 hover:text-zinc-200 hover:bg-white/5 transition-colors cursor-pointer disabled:opacity-50"
              data-testid="startup-modal-snooze-btn"
            >
              Позже
            </button>
            <button
              type="button"
              onClick={props.onInstallAndRestart}
              disabled={props.isApplying}
              class="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark font-semibold text-xs font-mono shadow-[0_0_12px_rgba(0,212,178,0.3)] transition-all cursor-pointer disabled:opacity-70"
              data-testid="startup-modal-install-btn"
            >
              <Show
                when={props.isApplying}
                fallback={
                  <>
                    <Download class="w-4 h-4" />
                    <span>Установить и перезапустить</span>
                  </>
                }
              >
                <Loader2 class="w-4 h-4 animate-spin" />
                <span>Загрузка и установка...</span>
              </Show>
            </button>
          </div>
        </div>
      </div>
    </Show>
  );
};
