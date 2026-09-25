import { Component, createSignal, createEffect, Show } from "solid-js";
import { Package, Loader2, CheckCircle2, AlertCircle, X, Upload, FolderOpen } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO } from "../../bindings/ipc_types";

interface MrPackExportModalProps {
  instance: InstanceDTO;
  isOpen: boolean;
  onClose: () => void;
  onExported?: (exportPath: string) => void;
}

export const MrPackExportModal: Component<MrPackExportModalProps> = (props) => {
  const [packName, setPackName] = createSignal("");
  const [versionId, setVersionId] = createSignal("1.0.0");
  const [summary, setSummary] = createSignal("");
  const [isExporting, setIsExporting] = createSignal(false);
  const [exportPath, setExportPath] = createSignal("");
  const [error, setError] = createSignal("");

  createEffect(() => {
    if (props.isOpen && props.instance) {
      setPackName(props.instance.name || "My Modpack");
      setVersionId("1.0.0");
      setSummary(`${props.instance.name} for Minecraft ${props.instance.game_version}`);
      setIsExporting(false);
      setExportPath("");
      setError("");
    }
  });

  const handleClose = () => {
    setIsExporting(false);
    setExportPath("");
    setError("");
    props.onClose();
  };

  const handleExport = async () => {
    const name = packName().trim();
    const ver = versionId().trim();
    const sum = summary().trim();

    if (!name || !ver) {
      setError("Укажите имя пакета и версию");
      return;
    }

    setIsExporting(true);
    setError("");

    try {
      const path = await launcherAPI.exportMrPack({
        instance_id: props.instance.id,
        name,
        version: ver,
        summary: sum,
      });

      setExportPath(path);
      if (props.onExported) {
        props.onExported(path);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Ошибка экспорта: ${msg}`);
    } finally {
      setIsExporting(false);
    }
  };

  return (
    <Show when={props.isOpen}>
      <div
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4"
        data-testid="mrpack-export-modal"
      >
        <div class="w-full max-w-lg rounded-2xl bg-nord-surface border border-white/10 shadow-2xl overflow-hidden flex flex-col">
          {/* Header */}
          <div class="px-6 py-4 border-b border-white/10 flex items-center justify-between bg-black/20">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-xl bg-nord-cyan/10 text-nord-cyan border border-nord-cyan/20">
                <Package class="w-5 h-5" />
              </div>
              <div>
                <h2 class="text-base font-bold text-white tracking-tight">
                  Экспорт сборки в .mrpack
                </h2>
                <p class="text-xs text-zinc-400 mt-0.5">
                  Создание стандартного пакета Modrinth Modpack
                </p>
              </div>
            </div>

            <button
              type="button"
              onClick={handleClose}
              class="p-2 rounded-lg text-zinc-400 hover:text-white hover:bg-white/5 transition-colors cursor-pointer"
              title="Закрыть"
              data-testid="mrpack-export-close-btn"
            >
              <X class="w-5 h-5" />
            </button>
          </div>

          {/* Body */}
          <div class="p-6 space-y-4">
            {/* Error Banner */}
            <Show when={error()}>
              <div
                class="p-3.5 rounded-xl bg-nord-rose/10 border border-nord-rose/20 text-xs text-nord-rose flex items-center justify-between gap-3 font-mono"
                data-testid="mrpack-export-error"
              >
                <div class="flex items-center gap-2">
                  <AlertCircle class="w-4 h-4 shrink-0" />
                  <span>{error()}</span>
                </div>
                <button
                  type="button"
                  onClick={() => setError("")}
                  class="text-[11px] text-zinc-400 hover:text-zinc-200 underline cursor-pointer"
                >
                  Закрыть
                </button>
              </div>
            </Show>

            <Show
              when={!exportPath()}
              fallback={
                <div
                  class="p-6 text-center space-y-3 bg-black/20 rounded-2xl border border-nord-emerald/20"
                  data-testid="mrpack-export-success"
                >
                  <div class="w-12 h-12 mx-auto rounded-full bg-nord-emerald/10 border border-nord-emerald/30 flex items-center justify-center text-nord-emerald">
                    <CheckCircle2 class="w-6 h-6" />
                  </div>
                  <h3 class="text-base font-bold text-white">
                    Пакет успешно создан!
                  </h3>
                  <p class="text-xs font-mono text-zinc-400 break-all bg-black/40 p-3 rounded-lg border border-white/5">
                    {exportPath()}
                  </p>
                  <p class="text-xs text-zinc-500">
                    Пакет содержит манифест modrinth.index.json и все модификации/конфигурации.
                  </p>
                  <div class="pt-2">
                    <button
                      type="button"
                      onClick={() => launcherAPI.openPath(exportPath())}
                      class="px-3.5 py-2 rounded-xl bg-white/5 hover:bg-white/10 text-zinc-200 border border-white/10 text-xs font-medium inline-flex items-center gap-2 transition-colors cursor-pointer"
                      data-testid="open-export-folder-button"
                    >
                      <FolderOpen class="w-4 h-4 text-nord-cyan" />
                      <span>Открыть папку экспорта</span>
                    </button>
                  </div>
                </div>
              }
            >
              <div class="space-y-3">
                <div>
                  <label class="text-xs font-semibold text-zinc-300 block mb-1">
                    Название пакета
                  </label>
                  <input
                    type="text"
                    value={packName()}
                    onInput={(e) => setPackName(e.currentTarget.value)}
                    disabled={isExporting()}
                    placeholder="Мой модпак"
                    class="w-full px-3.5 py-2 rounded-xl bg-zinc-900 border border-white/10 text-white font-mono text-xs focus:outline-none focus:border-nord-cyan disabled:opacity-50"
                    data-testid="mrpack-export-name-input"
                  />
                </div>

                <div>
                  <label class="text-xs font-semibold text-zinc-300 block mb-1">
                    Версия
                  </label>
                  <input
                    type="text"
                    value={versionId()}
                    onInput={(e) => setVersionId(e.currentTarget.value)}
                    disabled={isExporting()}
                    placeholder="1.0.0"
                    class="w-full px-3.5 py-2 rounded-xl bg-zinc-900 border border-white/10 text-white font-mono text-xs focus:outline-none focus:border-nord-cyan disabled:opacity-50"
                    data-testid="mrpack-export-version-input"
                  />
                </div>

                <div>
                  <label class="text-xs font-semibold text-zinc-300 block mb-1">
                    Описание
                  </label>
                  <textarea
                    value={summary()}
                    onInput={(e) => setSummary(e.currentTarget.value)}
                    disabled={isExporting()}
                    rows={3}
                    placeholder="Краткое описание сборки..."
                    class="w-full px-3.5 py-2 rounded-xl bg-zinc-900 border border-white/10 text-white font-mono text-xs focus:outline-none focus:border-nord-cyan disabled:opacity-50 resize-none"
                    data-testid="mrpack-export-summary-input"
                  />
                </div>

                <div class="p-3 rounded-xl bg-black/20 border border-white/5 text-xs font-mono space-y-1 text-zinc-400">
                  <div class="flex justify-between">
                    <span>Игра:</span>
                    <span class="text-zinc-200">Minecraft {props.instance.game_version}</span>
                  </div>
                  <div class="flex justify-between">
                    <span>Загрузчик:</span>
                    <span class="text-nord-cyan capitalize">{props.instance.loader}</span>
                  </div>
                </div>
              </div>
            </Show>
          </div>

          {/* Footer */}
          <div class="px-6 py-4 border-t border-white/10 flex items-center justify-end gap-3 bg-black/20">
            <Show
              when={!exportPath()}
              fallback={
                <button
                  type="button"
                  onClick={handleClose}
                  class="px-4 py-2 rounded-xl bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark font-bold text-xs transition-colors cursor-pointer"
                  data-testid="mrpack-export-done-btn"
                >
                  Готово
                </button>
              }
            >
              <button
                type="button"
                onClick={handleClose}
                disabled={isExporting()}
                class="px-4 py-2 rounded-xl bg-white/5 hover:bg-white/10 text-zinc-300 font-medium text-xs transition-colors cursor-pointer disabled:opacity-50"
              >
                Отмена
              </button>
              <button
                type="button"
                onClick={handleExport}
                disabled={isExporting() || !packName().trim() || !versionId().trim()}
                class="px-4 py-2 rounded-xl bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark font-bold text-xs flex items-center gap-2 transition-all cursor-pointer disabled:opacity-50 shadow-[0_0_12px_rgba(0,212,178,0.2)]"
                data-testid="mrpack-export-submit-btn"
              >
                <Show when={isExporting()} fallback={<Upload class="w-4 h-4" />}>
                  <Loader2 class="w-4 h-4 animate-spin" />
                </Show>
                <span>{isExporting() ? "Экспорт..." : "Экспортировать"}</span>
              </button>
            </Show>
          </div>
        </div>
      </div>
    </Show>
  );
};
