import { Component, createSignal, createEffect, onCleanup, onMount, Show, For } from "solid-js";
import { LayoutGrid, Package, User, Settings, AlertTriangle, AlertCircle, Cpu, Sliders, Clock, Terminal, Activity, Download, Upload } from "lucide-solid";
import { LaunchButton, LaunchButtonState } from "./components/common/LaunchButton";
import { ModSearchInput } from "./components/common/ModSearchInput";
import { InstanceCard } from "./components/instance/InstanceCard";
import { InstanceSettingsModal, SettingsTab } from "./components/instance/InstanceSettingsModal";
import { MrPackImportModal } from "./components/instance/MrPackImportModal";
import { MrPackExportModal } from "./components/instance/MrPackExportModal";
import { JavaManager } from "./components/java/JavaManager";
import { AccountManager } from "./components/accounts/AccountManager";
import { CrashModal } from "./components/console/CrashModal";
import { UpdatePanel, formatVersion } from "./components/updater/UpdatePanel";
import { StartupUpdateModal } from "./components/updater/StartupUpdateModal";
import { CurseForgeKeyCard } from "./components/settings/CurseForgeKeyCard";
import { launcherAPI } from "./services/api";
import type { InstanceDTO, CrashReportDTO, UpdateInfoDTO } from "./bindings/ipc_types";

export type UpdateBadgeState =
  | "available"
  | "downloading"
  | "ready-to-restart"
  | "error"
  | "up-to-date"
  | "snoozed-visible"
  | "unknown";

type NavTab = "instances" | "accounts" | "settings" | "java_manager";

