import { Component, createSignal, For, Show } from "solid-js";
import { X, ArrowRight, Download, Loader2, Package, Layers, Sparkles } from "lucide-solid";
import type { InstalledModDTO, ModUpdateItemDTO } from "../../bindings/ipc_types";
import { renderMarkdownLite } from "../common/MarkdownLite";

export interface ModUpdatesDiffModalProps {
  isOpen: boolean;
  onClose: () => void;
  installedMods: InstalledModDTO[];
  updates: ModUpdateItemDTO[];
  onUpdateMod?: (update: ModUpdateItemDTO) => Promise<void> | void;
  onUpdateAll?: () => Promise<void> | void;
  updatingFiles?: Set<string>;
  updatingAll?: boolean;
}

export const ModUpdatesDiffModal: Component<ModUpdatesDiffModalProps> = (props) => {
  const [filterQuery, setFilterQuery] = createSignal("");

  const filteredUpdates = () => {
    const q = filterQuery().trim().toLowerCase();
    if (!q) return props.updates;
    return props.updates.filter((u) => {
      const installed = props.installedMods.find(
        (m) => m.file_name === u.file_name || (Boolean(u.mod_id) && m.mod_id === u.mod_id)
      );
      const name = installed?.name || u.file_name;
      return (
        name.toLowerCase().includes(q) ||
        u.file_name.toLowerCase().includes(q) ||
        u.source.toLowerCase().includes(q)
      );
    });
  };

  const getReleaseTypeClass = (type?: string) => {
    switch (type?.toLowerCase()) {
      case "release":
        return "bg-emerald-500/10 border-emerald-500/30 text-emerald-400";
      case "beta":
        return "bg-blue-500/10 border-blue-500/30 text-blue-400";
      case "alpha":
        return "bg-amber-500/10 border-amber-500/30 text-amber-400";
      default:
        return "bg-zinc-800 border-zinc-700 text-zinc-400";
    }
  };

  return (
    <Show when={props.isOpen}>
      <div
        class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm animate-fade-in"
        data-testid="mod-updates-diff-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="mod-updates-diff-title"
      >
        <div class="relative w-full max-w-2xl max-h-[85vh] flex flex-col bg-zinc-900 border border-zinc-800 rounded-xl shadow-2xl overflow-hidden font-sans">
          {/* Header */}
          <div class="flex items-center justify-between p-4 border-b border-zinc-800 bg-zinc-950/60">
            <div class="flex items-center gap-2.5">
              <div class="w-8 h-8 rounded-lg bg-[#00D4B2]/10 border border-[#00D4B2]/30 flex items-center justify-center text-[#00D4B2]">
                <Sparkles class="w-4 h-4" />
              </div>
              <div>
                <h2 id="mod-updates-diff-title" class="text-sm font-semibold text-zinc-100">
                  Доступные обновления модов
                </h2>
                <p class="text-xs text-zinc-400 font-mono">
                  Обнаружено обновлений: {props.updates.length}
                </p>
              </div>
            </div>
            <button
              type="button"
              onClick={props.onClose}
              data-testid="close-mod-updates-diff-modal-btn"
              class="w-8 h-8 rounded-lg border border-zinc-800 bg-zinc-900 flex items-center justify-center text-zinc-400 hover:text-zinc-100 hover:border-zinc-700 transition-colors"
              title="Закрыть"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          {/* Search / Filter */}
          <Show when={props.updates.length > 3}>
            <div class="px-4 py-2 border-b border-zinc-800/80 bg-zinc-950/30">
              <input
                type="text"
                placeholder="Фильтр по названию или источнику..."
                value={filterQuery()}
                onInput={(e) => setFilterQuery(e.currentTarget.value)}
                data-testid="mod-updates-diff-filter-input"
                class="w-full bg-zinc-950 border border-zinc-800 rounded px-3 py-1.5 text-xs text-zinc-200 placeholder-zinc-500 font-mono outline-none focus:border-[#00D4B2]"
              />
            </div>
          </Show>

          {/* List of paired updates */}
          <div class="flex-1 overflow-y-auto p-4 space-y-3 divide-y divide-zinc-800/40">
            <Show
              when={filteredUpdates().length > 0}
              fallback={
                <div class="text-center py-10 text-zinc-500 text-xs font-mono">
                  Нет обновлений, соответствующих фильтру.
                </div>
              }
            >
              <For each={filteredUpdates()}>
                {(update) => {
                  const installed = () =>
                    props.installedMods.find(
                      (m) =>
                        m.file_name === update.file_name ||
                        (Boolean(update.mod_id) && m.mod_id === update.mod_id)
                    );
                  const isUpdating = () =>
                    Boolean(props.updatingFiles?.has(update.file_name));

                  return (
                    <div
                      class="pt-3 first:pt-0 space-y-2"
                      data-testid={`update-diff-item-${update.file_name}`}
                    >
                      {/* Top row: name, source badge, and action */}
                      <div class="flex items-center justify-between gap-3">
                        <div class="flex items-center gap-2 min-w-0">
                          <Package class="w-4 h-4 text-zinc-400 shrink-0" />
                          <span class="text-xs font-semibold text-zinc-100 truncate">
                            {installed()?.name || update.file_name}
                          </span>
                          <span class="text-[10px] font-mono px-1.5 py-0.5 rounded border bg-zinc-800/80 border-zinc-700 text-zinc-400 capitalize shrink-0">
                            {update.source}
                          </span>
                        </div>

                        <Show when={props.onUpdateMod}>
                          <button
                            type="button"
                            onClick={() => props.onUpdateMod?.(update)}
                            disabled={isUpdating() || props.updatingAll}
                            data-testid={`apply-update-btn-${update.file_name}`}
                            class="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded bg-[#00D4B2] text-zinc-950 hover:bg-[#00D4B2]/90 transition-colors disabled:opacity-50 shrink-0 cursor-pointer"
                          >
                            <Show
                              when={isUpdating()}
                              fallback={<Download class="w-3.5 h-3.5" />}
                            >
                              <Loader2 class="w-3.5 h-3.5 animate-spin" />
                            </Show>
                            <span>{isUpdating() ? "Обновление..." : "Обновить"}</span>
                          </button>
                        </Show>
                      </div>

                      {/* Version Pairing & Diff (G1, G3) */}
                      <div class="flex items-center gap-2 p-2 rounded-lg bg-zinc-950/70 border border-zinc-800/80 text-xs font-mono">
                        <div class="flex items-center gap-1.5">
                          <span class="text-zinc-400 text-[11px]">Текущая:</span>
                          <span class="text-zinc-200">
                            v{installed()?.version || update.current_version || "—"}
                          </span>
                          <Show when={installed()?.release_type}>
                            <span
                              class={`text-[9px] px-1 py-0.2 rounded border uppercase ${getReleaseTypeClass(
                                installed()?.release_type
                              )}`}
                            >
                              {installed()?.release_type}
                            </span>
                          </Show>
                        </div>

                        <ArrowRight class="w-3.5 h-3.5 text-[#00D4B2] shrink-0" />

                        <div class="flex items-center gap-1.5">
                          <span class="text-zinc-400 text-[11px]">Новая:</span>
                          <span class="text-[#00D4B2] font-semibold">
                            v{update.latest_version}
                          </span>
                          <span
                            class={`text-[9px] px-1 py-0.2 rounded border uppercase ${getReleaseTypeClass(
                              update.release_type
                            )}`}
                          >
                            {update.release_type}
                          </span>
                        </div>
                      </div>

                      {/* Dependencies Delta (G1, G3) */}
                      <div class="px-2 py-1 text-[11px] font-mono text-zinc-400 flex items-center gap-2">
                        <Layers class="w-3 h-3 text-zinc-500 shrink-0" />
                        <Show
                          when={update.dependencies && update.dependencies.length > 0}
                          fallback={<span>Зависимости: без изменений / без внешних зависимостей</span>}
                        >
                          <div class="flex items-center gap-1.5 flex-wrap">
                            <span class="text-zinc-300">
                              Дельта зависимостей ({update.dependencies!.length}):
                            </span>
                            <For each={update.dependencies}>
                              {(dep) => (
                                <span class="px-1.5 py-0.2 rounded bg-zinc-800 text-[10px] text-zinc-300 border border-zinc-700">
                                  {dep}
                                </span>
                              )}
                            </For>
                          </div>
                        </Show>
                      </div>

                      {/* Candidate Changelog (G2) */}
                      <Show when={update.changelog}>
                        <div class="mt-1 p-2 rounded-lg bg-zinc-950/40 border border-white/5 text-[11px] font-mono text-zinc-300 max-h-32 overflow-y-auto">
                          <div class="text-[10px] uppercase font-bold text-zinc-500 mb-1">
                            История изменений
                          </div>
                          {renderMarkdownLite(update.changelog)}
                        </div>
                      </Show>
                    </div>
                  );
                }}
              </For>
            </Show>
          </div>

          {/* Footer Actions */}
          <div class="flex items-center justify-between p-4 border-t border-zinc-800 bg-zinc-950/60">
            <button
              type="button"
              onClick={props.onClose}
              data-testid="close-diff-modal-footer-btn"
              class="px-3 py-1.5 rounded-lg border border-zinc-700 bg-zinc-800 text-xs font-medium text-zinc-300 hover:text-white hover:border-zinc-600 transition-colors cursor-pointer"
            >
              Закрыть
            </button>

            <Show when={props.onUpdateAll && props.updates.length > 0}>
              <button
                type="button"
                onClick={props.onUpdateAll}
                disabled={props.updatingAll}
                data-testid="diff-update-all-btn"
                class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-[#00D4B2] text-xs font-semibold text-zinc-950 hover:bg-[#00D4B2]/90 transition-colors disabled:opacity-50 cursor-pointer"
              >
                <Show
                  when={props.updatingAll}
                  fallback={<Download class="w-3.5 h-3.5" />}
                >
                  <Loader2 class="w-3.5 h-3.5 animate-spin" />
                </Show>
                <span>
                  {props.updatingAll
                    ? "Обновление..."
                    : `Обновить все (${props.updates.length})`}
                </span>
              </button>
            </Show>
          </div>
        </div>
      </div>
    </Show>
  );
};
