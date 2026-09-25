import { Component, createSignal, onCleanup, onMount, For, Show } from "solid-js";
import { Cpu, Download, Plus, Trash2, CheckCircle2, AlertCircle, Loader2, X, RefreshCw, ArrowUpCircle } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { JavaInstallationDTO, JavaDownloadStatusDTO, JavaRuntimeUpdateDTO } from "../../bindings/ipc_types";

interface JavaManagerProps {
  onClose?: () => void;
}

export const JavaManager: Component<JavaManagerProps> = (props) => {
  const [runtimes, setRuntimes] = createSignal<JavaInstallationDTO[]>([]);
  const [updates, setUpdates] = createSignal<JavaRuntimeUpdateDTO[]>([]);
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");
  const [customPath, setCustomPath] = createSignal("");
  const [addingPath, setAddingPath] = createSignal(false);
  const [downloadStatus, setDownloadStatus] = createSignal<JavaDownloadStatusDTO | null>(null);
  const [upgradingMajor, setUpgradingMajor] = createSignal<number | null>(null);
  const [isCleaningUnused, setIsCleaningUnused] = createSignal(false);

  let pollTimer: ReturnType<typeof setInterval> | null = null;

  const stopPolling = () => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  };

  const refreshRuntimes = async () => {
    setLoading(true);
    setError("");
    try {
      const list = await launcherAPI.listJavaRuntimes();
      setRuntimes(list);
      try {
        const upd = await launcherAPI.checkJavaRuntimeUpdates();
        setUpdates(upd || []);
      } catch (_err: unknown) {
        // non-fatal check failure
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Ошибка загрузки рантаймов: ${msg}`);
    } finally {
      setLoading(false);
    }
  };

  onMount(() => {
    refreshRuntimes();
  });

  onCleanup(() => {
    stopPolling();
  });

  const startDownload = async (major: number) => {
    setError("");
    try {
      await launcherAPI.downloadJavaRuntime(major);
      startStatusPolling();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Не удалось начать скачивание: ${msg}`);
    }
  };

  const startStatusPolling = () => {
    stopPolling();
    const intervalMs = import.meta.env.MODE === "test" ? 50 : 500;
    pollTimer = setInterval(async () => {
      try {
        const status = await launcherAPI.getJavaDownloadStatus();
        setDownloadStatus(status);

        if (status.status === "ready") {
          stopPolling();
          refreshRuntimes();
        } else if (status.status === "failed") {
          stopPolling();
          if (status.error) {
            setError(`Ошибка загрузки Java: ${status.error}`);
          }
        }
      } catch (_err: unknown) {
        // non-fatal polling error
      }
    }, intervalMs);
  };

  const handleAddCustom = async () => {
    const p = customPath().trim();
    if (!p) {
      setError("Укажите путь к папке JDK или исполняемому файлу java");
      return;
    }

    setAddingPath(true);
    setError("");

    try {
      await launcherAPI.addJavaRuntime(p);
      setCustomPath("");
      await refreshRuntimes();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Не удалось добавить рантайм: ${msg}`);
    } finally {
      setAddingPath(false);
    }
  };

  const handleRemove = async (rt: JavaInstallationDTO) => {
    if (rt.used_by && rt.used_by.length > 0) {
      setError(`Невозможно удалить рантайм: используется сборками (${rt.used_by.join(", ")})`);
      return;
    }

    setError("");
    try {
      await launcherAPI.removeJavaRuntime(rt.path);
      await refreshRuntimes();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Не удалось удалить рантайм: ${msg}`);
    }
  };

  const isDownloading = () => {
    const s = downloadStatus()?.status;
    return s === "downloading" || s === "extracting";
  };

  const handleUpgrade = async (major: number) => {
    setUpgradingMajor(major);
    setError("");
    try {
      await launcherAPI.upgradeJavaRuntime(major);
      await refreshRuntimes();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Не удалось обновить Java ${major}: ${msg}`);
    } finally {
      setUpgradingMajor(null);
    }
  };

  const handleCleanUnused = async () => {
    setIsCleaningUnused(true);
    setError("");
    try {
      const unusedList = runtimes().filter(
        (r) => r.kind === "managed" && (!r.used_by || r.used_by.length === 0)
      );
      for (const rt of unusedList) {
        await launcherAPI.removeJavaRuntime(rt.path);
      }
      await refreshRuntimes();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Не удалось очистить неиспользуемые рантаймы: ${msg}`);
    } finally {
      setIsCleaningUnused(false);
    }
  };

  const unusedManagedCount = () =>
    runtimes().filter(
      (r) => r.kind === "managed" && (!r.used_by || r.used_by.length === 0)
    ).length;

  return (
    <div class="space-y-6 max-w-4xl mx-auto" data-testid="java-manager-view">
      {/* Header */}
      <div class="flex items-center justify-between border-b border-white/5 pb-4">
        <div class="flex items-center gap-3">
          <div class="p-2.5 rounded-xl bg-nord-cyan/10 text-nord-cyan border border-nord-cyan/20">
            <Cpu class="w-6 h-6" />
          </div>
          <div>
            <h1 class="text-xl font-bold text-white tracking-tight">
              Менеджер сред выполнения Java
            </h1>
            <p class="text-xs text-zinc-400 mt-0.5">
              Управление версиями Adoptium OpenJDK и обнаруженными рантаймами в системе
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            onClick={refreshRuntimes}
            disabled={loading()}
            class="p-2 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-300 transition-colors cursor-pointer disabled:opacity-50"
            title="Обновить список"
            data-testid="java-refresh-button"
          >
            <RefreshCw class={`w-4 h-4 ${loading() ? "animate-spin" : ""}`} />
          </button>
          <Show when={props.onClose}>
            <button
              type="button"
              onClick={props.onClose}
              class="p-2 rounded-lg text-zinc-400 hover:text-white hover:bg-white/5 transition-colors cursor-pointer"
              title="Закрыть"
              data-testid="java-close-button"
            >
              <X class="w-5 h-5" />
            </button>
          </Show>
        </div>
      </div>

      {/* Error Banner */}
      <Show when={error()}>
        <div
          class="p-3.5 rounded-xl bg-nord-rose/10 border border-nord-rose/20 text-xs text-nord-rose flex items-center justify-between gap-3 font-mono"
          data-testid="java-error-banner"
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

      {/* Quick Download Adoptium Section */}
      <div class="p-5 rounded-2xl bg-nord-surface border border-white/10 shadow-lg space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-sm font-bold text-white tracking-tight">
              Установить рекомендуемый Adoptium OpenJDK
            </h2>
            <p class="text-xs text-zinc-400 mt-0.5">
              Пакеты загружаются с официального API Eclipse Adoptium и верифицируются по SHA-256
            </p>
          </div>
        </div>

        {/* Action Buttons */}
        <div class="grid grid-cols-2 md:grid-cols-5 gap-3">
          <button
            type="button"
            onClick={() => startDownload(25)}
            disabled={isDownloading()}
            class="p-3.5 rounded-xl bg-zinc-900 border border-white/10 hover:border-nord-cyan/40 text-left transition-all cursor-pointer disabled:opacity-50 group"
            data-testid="download-java-25-button"
          >
            <div class="flex items-center justify-between mb-1">
              <span class="font-bold text-white text-xs group-hover:text-nord-cyan transition-colors">
                Java 25 LTS
              </span>
              <Download class="w-4 h-4 text-zinc-500 group-hover:text-nord-cyan transition-colors" />
            </div>
            <p class="text-[11px] text-zinc-400">
              Minecraft 26.1+ (Temurin 25)
            </p>
          </button>

          <button
            type="button"
            onClick={() => startDownload(21)}
            disabled={isDownloading()}
            class="p-3.5 rounded-xl bg-zinc-900 border border-white/10 hover:border-nord-cyan/40 text-left transition-all cursor-pointer disabled:opacity-50 group"
            data-testid="download-java-21-button"
          >
            <div class="flex items-center justify-between mb-1">
              <span class="font-bold text-white text-xs group-hover:text-nord-cyan transition-colors">
                Java 21 LTS
              </span>
              <Download class="w-4 h-4 text-zinc-500 group-hover:text-nord-cyan transition-colors" />
            </div>
            <p class="text-[11px] text-zinc-400">
              Minecraft 1.20.5 - 26.0 (Temurin 21)
            </p>
          </button>

          <button
            type="button"
            onClick={() => startDownload(17)}
            disabled={isDownloading()}
            class="p-3.5 rounded-xl bg-zinc-900 border border-white/10 hover:border-nord-cyan/40 text-left transition-all cursor-pointer disabled:opacity-50 group"
            data-testid="download-java-17-button"
          >
            <div class="flex items-center justify-between mb-1">
              <span class="font-bold text-white text-xs group-hover:text-nord-cyan transition-colors">
                Java 17 LTS
              </span>
              <Download class="w-4 h-4 text-zinc-500 group-hover:text-nord-cyan transition-colors" />
            </div>
            <p class="text-[11px] text-zinc-400">
              Minecraft 1.17 - 1.20.4 (Temurin 17)
            </p>
          </button>

          <button
            type="button"
            onClick={() => startDownload(11)}
            disabled={isDownloading()}
            class="p-3.5 rounded-xl bg-zinc-900 border border-white/10 hover:border-nord-cyan/40 text-left transition-all cursor-pointer disabled:opacity-50 group"
            data-testid="download-java-11-button"
          >
            <div class="flex items-center justify-between mb-1">
              <span class="font-bold text-white text-xs group-hover:text-nord-cyan transition-colors">
                Java 11 LTS
              </span>
              <Download class="w-4 h-4 text-zinc-500 group-hover:text-nord-cyan transition-colors" />
            </div>
            <p class="text-[11px] text-zinc-400">
              Legacy / Модпаки (Temurin 11)
            </p>
          </button>

          <button
            type="button"
            onClick={() => startDownload(8)}
            disabled={isDownloading()}
            class="p-3.5 rounded-xl bg-zinc-900 border border-white/10 hover:border-nord-cyan/40 text-left transition-all cursor-pointer disabled:opacity-50 group"
            data-testid="download-java-8-button"
          >
            <div class="flex items-center justify-between mb-1">
              <span class="font-bold text-white text-xs group-hover:text-nord-cyan transition-colors">
                Java 8 LTS
              </span>
              <Download class="w-4 h-4 text-zinc-500 group-hover:text-nord-cyan transition-colors" />
            </div>
            <p class="text-[11px] text-zinc-400">
              Minecraft 1.16.5 и старее (Temurin 8)
            </p>
          </button>
        </div>

        {/* Live Download / Extraction Progress Card */}
        <Show when={downloadStatus() && downloadStatus()?.status !== "idle"}>
          <div
            class="p-4 rounded-xl bg-black/30 border border-nord-cyan/20 space-y-2.5"
            data-testid="java-download-progress"
          >
            <div class="flex items-center justify-between text-xs">
              <div class="flex items-center gap-2">
                <Show
                  when={isDownloading()}
                  fallback={<CheckCircle2 class="w-4 h-4 text-nord-emerald" />}
                >
                  <Loader2 class="w-4 h-4 text-nord-cyan animate-spin" />
                </Show>
                <span class="font-bold text-white capitalize">
                  {downloadStatus()?.status === "downloading" && `Загрузка Adoptium Java ${downloadStatus()?.major}...`}
                  {downloadStatus()?.status === "extracting" && `Распаковка и верификация SHA-256...`}
                  {downloadStatus()?.status === "ready" && `Java ${downloadStatus()?.major} готова к использованию!`}
                  {downloadStatus()?.status === "failed" && `Сбой загрузки: ${downloadStatus()?.error}`}
                </span>
              </div>
              <span class="font-mono text-zinc-400 font-semibold">
                {downloadStatus()?.percentage?.toFixed(0) || 0}%
              </span>
            </div>

            {/* Progress Bar */}
            <div class="w-full h-2 rounded-full bg-zinc-800 overflow-hidden">
              <div
                class="h-full bg-nord-cyan transition-all duration-300 ease-out"
                style={{ width: `${downloadStatus()?.percentage || 0}%` }}
              />
            </div>

            <div class="flex items-center justify-between text-[11px] font-mono text-zinc-500">
              <span>
                {((downloadStatus()?.bytes_read || 0) / (1024 * 1024)).toFixed(1)} MB /{" "}
                {((downloadStatus()?.total_bytes || 0) / (1024 * 1024)).toFixed(1)} MB
              </span>
              <span class="capitalize text-nord-cyan/80">
                Статус: {downloadStatus()?.status}
              </span>
            </div>
          </div>
        </Show>
      </div>

      {/* Add External Runtime */}
      <div class="p-5 rounded-2xl bg-nord-surface border border-white/10 shadow-lg space-y-3">
        <h2 class="text-sm font-bold text-white tracking-tight">
          Добавить внешнюю версию Java
        </h2>
        <p class="text-xs text-zinc-400">
          Укажите путь к каталогу JDK или исполняемому файлу (java.exe). Версия будет определена автоматически.
        </p>

        <div class="flex items-center gap-2">
          <input
            type="text"
            value={customPath()}
            onInput={(e) => setCustomPath(e.currentTarget.value)}
            placeholder="C:\Program Files\Java\jdk-21 или /usr/lib/jvm/java-21"
            class="flex-1 px-3.5 py-2 rounded-xl bg-zinc-900 border border-white/10 text-white font-mono text-xs focus:outline-none focus:border-nord-cyan"
            data-testid="add-java-path-input"
          />
          <button
            type="button"
            onClick={handleAddCustom}
            disabled={addingPath() || !customPath().trim()}
            class="px-4 py-2 rounded-xl bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark font-bold text-xs flex items-center gap-2 transition-all cursor-pointer disabled:opacity-50 shrink-0"
            data-testid="add-java-path-button"
          >
            <Show when={addingPath()} fallback={<Plus class="w-4 h-4" />}>
              <Loader2 class="w-4 h-4 animate-spin" />
            </Show>
            <span>Добавить</span>
          </button>
        </div>
      </div>

      {/* Installed Runtimes List */}
      <div class="p-5 rounded-2xl bg-nord-surface border border-white/10 shadow-lg space-y-4">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-sm font-bold text-white tracking-tight">
              Обнаруженные и управляемые рантаймы ({runtimes().length})
            </h2>
            <p class="text-xs text-zinc-400 mt-0.5">
              Управляемые рантаймы изолированы в директории Nord Launcher
            </p>
          </div>

          <Show when={unusedManagedCount() > 0}>
            <button
              type="button"
              onClick={handleCleanUnused}
              disabled={isCleaningUnused()}
              class="px-3 py-1.5 rounded-lg bg-nord-rose/10 hover:bg-nord-rose/20 text-nord-rose border border-nord-rose/20 text-xs font-mono font-medium flex items-center gap-1.5 transition-colors cursor-pointer disabled:opacity-50"
              data-testid="clean-unused-runtimes-button"
            >
              <Show when={isCleaningUnused()} fallback={<Trash2 class="w-3.5 h-3.5" />}>
                <Loader2 class="w-3.5 h-3.5 animate-spin" />
              </Show>
              <span>Очистить неиспользуемые ({unusedManagedCount()})</span>
            </button>
          </Show>
        </div>

        <Show
          when={runtimes().length > 0}
          fallback={
            <div class="p-8 text-center text-zinc-500 font-mono text-xs">
              Рантаймы не найдены. Установите Adoptium JDK выше.
            </div>
          }
        >
          <div class="space-y-2.5" data-testid="java-runtimes-list">
            <For each={runtimes()}>
              {(rt) => {
                const updateInfo = () =>
                  updates().find((u) => u.major_version === rt.major_version && u.update_available);
                const isUnused = () =>
                  rt.kind === "managed" && (!rt.used_by || rt.used_by.length === 0);

                return (
                  <div class="p-3.5 rounded-xl bg-zinc-900/70 border border-white/5 flex items-center justify-between gap-4">
                    <div class="flex items-center gap-3 min-w-0">
                      <div class="w-10 h-10 rounded-lg bg-black/40 border border-white/10 flex flex-col items-center justify-center shrink-0">
                        <span class="text-[9px] font-mono text-zinc-500 uppercase">Java</span>
                        <span class="text-sm font-bold text-white font-mono leading-none">
                          {rt.major_version || "?"}
                        </span>
                      </div>

                      <div class="min-w-0">
                        <div class="flex items-center gap-2 flex-wrap">
                          <span class="font-bold text-white text-xs">
                            {rt.vendor || "OpenJDK"} {rt.full_version || ""}
                          </span>
                          <span
                            class={`px-2 py-0.5 rounded text-[10px] font-mono font-semibold uppercase ${
                              rt.kind === "managed"
                                ? "bg-nord-cyan/15 text-nord-cyan border border-nord-cyan/30"
                                : "bg-white/10 text-zinc-300 border border-white/10"
                            }`}
                          >
                            {rt.kind === "managed" ? "Managed" : "Detected"}
                          </span>

                          <Show when={isUnused()}>
                            <span
                              class="px-2 py-0.5 rounded text-[10px] font-mono font-semibold bg-zinc-800 text-zinc-400 border border-white/5"
                              data-testid={`java-unused-badge-${rt.major_version}`}
                            >
                              Не используется
                            </span>
                          </Show>

                          <Show when={rt.kind === "managed" && updateInfo()}>
                            <span
                              class="px-2 py-0.5 rounded text-[10px] font-mono font-semibold bg-amber-500/15 text-amber-400 border border-amber-500/30"
                              data-testid={`java-update-badge-${rt.major_version}`}
                            >
                              Доступна {updateInfo()?.latest_version}
                            </span>
                          </Show>
                        </div>
                        <p class="font-mono text-[11px] text-zinc-500 truncate mt-0.5">
                          {rt.path}
                        </p>

                        <Show when={rt.used_by && rt.used_by.length > 0}>
                          <div class="flex items-center gap-1.5 mt-1.5">
                            <span class="text-[10px] text-zinc-500 font-mono">Используется:</span>
                            <For each={rt.used_by}>
                              {(name) => (
                                <span class="px-1.5 py-0.2 rounded bg-nord-emerald/10 text-nord-emerald border border-nord-emerald/20 text-[10px] font-mono">
                                  {name}
                                </span>
                              )}
                            </For>
                          </div>
                        </Show>
                      </div>
                    </div>

                    {/* Actions */}
                    <div class="flex items-center gap-2 shrink-0">
                      <Show when={rt.kind === "managed" && updateInfo()}>
                        <button
                          type="button"
                          onClick={() => handleUpgrade(rt.major_version)}
                          disabled={upgradingMajor() === rt.major_version}
                          class="px-2.5 py-1.5 rounded-lg bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark text-xs font-bold flex items-center gap-1.5 transition-colors cursor-pointer disabled:opacity-50"
                          title={`Обновить до ${updateInfo()?.latest_version}`}
                          data-testid={`java-upgrade-button-${rt.major_version}`}
                        >
                          <Show when={upgradingMajor() === rt.major_version} fallback={<ArrowUpCircle class="w-3.5 h-3.5" />}>
                            <Loader2 class="w-3.5 h-3.5 animate-spin" />
                          </Show>
                          <span>Обновить</span>
                        </button>
                      </Show>

                      <Show when={rt.kind === "managed"}>
                        <button
                          type="button"
                          onClick={() => handleRemove(rt)}
                          disabled={rt.used_by && rt.used_by.length > 0}
                          class="p-2 rounded-lg text-zinc-400 hover:text-nord-rose hover:bg-nord-rose/10 transition-colors cursor-pointer disabled:opacity-30 disabled:cursor-not-allowed"
                          title={
                            rt.used_by && rt.used_by.length > 0
                              ? `Используется сборками: ${rt.used_by.join(", ")}`
                              : "Удалить управляемый рантайм"
                          }
                          data-testid={`delete-runtime-${rt.major_version}`}
                        >
                          <Trash2 class="w-4 h-4" />
                        </button>
                      </Show>
                    </div>
                  </div>
                );
              }}
            </For>
          </div>
        </Show>
      </div>
    </div>
  );
};
