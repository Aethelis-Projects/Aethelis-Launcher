import { Component, createSignal, Show, onCleanup } from "solid-js";
import { Package, FolderOpen, Loader2, CheckCircle2, AlertCircle, X, Download } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO, MrPackImportPlanDTO, MrPackImportStatusDTO } from "../../bindings/ipc_types";

interface MrPackImportModalProps {
  isOpen: boolean;
  onClose: () => void;
  onImported?: (instance: InstanceDTO) => void;
}

export const MrPackImportModal: Component<MrPackImportModalProps> = (props) => {
  const [filePath, setFilePath] = createSignal("");
  const [loadingPlan, setLoadingPlan] = createSignal(false);
  const [plan, setPlan] = createSignal<MrPackImportPlanDTO | null>(null);
  const [planError, setPlanError] = createSignal("");
  const [instanceName, setInstanceName] = createSignal("");
  const [isImporting, setIsImporting] = createSignal(false);
  const [importStatus, setImportStatus] = createSignal<MrPackImportStatusDTO | null>(null);
  const [importError, setImportError] = createSignal("");
  const [isCompleted, setIsCompleted] = createSignal(false);

  let pollTimer: ReturnType<typeof setInterval> | null = null;

  const stopPolling = () => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  };

  onCleanup(() => {
    stopPolling();
  });

  const resetState = () => {
    stopPolling();
    setFilePath("");
    setLoadingPlan(false);
    setPlan(null);
    setPlanError("");
    setInstanceName("");
    setIsImporting(false);
    setImportStatus(null);
    setImportError("");
    setIsCompleted(false);
  };

  const handleClose = () => {
    resetState();
    props.onClose();
  };

  const loadPlan = async (path: string) => {
    const trimmed = path.trim();
    if (!trimmed) return;

    setLoadingPlan(true);
    setPlanError("");
    setPlan(null);

    try {
      const p = await launcherAPI.getMrPackImportPlan(trimmed);
      setPlan(p);
      setInstanceName(p.name || "Modpack Instance");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setPlanError(`Не удалось прочитать пакет: ${msg}`);
    } finally {
      setLoadingPlan(false);
    }
  };

  const handleBrowse = async () => {
    try {
      const path = await launcherAPI.pickMrPackFile();
      if (path) {
        setFilePath(path);
        await loadPlan(path);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setPlanError(`Ошибка выбора файла: ${msg}`);
    }
  };

  const startStatusPolling = (name: string) => {
    stopPolling();
    const intervalMs = import.meta.env.MODE === "test" ? 50 : 500;
    pollTimer = setInterval(async () => {
      try {
        const status = await launcherAPI.getMrPackImportStatus(name);
        setImportStatus(status);

        if (status.status === "complete") {
          stopPolling();
          setIsImporting(false);
          setIsCompleted(true);
        } else if (status.status === "failed") {
          stopPolling();
          setIsImporting(false);
          setImportError(status.error || "Ошибка импорта модпака");
        }
      } catch (_err: unknown) {
        // non-fatal polling error
      }
    }, intervalMs);
  };

  const handleStartImport = async () => {
    const path = filePath().trim();
    const name = instanceName().trim();
    if (!path || !name) {
      setImportError("Укажите путь к .mrpack и имя сборки");
      return;
    }

    setIsImporting(true);
    setImportError("");
    setIsCompleted(false);
    setImportStatus({
      task_id: `import-${name}`,
      status: "downloading",
      percentage: 5,
      current_file: "Загрузка файлов .mrpack...",
      files_done: 0,
      total_files: plan()?.total_files || 0,
      bytes_read: 0,
      total_bytes: plan()?.total_size || 0,
      error: "",
    });

    try {
      startStatusPolling(name);
      const created = await launcherAPI.importMrPack({
        mrpack_path: path,
        instance_name: name,
      });

      if (props.onImported) {
        props.onImported(created);
      }
    } catch (err: unknown) {
      stopPolling();
      setIsImporting(false);
      const msg = err instanceof Error ? err.message : String(err);
      setImportError(`Ошибка импорта: ${msg}`);
    }
  };

  const formatSize = (bytes: number): string => {
    if (!bytes) return "0 B";
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  return (
    <Show when={props.isOpen}>
      <div
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4"
        data-testid="mrpack-import-modal"
      >
        <div class="w-full max-w-xl rounded-2xl bg-nord-surface border border-white/10 shadow-2xl overflow-hidden flex flex-col max-h-[90vh]">
          {/* Header */}
          <div class="px-6 py-4 border-b border-white/10 flex items-center justify-between bg-black/20">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-xl bg-nord-cyan/10 text-nord-cyan border border-nord-cyan/20">
                <Package class="w-5 h-5" />
              </div>
              <div>
                <h2 class="text-base font-bold text-white tracking-tight">
                  Импорт модпака (.mrpack)
                </h2>
                <p class="text-xs text-zinc-400 mt-0.5">
                  Быстрый импорт сборок формата Modrinth Modpack
                </p>
              </div>
            </div>

            <button
              type="button"
              onClick={handleClose}
              class="p-2 rounded-lg text-zinc-400 hover:text-white hover:bg-white/5 transition-colors cursor-pointer"
              title="Закрыть"
              data-testid="mrpack-modal-close-btn"
            >
              <X class="w-5 h-5" />
            </button>
          </div>

          {/* Body */}
          <div class="p-6 space-y-4 overflow-y-auto flex-1">
            {/* Error Banner */}
            <Show when={importError() || planError()}>
              <div
                class="p-3.5 rounded-xl bg-nord-rose/10 border border-nord-rose/20 text-xs text-nord-rose flex items-center justify-between gap-3 font-mono"
                data-testid="mrpack-import-error"
              >
                <div class="flex items-center gap-2">
                  <AlertCircle class="w-4 h-4 shrink-0" />
                  <span>{importError() || planError()}</span>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setImportError("");
                    setPlanError("");
                  }}
                  class="text-[11px] text-zinc-400 hover:text-zinc-200 underline cursor-pointer"
                >
                  Закрыть
                </button>
              </div>
            </Show>

            {/* Step 1: File selection */}
            <Show when={!isCompleted()}>
              <div class="space-y-2">
                <label class="text-xs font-semibold text-zinc-300 block">
                  Файл пакета (.mrpack)
                </label>
                <div class="flex items-center gap-2">
                  <input
                    type="text"
                    value={filePath()}
                    onInput={(e) => setFilePath(e.currentTarget.value)}
                    onChange={(e) => loadPlan(e.currentTarget.value)}
                    disabled={isImporting()}
                    placeholder="C:\Downloads\modpack.mrpack"
                    class="flex-1 px-3.5 py-2 rounded-xl bg-zinc-900 border border-white/10 text-white font-mono text-xs focus:outline-none focus:border-nord-cyan disabled:opacity-50"
                    data-testid="mrpack-path-input"
                  />
                  <button
                    type="button"
                    onClick={handleBrowse}
                    disabled={isImporting()}
                    class="px-3.5 py-2 rounded-xl bg-white/10 hover:bg-white/15 text-white font-medium text-xs flex items-center gap-1.5 transition-colors cursor-pointer disabled:opacity-50 shrink-0"
                    data-testid="mrpack-browse-btn"
                  >
                    <FolderOpen class="w-4 h-4 text-nord-cyan" />
                    <span>Обзор...</span>
                  </button>
                </div>
              </div>

              {/* Loading Plan Indicator */}
              <Show when={loadingPlan()}>
                <div class="p-6 rounded-xl bg-black/20 border border-white/5 flex items-center justify-center gap-2 text-xs font-mono text-nord-cyan">
                  <Loader2 class="w-4 h-4 animate-spin" />
                  <span>Чтение структуры .mrpack...</span>
                </div>
              </Show>

              {/* Plan Preview Card */}
              <Show when={plan() && !loadingPlan()}>
                <div
                  class="p-4 rounded-xl bg-black/30 border border-white/10 space-y-3"
                  data-testid="mrpack-plan-card"
                >
                  <div class="flex items-start justify-between gap-3">
                    <div>
                      <h3 class="font-bold text-white text-sm">
                        {plan()?.name}
                      </h3>
                      <Show when={plan()?.summary}>
                        <p class="text-xs text-zinc-400 mt-0.5 line-clamp-2">
                          {plan()?.summary}
                        </p>
                      </Show>
                    </div>
                    <span class="px-2 py-0.5 rounded text-[10px] font-mono uppercase bg-nord-cyan/15 text-nord-cyan border border-nord-cyan/30 shrink-0">
                      .mrpack
                    </span>
                  </div>

                  <div class="grid grid-cols-3 gap-2 pt-2 border-t border-white/5 text-xs font-mono">
                    <div class="p-2 rounded-lg bg-zinc-900/60 border border-white/5">
                      <span class="text-[10px] text-zinc-500 block">Версия игры</span>
                      <span class="text-white font-semibold">{plan()?.game_version}</span>
                    </div>
                    <div class="p-2 rounded-lg bg-zinc-900/60 border border-white/5">
                      <span class="text-[10px] text-zinc-500 block">Загрузчик</span>
                      <span class="text-nord-cyan font-semibold capitalize">
                        {plan()?.loader} {plan()?.loader_version || ""}
                      </span>
                    </div>
                    <div class="p-2 rounded-lg bg-zinc-900/60 border border-white/5">
                      <span class="text-[10px] text-zinc-500 block">Файлы / Размер</span>
                      <span class="text-zinc-300 font-semibold">
                        {plan()?.total_files} ({formatSize(plan()?.total_size || 0)})
                      </span>
                    </div>
                  </div>
                </div>

                {/* Instance Name Input */}
                <div class="space-y-2">
                  <label class="text-xs font-semibold text-zinc-300 block">
                    Имя новой сборки
                  </label>
                  <input
                    type="text"
                    value={instanceName()}
                    onInput={(e) => setInstanceName(e.currentTarget.value)}
                    disabled={isImporting()}
                    placeholder="Nordic Modpack"
                    class="w-full px-3.5 py-2 rounded-xl bg-zinc-900 border border-white/10 text-white font-mono text-xs focus:outline-none focus:border-nord-cyan disabled:opacity-50"
                    data-testid="mrpack-instance-name-input"
                  />
                </div>
              </Show>

              {/* Import Progress Card */}
              <Show when={isImporting() && importStatus()}>
                <div
                  class="p-4 rounded-xl bg-black/30 border border-nord-cyan/20 space-y-2.5"
                  data-testid="mrpack-import-progress"
                >
                  <div class="flex items-center justify-between text-xs">
                    <div class="flex items-center gap-2">
                      <Loader2 class="w-4 h-4 text-nord-cyan animate-spin" />
                      <span class="font-bold text-white capitalize">
                        {importStatus()?.status === "downloading" && "Загрузка модификаций..."}
                        {importStatus()?.status === "extracting" && "Распаковка переопределений..."}
                        {importStatus()?.status === "complete" && "Импорт завершен"}
                      </span>
                    </div>
                    <span class="font-mono text-zinc-400 font-semibold">
                      {importStatus()?.percentage?.toFixed(0) || 0}%
                    </span>
                  </div>

                  {/* Progress Bar */}
                  <div class="w-full h-2 rounded-full bg-zinc-800 overflow-hidden">
                    <div
                      class="h-full bg-nord-cyan transition-all duration-300 ease-out"
                      style={{ width: `${importStatus()?.percentage || 0}%` }}
                    />
                  </div>

                  <Show when={importStatus()?.current_file}>
                    <p class="text-[11px] font-mono text-zinc-500 truncate">
                      {importStatus()?.current_file}
                    </p>
                  </Show>
                </div>
              </Show>
            </Show>

            {/* Step 3: Success Screen */}
            <Show when={isCompleted()}>
              <div
                class="p-8 text-center space-y-3 bg-black/20 rounded-2xl border border-nord-emerald/20"
                data-testid="mrpack-import-success"
              >
                <div class="w-12 h-12 mx-auto rounded-full bg-nord-emerald/10 border border-nord-emerald/30 flex items-center justify-center text-nord-emerald">
                  <CheckCircle2 class="w-6 h-6" />
                </div>
                <h3 class="text-base font-bold text-white">
                  Сборка успешно импортирована!
                </h3>
                <p class="text-xs text-zinc-400 max-w-sm mx-auto">
                  Сборка "{instanceName()}" готова к запуску. Все файлы проверены и установлены.
                </p>
              </div>
            </Show>
          </div>

          {/* Footer Actions */}
          <div class="px-6 py-4 border-t border-white/10 flex items-center justify-end gap-3 bg-black/20">
            <Show
              when={!isCompleted()}
              fallback={
                <button
                  type="button"
                  onClick={handleClose}
                  class="px-4 py-2 rounded-xl bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark font-bold text-xs transition-colors cursor-pointer"
                  data-testid="mrpack-done-btn"
                >
                  Готово
                </button>
              }
            >
              <button
                type="button"
                onClick={handleClose}
                disabled={isImporting()}
                class="px-4 py-2 rounded-xl bg-white/5 hover:bg-white/10 text-zinc-300 font-medium text-xs transition-colors cursor-pointer disabled:opacity-50"
              >
                Отмена
              </button>
              <button
                type="button"
                onClick={handleStartImport}
                disabled={isImporting() || !plan() || !instanceName().trim()}
                class="px-4 py-2 rounded-xl bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark font-bold text-xs flex items-center gap-2 transition-all cursor-pointer disabled:opacity-50 shadow-[0_0_12px_rgba(0,212,178,0.2)]"
                data-testid="mrpack-submit-btn"
              >
                <Show when={isImporting()} fallback={<Download class="w-4 h-4" />}>
                  <Loader2 class="w-4 h-4 animate-spin" />
                </Show>
                <span>{isImporting() ? "Импорт..." : "Импортировать"}</span>
              </button>
            </Show>
          </div>
        </div>
      </div>
    </Show>
  );
};
