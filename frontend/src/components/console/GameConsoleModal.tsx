import {
  Component,
  createSignal,
  createMemo,
  createEffect,
  onMount,
  onCleanup,
  Show,
  For,
} from "solid-js";
import {
  Terminal,
  Search,
  Copy,
  Check,
  Download,
  Trash2,
  X,
  AlertOctagon,
  ArrowDown,
  RefreshCw,
  FolderOpen,
} from "lucide-solid";
import type { InstanceDTO } from "../../bindings/ipc_types";
import { launcherAPI } from "../../services/api";

interface GameConsoleModalProps {
  isOpen: boolean;
  onClose: () => void;
  instance: InstanceDTO | null;
  onOpenCrash?: () => void;
}

type LogLevelFilter = "ALL" | "INFO" | "WARN" | "ERROR";

export const GameConsoleModal: Component<GameConsoleModalProps> = (props) => {
  const [lines, setLines] = createSignal<string[]>([]);
  const [searchQuery, setSearchQuery] = createSignal("");
  const [selectedLevel, setSelectedLevel] = createSignal<LogLevelFilter>("ALL");
  const [autoScroll, setAutoScroll] = createSignal(true);
  const [copied, setCopied] = createSignal(false);
  const [saveStatus, setSaveStatus] = createSignal<string | null>(null);
  const [isSaving, setIsSaving] = createSignal(false);
  const [isLoading, setIsLoading] = createSignal(false);

  let logContainerRef: HTMLDivElement | undefined;

  const fetchLogs = async () => {
    if (!props.instance) return;
    setIsLoading(true);
    try {
      const result = await launcherAPI.getGameLogs(props.instance.id);
      if (Array.isArray(result)) {
        setLines(result);
      }
    } catch (_err) {
      // non-fatal fetch error
    } finally {
      setIsLoading(false);
    }
  };

  createEffect(() => {
    if (props.isOpen && props.instance) {
      fetchLogs();
    }
  });

  onMount(() => {
    let cleanupWailsListener: (() => void) | undefined;

    // Subscribe to Wails v3 custom events if available
    const setupWailsListener = () => {
      const w = window as unknown as {
        wails?: {
          Events?: {
            On?: (name: string, callback: (event: unknown) => void) => () => void;
          };
        };
      };
      if (w.wails?.Events?.On) {
        cleanupWailsListener = w.wails.Events.On("game:log_batch", (event: unknown) => {
          const raw = event as { data?: { instance_id?: string; lines?: string[] }; instance_id?: string; lines?: string[] };
          const data = raw?.data || raw;
          if (data?.instance_id === props.instance?.id && Array.isArray(data.lines)) {
            setLines((prev) => {
              const combined = [...prev, ...data.lines!];
              if (combined.length > 5000) {
                return combined.slice(combined.length - 5000);
              }
              return combined;
            });
          }
        });
      }
    };

    setupWailsListener();

    // Polling interval while modal is open and instance is running
    const timer = setInterval(() => {
      if (props.isOpen && props.instance && props.instance.state === "running") {
        fetchLogs();
      }
    }, 1500);

    onCleanup(() => {
      clearInterval(timer);
      if (cleanupWailsListener) {
        cleanupWailsListener();
      }
    });
  });

  const filteredLines = createMemo(() => {
    const query = searchQuery().toLowerCase().trim();
    const level = selectedLevel();
    const all = lines();

    return all.filter((line) => {
      // Level filter
      if (level === "INFO" && !line.includes("INFO")) {
        return false;
      }
      if (level === "WARN" && !line.includes("WARN")) {
        return false;
      }
      if (
        level === "ERROR" &&
        !line.includes("ERROR") &&
        !line.includes("FATAL") &&
        !line.includes("Exception")
      ) {
        return false;
      }

      // Search filter
      if (query && !line.toLowerCase().includes(query)) {
        return false;
      }

      return true;
    });
  });

  createEffect(() => {
    filteredLines();
    if (autoScroll() && logContainerRef) {
      setTimeout(() => {
        if (logContainerRef) {
          logContainerRef.scrollTop = logContainerRef.scrollHeight;
        }
      }, 0);
    }
  });

  const handleCopy = async () => {
    const text = filteredLines().join("\n");
    if (!text) return;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (_err) {
      // ignore
    }
  };

  const handleSave = async () => {
    if (!props.instance) return;
    setIsSaving(true);
    setSaveStatus(null);
    try {
      const res = await launcherAPI.saveGameLog({ instance_id: props.instance.id });
      if (res.success) {
        setSaveStatus(`Сохранено: ${res.file_path}`);
      } else {
        setSaveStatus(res.error || "Ошибка сохранения лога");
      }
    } catch (e) {
      setSaveStatus(e instanceof Error ? e.message : "Не удалось сохранить лог");
    } finally {
      setIsSaving(false);
      setTimeout(() => setSaveStatus(null), 4000);
    }
  };

  const handleClear = () => {
    setLines([]);
  };

  const getLineClass = (line: string): string => {
    if (line.includes("ERROR") || line.includes("FATAL")) {
      return "text-red-400 bg-red-950/20";
    }
    if (line.includes("WARN")) {
      return "text-amber-400 bg-amber-950/15";
    }
    if (line.includes("DEBUG") || line.includes("TRACE")) {
      return "text-zinc-500";
    }
    return "text-zinc-300";
  };

  return (
    <Show when={props.isOpen && props.instance}>
      <div
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/85 backdrop-blur-md p-4 animate-in fade-in duration-200"
        data-testid="game-console-modal"
      >
        <div class="w-full max-w-5xl bg-zinc-950 border border-zinc-800 rounded-xl shadow-2xl overflow-hidden flex flex-col h-[85vh] max-h-[820px]">
          {/* Header */}
          <div class="bg-zinc-900/90 border-b border-zinc-800 px-5 py-3.5 flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-nord-cyan/10 border border-nord-cyan/20">
                <Terminal class="w-5 h-5 text-nord-cyan" />
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h2 class="text-sm font-semibold text-zinc-100">
                    Консоль процесса: {props.instance!.name}
                  </h2>
                  <span
                    class={`text-[10px] font-mono uppercase tracking-wider px-2 py-0.5 rounded border ${
                      props.instance!.state === "running"
                        ? "bg-emerald-500/10 border-emerald-500/30 text-emerald-400"
                        : props.instance!.state === "crashed"
                        ? "bg-red-500/10 border-red-500/30 text-red-400"
                        : "bg-zinc-800 border-zinc-700 text-zinc-400"
                    }`}
                  >
                    {props.instance!.state}
                  </span>
                </div>
                <div class="text-[11px] font-mono text-zinc-400 flex items-center gap-2 mt-0.5">
                  <span>
                    {filteredLines().length} из {lines().length} строк
                  </span>
                  <span>|</span>
                  <span>Буфер: до 5 000 строк</span>
                </div>
              </div>
            </div>

            <div class="flex items-center gap-2">
              <button
                type="button"
                onClick={fetchLogs}
                disabled={isLoading()}
                title="Обновить журнал"
                class="p-1.5 rounded-lg border border-zinc-700 text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800 transition-colors disabled:opacity-50"
              >
                <RefreshCw class={`w-4 h-4 ${isLoading() ? "animate-spin text-nord-cyan" : ""}`} />
              </button>
              <button
                type="button"
                onClick={props.onClose}
                class="text-zinc-400 hover:text-zinc-100 p-1.5 rounded-lg border border-transparent hover:border-zinc-700 hover:bg-zinc-800 transition-colors"
                title="Закрыть консоль"
              >
                <X class="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Crash Alert Banner */}
          <Show when={props.instance!.state === "crashed" && props.onOpenCrash}>
            <div class="bg-red-950/50 border-b border-red-500/40 px-5 py-2.5 flex items-center justify-between">
              <div class="flex items-center gap-2 text-xs text-red-300">
                <AlertOctagon class="w-4 h-4 text-red-400 shrink-0" />
                <span>
                  Игра аварийно завершилась. Доступна автоматическая диагностика сбоя.
                </span>
              </div>
              <button
                type="button"
                data-testid="console-crash-diag-btn"
                onClick={props.onOpenCrash}
                class="px-3 py-1 rounded bg-red-600/80 hover:bg-red-600 text-white font-mono text-xs flex items-center gap-1.5 transition-colors shadow-sm"
              >
                <AlertOctagon class="w-3.5 h-3.5" />
                Диагностика сбоя
              </button>
            </div>
          </Show>

          {/* Control Bar: Filters & Actions */}
          <div class="bg-zinc-900/60 border-b border-zinc-800 px-5 py-2.5 flex flex-wrap items-center justify-between gap-3">
            {/* Search and Level Filters */}
            <div class="flex items-center gap-2.5 flex-1 min-w-[280px]">
              <div class="relative flex-1 max-w-xs">
                <Search class="w-3.5 h-3.5 text-zinc-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
                <input
                  type="text"
                  data-testid="console-search-input"
                  placeholder="Поиск по логам..."
                  value={searchQuery()}
                  onInput={(e) => setSearchQuery(e.currentTarget.value)}
                  class="w-full pl-8 pr-7 py-1 bg-zinc-950 border border-zinc-800 rounded text-xs text-zinc-200 placeholder-zinc-500 focus:outline-none focus:border-nord-cyan transition-colors font-mono"
                />
                <Show when={searchQuery().length > 0}>
                  <button
                    type="button"
                    onClick={() => setSearchQuery("")}
                    class="absolute right-2 top-1/2 -translate-y-1/2 text-zinc-500 hover:text-zinc-300"
                  >
                    <X class="w-3.5 h-3.5" />
                  </button>
                </Show>
              </div>

              {/* Level Buttons */}
              <div class="flex items-center bg-zinc-950 border border-zinc-800 rounded p-0.5 text-xs font-mono">
                <button
                  type="button"
                  data-testid="console-level-all"
                  onClick={() => setSelectedLevel("ALL")}
                  class={`px-2 py-0.5 rounded transition-colors ${
                    selectedLevel() === "ALL"
                      ? "bg-zinc-800 text-zinc-100 font-semibold"
                      : "text-zinc-400 hover:text-zinc-200"
                  }`}
                >
                  ALL
                </button>
                <button
                  type="button"
                  data-testid="console-level-info"
                  onClick={() => setSelectedLevel("INFO")}
                  class={`px-2 py-0.5 rounded transition-colors ${
                    selectedLevel() === "INFO"
                      ? "bg-blue-900/50 text-blue-300 font-semibold"
                      : "text-zinc-400 hover:text-zinc-200"
                  }`}
                >
                  INFO
                </button>
                <button
                  type="button"
                  data-testid="console-level-warn"
                  onClick={() => setSelectedLevel("WARN")}
                  class={`px-2 py-0.5 rounded transition-colors ${
                    selectedLevel() === "WARN"
                      ? "bg-amber-900/50 text-amber-300 font-semibold"
                      : "text-zinc-400 hover:text-zinc-200"
                  }`}
                >
                  WARN
                </button>
                <button
                  type="button"
                  data-testid="console-level-error"
                  onClick={() => setSelectedLevel("ERROR")}
                  class={`px-2 py-0.5 rounded transition-colors ${
                    selectedLevel() === "ERROR"
                      ? "bg-red-900/50 text-red-300 font-semibold"
                      : "text-zinc-400 hover:text-zinc-200"
                  }`}
                >
                  ERROR
                </button>
              </div>
            </div>

            {/* Actions */}
            <div class="flex items-center gap-2">
              {/* Autoscroll Toggle */}
              <button
                type="button"
                data-testid="console-autoscroll-btn"
                onClick={() => setAutoScroll(!autoScroll())}
                class={`px-2.5 py-1 rounded border font-mono text-xs flex items-center gap-1.5 transition-colors ${
                  autoScroll()
                    ? "bg-nord-cyan/15 border-nord-cyan/40 text-nord-cyan"
                    : "bg-zinc-900 border-zinc-800 text-zinc-400 hover:text-zinc-200"
                }`}
                title="Автопрокрутка к последней строке"
              >
                <ArrowDown class="w-3.5 h-3.5" />
                Автопрокрутка: {autoScroll() ? "ВКЛ" : "ВЫКЛ"}
              </button>

              {/* Clear button */}
              <button
                type="button"
                data-testid="console-clear-btn"
                onClick={handleClear}
                class="px-2.5 py-1 rounded bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 text-zinc-300 font-mono text-xs flex items-center gap-1.5 transition-colors"
                title="Очистить текущий вывод"
              >
                <Trash2 class="w-3.5 h-3.5" />
                Очистить
              </button>

              {/* Copy button */}
              <button
                type="button"
                data-testid="console-copy-btn"
                onClick={handleCopy}
                class="px-2.5 py-1 rounded bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 text-zinc-300 font-mono text-xs flex items-center gap-1.5 transition-colors"
              >
                <Show when={copied()} fallback={<Copy class="w-3.5 h-3.5" />}>
                  <Check class="w-3.5 h-3.5 text-nord-emerald" />
                </Show>
                {copied() ? "Скопировано" : "Копировать"}
              </button>

              {/* Save log button */}
              <button
                type="button"
                data-testid="console-save-btn"
                disabled={isSaving()}
                onClick={handleSave}
                class="px-3 py-1 rounded bg-nord-cyan/20 hover:bg-nord-cyan/30 border border-nord-cyan/40 text-nord-cyan font-mono text-xs flex items-center gap-1.5 transition-colors disabled:opacity-50"
              >
                <Download class="w-3.5 h-3.5" />
                {isSaving() ? "Сохранение..." : "Сохранить лог"}
              </button>
            </div>
          </div>

          {/* Toast / Status banner if save executed */}
          <Show when={saveStatus()}>
            <div class="bg-zinc-900 border-b border-zinc-800 px-5 py-2 text-xs font-mono text-zinc-200 flex items-center justify-between">
              <span class="truncate">{saveStatus()}</span>
              <button
                type="button"
                onClick={() => setSaveStatus(null)}
                class="text-zinc-500 hover:text-zinc-300 p-0.5 ml-2"
              >
                <X class="w-3.5 h-3.5" />
              </button>
            </div>
          </Show>

          {/* Main Log Display Area */}
          <div
            ref={logContainerRef}
            class="flex-1 p-4 overflow-y-auto bg-zinc-950 font-mono text-xs leading-relaxed select-text space-y-0.5"
            data-testid="console-lines-container"
          >
            <Show
              when={filteredLines().length > 0}
              fallback={
                <div class="h-full flex flex-col items-center justify-center text-zinc-600 space-y-2 py-12">
                  <Terminal class="w-8 h-8 opacity-40" />
                  <p class="font-mono text-xs">
                    {lines().length === 0
                      ? "Журнал пуст. Запустите инстанс для просмотра вывода игры."
                      : "Нет строк, соответствующих заданному фильтру."}
                  </p>
                </div>
              }
            >
              <For each={filteredLines()}>
                {(line) => (
                  <div class={`px-2 py-0.5 rounded break-all font-mono ${getLineClass(line)}`}>
                    {line}
                  </div>
                )}
              </For>
            </Show>
          </div>

          {/* Footer Info */}
          <div class="bg-zinc-900/80 border-t border-zinc-800 px-5 py-2.5 flex items-center justify-between text-xs text-zinc-500">
            <div class="flex items-center gap-2">
              <span class="font-mono">Nord Engine Console v0.7.0</span>
            </div>
            <div class="flex items-center gap-3">
              <button
                type="button"
                onClick={() => launcherAPI.openPath(props.instance!.id)}
                class="hover:text-zinc-300 flex items-center gap-1 font-mono transition-colors"
              >
                <FolderOpen class="w-3.5 h-3.5" />
                Папка инстанса
              </button>
              <button
                type="button"
                onClick={props.onClose}
                class="px-3 py-1 rounded bg-zinc-800 hover:bg-zinc-700 text-zinc-300 font-mono transition-colors"
              >
                Закрыть
              </button>
            </div>
          </div>
        </div>
      </div>
    </Show>
  );
};