export const App: Component = () => {
  // Navigation
  const [currentNav, setCurrentNav] = createSignal<NavTab>("instances");

  // Instances state initialized empty and populated via launcherAPI.listInstances() (B2)
  const [instances, setInstances] = createSignal<InstanceDTO[]>([]);
  const [selectedInstanceId, setSelectedInstanceId] = createSignal("");
  const [launchState, setLaunchState] = createSignal<LaunchButtonState>("default");
  const [launchError, setLaunchError] = createSignal<string | undefined>();
  const [searchQuery, setSearchQuery] = createSignal("");
  const [crashReport, setCrashReport] = createSignal<CrashReportDTO | null>(null);
  const [availableUpdate, setAvailableUpdate] = createSignal<UpdateInfoDTO | null>(null);
  const [badgeState, setBadgeState] = createSignal<UpdateBadgeState>("unknown");
  const [isStartupModalOpen, setIsStartupModalOpen] = createSignal(false);
  const [isApplyingUpdate, setIsApplyingUpdate] = createSignal(false);
  const [systemError, setSystemError] = createSignal<string>("");
  const [isSettingsOpen, setIsSettingsOpen] = createSignal(false);
  const [settingsInitialTab, setSettingsInitialTab] = createSignal<SettingsTab>("general");
  const [isImportModalOpen, setIsImportModalOpen] = createSignal(false);
  const [isExportModalOpen, setIsExportModalOpen] = createSignal(false);

  const openSettingsWithTab = (tab: SettingsTab) => {
    setSettingsInitialTab(tab);
    setIsSettingsOpen(true);
  };
  const [installedModsCount, setInstalledModsCount] = createSignal(0);
  const [logTail, setLogTail] = createSignal<string[]>([]);

  let pollInterval: ReturnType<typeof setInterval> | null = null;
  let updateTimer: ReturnType<typeof setTimeout> | null = null;
  let periodicUpdateTimer: ReturnType<typeof setInterval> | null = null;

  const performUpdateCheck = async (isStartup = false) => {
    try {
      const info = await launcherAPI.checkForUpdates();
      if (info.has_update) {
        setAvailableUpdate(info);
        const snoozeStr = localStorage.getItem("nord_update_snooze");
        const snoozedUntil = snoozeStr ? parseInt(snoozeStr, 10) : 0;
        const isSnoozed = Date.now() < snoozedUntil;

        if (isSnoozed) {
          setBadgeState("snoozed-visible");
          setIsStartupModalOpen(false);
        } else {
          setBadgeState("available");
          if (isStartup) {
            setIsStartupModalOpen(true);
          }
        }
      } else {
        setBadgeState("up-to-date");
      }
    } catch (_err: unknown) {
      if (isStartup) {
        // Offline / error on startup: unknown state, NO badge! (B6)
        setBadgeState("unknown");
      } else {
        setBadgeState("error");
      }
    }
  };

  const handleInstallAndRestart = async () => {
    setBadgeState("downloading");
    setIsApplyingUpdate(true);
    try {
      const res = await launcherAPI.applyUpdate();
      if (res.success) {
        setBadgeState("ready-to-restart");
        await launcherAPI.restartApplication();
      } else {
        setBadgeState("error");
        setIsApplyingUpdate(false);
      }
    } catch (_err: unknown) {
      setBadgeState("error");
      setIsApplyingUpdate(false);
    }
  };

  const handleSnooze = () => {
    localStorage.setItem("nord_update_snooze", String(Date.now() + 24 * 60 * 60 * 1000));
    setIsStartupModalOpen(false);
    setBadgeState("snoozed-visible");
  };

  const handleCloseStartupModal = () => {
    handleSnooze();
  };

  const stopStatePolling = () => {
    if (pollInterval) {
      clearInterval(pollInterval);
      pollInterval = null;
    }
  };

  // State polling with race protection (D2 & M1)
  const startStatePolling = (targetInstanceId: string) => {
    stopStatePolling();
    let hasObservedRunning = false;
    let consecutiveIdleCount = 0;

    pollInterval = setInterval(async () => {
      try {
        const list = await launcherAPI.listInstances();
        setInstances(list);

        const current = list.find((i) => i.id === targetInstanceId);
        if (!current) {
          stopStatePolling();
          setLaunchState("default");
          return;
        }

        if (current.state === "running") {
          hasObservedRunning = true;
          consecutiveIdleCount = 0;
          setLaunchState("success");
          try {
            const tail = await launcherAPI.getLogTail(targetInstanceId, 100);
            setLogTail(tail);
          } catch (_err: unknown) {
            // non-fatal log tail fetch
          }
        } else if (current.state === "crashed") {
          stopStatePolling();
          setLaunchState("error");
          setLaunchError("Игра аварийно завершилась");
          try {
            const crash = await launcherAPI.getLastCrashReport(targetInstanceId);
            if (crash) {
              setCrashReport(crash);
            }
          } catch (_e: unknown) {
            // non-fatal crash report fetch
          }
        } else if (current.state === "idle") {
          consecutiveIdleCount++;
          // D2: Exit only after having observed running once, OR after >= 2 consecutive idle samples
          if (hasObservedRunning || consecutiveIdleCount >= 2) {
            stopStatePolling();
            setLaunchState("default");
          }
        }
      } catch (err: unknown) {
        console.error("Polling error:", err);
        const msg = err instanceof Error ? err.message : String(err);
        setSystemError(`Ошибка мониторинга состояния игры: ${msg}`);
      }
    }, import.meta.env.MODE === "test" ? 50 : 1000);
  };

  onMount(() => {
    const loadInstances = async () => {
      try {
        const list = await launcherAPI.listInstances();
        setInstances(list);
        if (list.length > 0 && !list.some((i) => i.id === selectedInstanceId())) {
          setSelectedInstanceId(list[0].id);
        }
      } catch (err: unknown) {
        console.error("Failed to list instances:", err);
        const msg = err instanceof Error ? err.message : String(err);
        setSystemError(`Не удалось загрузить список сборок: ${msg}`);
      }
    };
    loadInstances();

    // Startup update check with 3s delay (or 50ms in test mode)
    const delay = import.meta.env.MODE === "test" ? 50 : 3000;
    updateTimer = setTimeout(() => {
      performUpdateCheck(true);
    }, delay);

    // Periodic check every 10 minutes (600s)
    const periodicDelay = import.meta.env.MODE === "test" ? 10000 : 10 * 60 * 1000;
    periodicUpdateTimer = setInterval(() => {
      performUpdateCheck(false);
    }, periodicDelay);
  });

  onCleanup(() => {
    stopStatePolling();
    if (updateTimer) clearTimeout(updateTimer);
    if (periodicUpdateTimer) clearInterval(periodicUpdateTimer);
  });

  const activeInstance = () => {
    const list = instances();
    if (list.length === 0) {
      return {
        id: "",
        name: "Сборка не выбрана",
        game_version: "1.21.1",
        loader: "fabric",
        loader_version: "",
        state: "idle",
        total_play_seconds: 0,
      } as InstanceDTO;
    }
    return list.find((i) => i.id === selectedInstanceId()) || list[0];
  };

  const filteredInstances = () => {
    const q = searchQuery().toLowerCase().trim();
    if (!q) return instances();
    return instances().filter(
      (i) =>
        i.name.toLowerCase().includes(q) ||
        i.game_version.toLowerCase().includes(q) ||
        i.loader.toLowerCase().includes(q)
    );
  };

  const [customJavaPath, setCustomJavaPath] = createSignal("");

  const loadModsCount = async (instId: string) => {
    if (!instId) return;
    try {
      const mods = await launcherAPI.listInstalledMods(instId);
      setInstalledModsCount(mods.length);
    } catch (_err: unknown) {
      // non-fatal
    }
  };

  const formatPlaytime = (seconds: number): string => {
    if (!seconds || seconds <= 0) return "0 ч";
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (hours > 0) {
      return `${hours} ч ${minutes} мин`;
    }
    return `${minutes} мин`;
  };

  createEffect(() => {
    const inst = activeInstance();
    setCustomJavaPath(inst.java_path || "");
    if (inst.id) {
      loadModsCount(inst.id);
      launcherAPI
        .getLogTail(inst.id, 100)
        .then((tail) => setLogTail(tail))
        .catch(() => {});
    }
  });

  const saveInstanceJavaPath = async () => {
    const targetId = activeInstance().id;
    if (!targetId) return;
    try {
      const updated = await launcherAPI.updateInstance({
        id: targetId,
        java_path: customJavaPath().trim(),
      });
      setInstances((prev) => prev.map((i) => (i.id === updated.id ? updated : i)));
    } catch (err: unknown) {
      console.error("Failed to update instance Java path:", err);
      const msg = err instanceof Error ? err.message : String(err);
      setSystemError(`Ошибка обновления параметров инстанса: ${msg}`);
    }
  };

  const handleLaunch = async () => {
    const inst = activeInstance();
    if (!inst || !inst.id) return;

    setLaunchState("loading");
    setLaunchError(undefined);

    try {
      const res = await launcherAPI.launchInstance(inst.id);
      if (!res.success) {
        setLaunchState("error");
        setLaunchError(res.error || "Failed to launch instance");
        return;
      }

      setLaunchState("success");
      setInstances((prev) =>
        prev.map((i) => (i.id === inst.id ? { ...i, state: "running" } : i))
      );

      startStatePolling(inst.id);
    } catch (err: unknown) {
      setLaunchState("error");
      const msg = err instanceof Error ? err.message : String(err);
      setLaunchError(msg || "Launch failed");
    }
  };

  const triggerMockCrash = () => {
    setCrashReport({
      category: "out_of_memory",
      summary: "Игра аварийно завершилась из-за нехватки оперативной памяти (Java Heap Space)",
      remedy: "Увеличьте объем выделенной памяти в настройках инстанса с 2048 МБ до 4096 МБ или 6144 МБ.",
      details: "java.lang.OutOfMemoryError: Java heap space",
      relevant_lines: [
        "[00:15:30] [main/INFO]: Starting Minecraft 1.21.1...",
        "[00:15:32] [Render thread/INFO]: Loading 142 mods...",
        "[00:15:40] [Render thread/ERROR]: java.lang.OutOfMemoryError: Java heap space",
        "[00:15:40] [Render thread/ERROR]: Failed to allocate 10485760 bytes",
      ],
      exit_code: 1,
    });
  };

  return (
    <div class="flex h-screen w-screen bg-nord-dark text-zinc-100 font-sans overflow-hidden">
      {/* Crash Modal */}
      <CrashModal report={crashReport()} onClose={() => setCrashReport(null)} />

      {/* Startup Update Modal (F3, B6) */}
      <StartupUpdateModal
        isOpen={isStartupModalOpen()}
        updateInfo={availableUpdate()}
        isApplying={isApplyingUpdate()}
        onInstallAndRestart={handleInstallAndRestart}
        onSnooze={handleSnooze}
        onClose={handleCloseStartupModal}
      />

      {/* Side-Rail Navigation (Hallmark N3) */}
      <aside class="w-16 flex flex-col items-center justify-between py-4 bg-nord-surface border-r border-white/5 select-none z-20">
        <div class="flex flex-col items-center gap-6">
          <div class="w-10 h-10 rounded-xl bg-nord-card border border-nord-cyan/30 flex items-center justify-center text-nord-cyan shadow-[0_0_12px_rgba(0,212,178,0.2)]">
            <span class="font-bold font-mono text-base tracking-tighter">N</span>
          </div>

          <nav class="flex flex-col items-center gap-2">
            <button
              type="button"
              onClick={() => setCurrentNav("instances")}
              class={`w-10 h-10 rounded-lg flex items-center justify-center transition-all ${
                currentNav() === "instances"
                  ? "bg-nord-cyan text-nord-dark shadow-[0_0_10px_rgba(0,212,178,0.2)] font-semibold"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Сборки"
              data-testid="nav-instances"
            >
              <LayoutGrid class="w-5 h-5" />
            </button>

            <button
              type="button"
              onClick={() => setCurrentNav("java_manager")}
              class={`w-10 h-10 rounded-lg flex items-center justify-center transition-all ${
                currentNav() === "java_manager"
                  ? "bg-nord-cyan text-nord-dark shadow-[0_0_10px_rgba(0,212,178,0.2)] font-semibold"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Менеджер Java (Java Runtimes)"
              data-testid="nav-java-manager"
            >
              <Cpu class="w-5 h-5" />
            </button>

            <button
              type="button"
              onClick={() => setCurrentNav("accounts")}
              class={`w-10 h-10 rounded-lg flex items-center justify-center transition-all ${
                currentNav() === "accounts"
                  ? "bg-nord-cyan text-nord-dark shadow-[0_0_10px_rgba(0,212,178,0.2)] font-semibold"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Accounts"
              data-testid="nav-accounts"
            >
              <User class="w-5 h-5" />
            </button>

            <button
              type="button"
              onClick={() => setCurrentNav("settings")}
              class={`w-10 h-10 rounded-lg flex items-center justify-center transition-all relative ${
                currentNav() === "settings"
                  ? "bg-nord-cyan text-nord-dark shadow-[0_0_10px_rgba(0,212,178,0.2)] font-semibold"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Settings & Updates"
              data-testid="nav-settings"
            >
              <Settings class="w-5 h-5" />
              <Show when={badgeState() === "available" || badgeState() === "snoozed-visible"}>
                <span
                  class="absolute top-1.5 right-1.5 w-2 h-2 rounded-full bg-nord-cyan ring-2 ring-nord-surface"
                  data-testid="nav-settings-update-dot"
                />
              </Show>
              <Show when={badgeState() === "downloading"}>
                <span
                  class="absolute top-1.5 right-1.5 w-2 h-2 rounded-full bg-nord-cyan animate-ping ring-2 ring-nord-surface"
                  data-testid="nav-settings-update-downloading-dot"
                />
              </Show>
              <Show when={badgeState() === "ready-to-restart"}>
                <span
                  class="absolute top-1.5 right-1.5 w-2 h-2 rounded-full bg-emerald-400 ring-2 ring-nord-surface"
                  data-testid="nav-settings-update-ready-dot"
                />
              </Show>
              <Show when={badgeState() === "error"}>
                <span
                  class="absolute -top-0.5 -right-0.5 w-3.5 h-3.5 rounded-full bg-amber-500 text-zinc-950 font-bold text-[9px] flex items-center justify-center ring-1 ring-nord-surface leading-none"
                  data-testid="nav-settings-update-error-dot"
                >
                  !
                </span>
              </Show>
            </button>
          </nav>
        </div>

        {/* Bottom crash simulator trigger for demonstration (H5: dev-gated) */}
        <Show when={import.meta.env.DEV}>
          <div class="flex flex-col items-center gap-3">
            <button
              type="button"
              onClick={triggerMockCrash}
              title="Тест диагностики сбоя (Crash Diagnostic Modal)"
              class="w-9 h-9 rounded-full bg-zinc-900 border border-red-500/30 text-red-400 hover:bg-red-500/10 flex items-center justify-center transition-colors"
            >
              <AlertTriangle class="w-4 h-4" />
            </button>
          </div>
        </Show>
      </aside>

      {/* Main Workspace (Workbench Layout) */}
      <main class="flex-1 flex flex-col min-w-0 bg-nord-dark overflow-hidden">
        {/* Top Command Bar */}
        <header class="h-14 px-6 flex items-center justify-between border-b border-white/5 bg-nord-surface/40 backdrop-blur-md z-10">
          <div class="flex items-center gap-4">
            <div class="w-80">
              <ModSearchInput
                value={searchQuery()}
                matchCount={filteredInstances().length}
                onSearch={setSearchQuery}
                onClear={() => setSearchQuery("")}
              />
            </div>

            <Show when={availableUpdate()?.has_update}>
              <button
                type="button"
                onClick={() => setCurrentNav("settings")}
                class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-nord-cyan/10 text-nord-cyan border border-nord-cyan/30 text-xs font-mono font-semibold hover:bg-nord-cyan/20 transition-all cursor-pointer select-none"
                data-testid="header-update-badge"
              >
                <span class="w-1.5 h-1.5 rounded-full bg-nord-cyan animate-pulse" />
                <span>Update {formatVersion(availableUpdate()?.version)} available</span>
              </button>
            </Show>

            <button
              type="button"
              onClick={() => setIsImportModalOpen(true)}
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-300 hover:text-white border border-white/10 text-xs font-medium transition-colors cursor-pointer"
              title="Импортировать сборку формата Modrinth .mrpack"
              data-testid="import-mrpack-btn"
            >
              <Download class="w-3.5 h-3.5 text-nord-cyan" />
              <span>Импорт .mrpack</span>
            </button>
          </div>

          {/* System Status Indicator */}
          <div class="flex items-center gap-4 text-xs font-mono text-zinc-400">
            <div class="flex items-center gap-1.5 px-2 py-0.5 rounded bg-nord-emerald/10 text-nord-emerald border border-nord-emerald/20 text-[11px]">
              <span class="w-1.5 h-1.5 rounded-full bg-nord-emerald" />
              SolidJS Fine-Grained
            </div>
          </div>
        </header>

        {/* System Error Banner (H6) */}
        <Show when={systemError()}>
          <div
            class="mx-6 mt-4 p-3 rounded-lg bg-nord-rose/10 border border-nord-rose/20 text-xs text-nord-rose flex items-center justify-between gap-3 font-mono"
            data-testid="system-error-banner"
          >
            <div class="flex items-center gap-2">
              <AlertCircle class="w-4 h-4 text-nord-rose shrink-0" />
              <span>{systemError()}</span>
            </div>
            <button
              type="button"
              onClick={() => setSystemError("")}
              class="text-[11px] text-zinc-400 hover:text-zinc-200 underline cursor-pointer"
            >
              Закрыть
            </button>
          </div>
        </Show>

        {/* Content Area */}
        <div class="flex-1 p-6 overflow-y-auto">
          {/* VIEW 1: Instances Cockpit & Drawer */}
          <Show when={currentNav() === "instances"}>
            <div class="flex gap-6 h-full">
              {/* Active Instance Cockpit */}
              <section class="flex-1 flex flex-col justify-between p-6 rounded-2xl bg-nord-surface border border-white/10 shadow-xl relative overflow-hidden">
                <div class="space-y-4">
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <span class="px-2 py-0.5 rounded text-[11px] font-mono font-medium uppercase tracking-wider bg-white/5 text-zinc-400 border border-white/5">
                        Активная сборка
                      </span>
                      <span class="font-mono text-xs text-zinc-500">
                        ID: {activeInstance().id}
                      </span>
                    </div>

                    <div class="flex items-center gap-2">
                      {/* Export MrPack Button */}
                      <button
                        type="button"
                        onClick={() => setIsExportModalOpen(true)}
                        class="px-3 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-300 hover:text-white border border-white/10 text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer"
                        title="Экспортировать сборку в .mrpack"
                        data-testid="export-mrpack-btn"
                      >
                        <Upload class="w-3.5 h-3.5 text-nord-cyan" />
                        <span>Экспорт .mrpack</span>
                      </button>

                      {/* Instance Settings Button */}
                      <button
                        type="button"
                        onClick={() => openSettingsWithTab("general")}
                        class="px-3 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-300 hover:text-white border border-white/10 text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer"
                        title="Настройки сборки"
                        data-testid="open-instance-settings-button"
                      >
                        <Sliders class="w-3.5 h-3.5 text-nord-cyan" />
                        <span>Настройки</span>
                      </button>
                    </div>
                  </div>

                  <div>
                    <h1 class="text-2xl font-bold text-white tracking-tight">
                      {activeInstance().name}
                    </h1>

                    <div class="flex items-center gap-2.5 mt-2">
                      <span class="px-2.5 py-1 rounded-md text-xs font-mono font-semibold bg-zinc-800 text-zinc-200 border border-white/10">
                        Minecraft {activeInstance().game_version}
                      </span>
                      <span class="px-2.5 py-1 rounded-md text-xs font-mono font-medium capitalize bg-nord-cyan/10 text-nord-cyan border border-nord-cyan/20">
                        {activeInstance().loader} {activeInstance().loader_version || ""}
                      </span>
                    </div>
                  </div>

                  {/* Cockpit Data Density Grid (J3) */}
                  <div class="grid grid-cols-4 gap-3 pt-1">
                    {/* RAM Badge */}
                    <div
                      class="p-3 rounded-xl bg-black/20 border border-white/5 space-y-1"
                      data-testid="cockpit-ram-badge"
                    >
                      <div class="flex items-center gap-1.5 text-zinc-500 text-[11px] font-mono">
                        <Activity class="w-3.5 h-3.5 text-nord-cyan" />
                        <span>Память JVM</span>
                      </div>
                      <p class="text-white font-mono font-bold text-xs truncate">
                        {activeInstance().min_ram_mb || 2048} - {activeInstance().max_ram_mb || 4096} MB
                      </p>
                    </div>

                    {/* Playtime Badge */}
                    <div
                      class="p-3 rounded-xl bg-black/20 border border-white/5 space-y-1"
                      data-testid="cockpit-playtime-badge"
                    >
                      <div class="flex items-center gap-1.5 text-zinc-500 text-[11px] font-mono">
                        <Clock class="w-3.5 h-3.5 text-nord-emerald" />
                        <span>Время в игре</span>
                      </div>
                      <p class="text-white font-mono font-bold text-xs">
                        {formatPlaytime(activeInstance().total_play_seconds)}
                      </p>
                    </div>

                    {/* Installed Mods Badge */}
                    <div
                      class="p-3 rounded-xl bg-black/20 hover:bg-white/5 border border-white/5 space-y-1 cursor-pointer transition-colors"
                      data-testid="cockpit-mods-count-badge"
                      onClick={() => openSettingsWithTab("mods")}
                      title="Нажмите для управления модами"
                    >
                      <div class="flex items-center gap-1.5 text-zinc-500 text-[11px] font-mono">
                        <Package class="w-3.5 h-3.5 text-nord-amber" />
                        <span>Моды</span>
                      </div>
                      <p class="text-white font-mono font-bold text-xs">
                        {installedModsCount()} модов
                      </p>
                    </div>

                    {/* Java Status Chip */}
                    <div
                      class="p-3 rounded-xl bg-black/20 border border-white/5 space-y-1 cursor-pointer hover:border-nord-cyan/30 transition-colors"
                      onClick={() => setCurrentNav("java_manager")}
                      title="Нажмите для открытия Java Manager"
                      data-testid="cockpit-java-chip"
                    >
                      <div class="flex items-center justify-between text-zinc-500 text-[11px] font-mono">
                        <div class="flex items-center gap-1.5">
                          <Cpu class="w-3.5 h-3.5 text-nord-cyan" />
                          <span>Java Рантайм</span>
                        </div>
                      </div>
                      <p class="text-nord-cyan font-mono font-bold text-xs truncate">
                        {activeInstance().java_path ? "Кастомный" : "Adoptium (Auto)"}
                      </p>
                    </div>
                  </div>

                  {/* Java Runtime Path Control (S3: preserved for backward compat & test coverage) */}
                  <div class="p-3.5 rounded-xl bg-black/20 border border-white/5 flex flex-col gap-2">
                    <div class="flex items-center justify-between">
                      <span class="text-xs font-semibold text-zinc-300">Среда выполнения Java (JavaPath)</span>
                      <span class="text-[11px] font-mono text-zinc-500">
                        {activeInstance().java_path ? "Пользовательский путь" : "Авто (Java 21 / 17 / 8)"}
                      </span>
                    </div>
                    <div class="flex items-center gap-2">
                      <input
                        type="text"
                        value={customJavaPath()}
                        placeholder="Авто-определение или путь к java.exe"
                        onInput={(e) => setCustomJavaPath(e.currentTarget.value)}
                        class="flex-1 px-3 py-1.5 rounded-lg bg-zinc-900 border border-white/10 text-xs font-mono text-zinc-200 placeholder:text-zinc-600 focus:outline-none focus:border-nord-cyan"
                        data-testid="instance-java-path-input"
                      />
                      <button
                        type="button"
                        onClick={saveInstanceJavaPath}
                        class="px-3 py-1.5 rounded-lg bg-white/10 hover:bg-white/15 text-xs font-medium text-white transition-colors cursor-pointer"
                        data-testid="save-java-path-button"
                      >
                        Сохранить
                      </button>
                    </div>
                    <p class="text-[11px] text-zinc-500">
                      При запуске выполняется строгая проверка соответствия мажорной версии Java (fail-closed).
                    </p>
                  </div>

                  {/* Live Console Log Tail (J3) */}
                  <div
                    class="p-3.5 rounded-xl bg-black/30 border border-white/5 flex flex-col gap-2"
                    data-testid="cockpit-log-tail"
                  >
                    <div class="flex items-center justify-between">
                      <div class="flex items-center gap-2">
                        <Terminal class="w-3.5 h-3.5 text-nord-cyan" />
                        <span class="text-xs font-semibold text-zinc-300">
                          Консоль процесса игры (Log Tail)
                        </span>
                        <Show when={activeInstance().state === "running"}>
                          <span class="w-2 h-2 rounded-full bg-nord-emerald animate-pulse" />
                        </Show>
                      </div>
                      <span class="text-[10px] font-mono text-zinc-500">
                        {logTail().length > 0 ? `${logTail().length} строк` : "Ожидание запуска"}
                      </span>
                    </div>

                    <div class="h-24 overflow-y-auto bg-black/60 rounded-lg p-2 font-mono text-[11px] text-zinc-400 select-text leading-relaxed border border-white/5">
                      <Show
                        when={logTail().length > 0}
                        fallback={
                          <div class="text-zinc-600 italic">
                            {activeInstance().state === "running"
                              ? "Загрузка логов процесса..."
                              : "Консоль ожидает запуска игры. Логи будут отображаться здесь в реальном времени."}
                          </div>
                        }
                      >
                        <For each={logTail()}>
                          {(line) => (
                            <div class="truncate text-zinc-300 font-mono py-0.5">
                              {line}
                            </div>
                          )}
                        </For>
                      </Show>
                    </div>
                  </div>
                </div>

                {/* Launch Actions */}
                <div class="pt-4 mt-4 border-t border-white/5 flex items-center justify-between">
                  <div class="flex flex-col text-xs text-zinc-400">
                    <span>
                      Память JVM:{" "}
                      <strong class="text-zinc-200 font-mono">
                        {activeInstance().min_ram_mb || 2048} - {activeInstance().max_ram_mb || 4096} МБ
                      </strong>
                    </span>
                    <span class="text-zinc-500 text-[11px] mt-0.5">
                      {activeInstance().java_path ? "Пользовательский JVM" : "Adoptium OpenJDK (Изолированная среда)"}
                    </span>
                  </div>

                  <LaunchButton
                    state={launchState()}
                    errorMessage={launchError()}
                    onClick={handleLaunch}
                    onRetry={handleLaunch}
                  />
                </div>
              </section>

              {/* Instances Drawer */}
              <section class="w-84 flex flex-col gap-3">
                <div class="flex items-center justify-between text-xs text-zinc-400 px-1 font-medium">
                  <span>Сборки ({filteredInstances().length})</span>
                  <span class="text-[11px] text-zinc-500 font-mono">Nord Engine</span>
                </div>

                <div class="flex flex-col gap-2.5">
                  {filteredInstances().map((inst) => (
                    <InstanceCard
                      instance={inst}
                      selected={inst.id === selectedInstanceId()}
                      onSelect={(i) => setSelectedInstanceId(i.id)}
                      onLaunch={() => {
                        setSelectedInstanceId(inst.id);
                        handleLaunch();
                      }}
                    />
                  ))}
                </div>
              </section>
            </div>
          </Show>

          {/* VIEW 2: Accounts Management */}
          <Show when={currentNav() === "accounts"}>
            <div class="max-w-3xl mx-auto">
              <AccountManager />
            </div>
          </Show>

          {/* VIEW 3: Settings & Updates */}
          <Show when={currentNav() === "settings"}>
            <div class="max-w-3xl mx-auto space-y-6">
              <CurseForgeKeyCard />
              <UpdatePanel
                channel="stable"
                onUpdateAvailable={(info) => setAvailableUpdate(info)}
              />
            </div>
          </Show>

          {/* VIEW 4: Java Runtime Manager (J1) */}
          <Show when={currentNav() === "java_manager"}>
            <div class="max-w-4xl mx-auto">
              <JavaManager onClose={() => setCurrentNav("instances")} />
            </div>
          </Show>
        </div>
      </main>

      {/* Instance Settings Modal (J2, C5) */}
      <InstanceSettingsModal
        instance={activeInstance()}
        isOpen={isSettingsOpen()}
        initialTab={settingsInitialTab()}
        onClose={() => {
          setIsSettingsOpen(false);
          const currentId = activeInstance().id;
          if (currentId) {
            loadModsCount(currentId);
          }
        }}
        onSaved={(updated) => {
          setInstances((prev) =>
            prev.map((i) => (i.id === updated.id ? updated : i))
          );
        }}
        onOpenJavaManager={() => setCurrentNav("java_manager")}
      />

      {/* MrPack Import Modal (v0.6.0) */}
      <MrPackImportModal
        isOpen={isImportModalOpen()}
        onClose={() => setIsImportModalOpen(false)}
        onImported={async (newInst) => {
          setIsImportModalOpen(false);
          try {
            const list = await launcherAPI.listInstances();
            setInstances(list);
            setSelectedInstanceId(newInst.id);
          } catch (_err) {
            // non-fatal
          }
        }}
      />

      {/* MrPack Export Modal (v0.6.0) */}
      <MrPackExportModal
        instance={activeInstance()}
        isOpen={isExportModalOpen()}
        onClose={() => setIsExportModalOpen(false)}
      />
    </div>
  );
};