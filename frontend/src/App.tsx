import { Component, createSignal, onCleanup, onMount, Show } from "solid-js";
import { LayoutGrid, Layers, Package, User, Settings, Terminal, Cpu, AlertTriangle } from "lucide-solid";
import { LaunchButton, LaunchButtonState } from "./components/common/LaunchButton";
import { ModSearchInput } from "./components/common/ModSearchInput";
import { InstanceCard } from "./components/instance/InstanceCard";
import { ModCatalog } from "./components/mods/ModCatalog";
import { InstalledModsManager } from "./components/mods/InstalledModsManager";
import { AccountManager } from "./components/accounts/AccountManager";
import { CrashModal } from "./components/console/CrashModal";
import { UpdatePanel, formatVersion } from "./components/updater/UpdatePanel";
import { CurseForgeKeyCard } from "./components/settings/CurseForgeKeyCard";
import { launcherAPI } from "./services/api";
import type { InstanceDTO, CrashReportDTO, UpdateInfoDTO } from "./bindings/ipc_types";

type NavTab = "instances" | "mods_catalog" | "mods_manager" | "accounts" | "settings";

export const App: Component = () => {
  // Navigation
  const [currentNav, setCurrentNav] = createSignal<NavTab>("instances");

  // Instances state
  const [instances, setInstances] = createSignal<InstanceDTO[]>([
    {
      id: "nord-opti-1",
      name: "Nordic Optimized 1.21",
      game_version: "1.21.1",
      loader: "fabric",
      loader_version: "0.16.5",
      state: "idle",
      total_play_seconds: 7320,
    },
    {
      id: "nord-vanilla-2",
      name: "Vanilla Survival",
      game_version: "1.20.6",
      loader: "vanilla",
      state: "idle",
      total_play_seconds: 14200,
    },
  ]);

  const [selectedInstanceId, setSelectedInstanceId] = createSignal("nord-opti-1");
  const [launchState, setLaunchState] = createSignal<LaunchButtonState>("default");
  const [launchError, setLaunchError] = createSignal<string | undefined>();
  const [searchQuery, setSearchQuery] = createSignal("");
  const [crashReport, setCrashReport] = createSignal<CrashReportDTO | null>(null);
  const [availableUpdate, setAvailableUpdate] = createSignal<UpdateInfoDTO | null>(null);

  // 100-tick fine-grained progress benchmark
  const [tickProgress, setTickProgress] = createSignal(0);
  const [tickCount, setTickCount] = createSignal(0);
  const [renderCount] = createSignal(1);

  let timer: ReturnType<typeof setInterval>;
  let updateTimer: ReturnType<typeof setTimeout>;

  onMount(() => {
    timer = setInterval(() => {
      setTickProgress((prev) => (prev >= 100 ? 0 : prev + 1));
      setTickCount((prev) => prev + 1);
    }, 10);

    // Quiet background update check 3 seconds after mounting (Decision D1)
    updateTimer = setTimeout(async () => {
      try {
        const info = await launcherAPI.checkForUpdates();
        if (info.has_update) {
          setAvailableUpdate(info);
        }
      } catch (_ignored: unknown) {
        // Quiet failure for background check (Decision D1)
      }
    }, 3000);
  });

  onCleanup(() => {
    clearInterval(timer);
    clearTimeout(updateTimer);
  });

  const activeInstance = () =>
    instances().find((i) => i.id === selectedInstanceId()) || instances()[0];

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

  const handleLaunch = () => {
    setLaunchState("loading");
    setLaunchError(undefined);

    setTimeout(() => {
      setLaunchState("success");
      setInstances((prev) =>
        prev.map((inst) =>
          inst.id === selectedInstanceId() ? { ...inst, state: "running" } : inst
        )
      );

      setTimeout(() => {
        setLaunchState("default");
      }, 2500);
    }, 1000);
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
              onClick={() => setCurrentNav("mods_catalog")}
              class={`w-10 h-10 rounded-lg flex items-center justify-center transition-all ${
                currentNav() === "mods_catalog"
                  ? "bg-nord-cyan text-nord-dark shadow-[0_0_10px_rgba(0,212,178,0.2)] font-semibold"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Каталог модов (Modrinth & CurseForge)"
              data-testid="nav-mods-catalog"
            >
              <Layers class="w-5 h-5" />
            </button>

            <button
              type="button"
              onClick={() => setCurrentNav("mods_manager")}
              class={`w-10 h-10 rounded-lg flex items-center justify-center transition-all ${
                currentNav() === "mods_manager"
                  ? "bg-nord-cyan text-nord-dark shadow-[0_0_10px_rgba(0,212,178,0.2)] font-semibold"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Управление модами инстанса"
              data-testid="nav-mods-manager"
            >
              <Package class="w-5 h-5" />
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
              <Show when={availableUpdate()?.has_update}>
                <span
                  class="absolute top-1.5 right-1.5 w-2 h-2 rounded-full bg-nord-cyan ring-2 ring-nord-surface"
                  data-testid="nav-settings-update-dot"
                />
              </Show>
            </button>
          </nav>
        </div>

        {/* Bottom crash simulator trigger for demonstration */}
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
          </div>

          {/* Spike Metrics Indicator */}
          <div class="flex items-center gap-4 text-xs font-mono text-zinc-400">
            <div class="flex items-center gap-2 px-2.5 py-1 rounded bg-zinc-900 border border-white/5">
              <Cpu class="w-3.5 h-3.5 text-nord-cyan" />
              <span>DOM Renders: <strong class="text-zinc-200">{renderCount()}</strong></span>
              <span class="text-zinc-600">|</span>
              <span>Ticks: <strong class="text-nord-cyan">{tickCount()}</strong></span>
            </div>

            <div class="flex items-center gap-1.5 px-2 py-0.5 rounded bg-nord-emerald/10 text-nord-emerald border border-nord-emerald/20 text-[11px]">
              <span class="w-1.5 h-1.5 rounded-full bg-nord-emerald" />
              SolidJS Fine-Grained
            </div>
          </div>
        </header>

        {/* Content Area */}
        <div class="flex-1 p-6 overflow-y-auto">
          {/* VIEW 1: Instances Cockpit & Drawer */}
          <Show when={currentNav() === "instances"}>
            <div class="flex gap-6 h-full">
              {/* Active Instance Cockpit */}
              <section class="flex-1 flex flex-col justify-between p-6 rounded-2xl bg-nord-surface border border-white/10 shadow-xl relative overflow-hidden">
                <div>
                  <div class="flex items-center justify-between">
                    <span class="px-2 py-0.5 rounded text-[11px] font-mono font-medium uppercase tracking-wider bg-white/5 text-zinc-400 border border-white/5">
                      Активная сборка
                    </span>
                    <span class="font-mono text-xs text-zinc-500">
                      ID: {activeInstance().id}
                    </span>
                  </div>

                  <h1 class="text-2xl font-bold text-white tracking-tight mt-3">
                    {activeInstance().name}
                  </h1>

                  <div class="flex items-center gap-2.5 mt-3">
                    <span class="px-2.5 py-1 rounded-md text-xs font-mono font-semibold bg-zinc-800 text-zinc-200 border border-white/10">
                      Minecraft {activeInstance().game_version}
                    </span>
                    <span class="px-2.5 py-1 rounded-md text-xs font-mono font-medium capitalize bg-nord-cyan/10 text-nord-cyan border border-nord-cyan/20">
                      {activeInstance().loader} {activeInstance().loader_version || ""}
                    </span>
                  </div>
                </div>

                {/* High-frequency Stream Progress Indicator */}
                <div class="my-6 p-4 rounded-xl bg-zinc-950/60 border border-white/5">
                  <div class="flex items-center justify-between text-xs font-mono text-zinc-400 mb-2">
                    <span class="flex items-center gap-1.5">
                      <Terminal class="w-3.5 h-3.5 text-zinc-500" />
                      Синхронизация ассетов (100 тиков/с):
                    </span>
                    <span class="text-nord-cyan font-bold" data-testid="tick-progress-text">
                      {tickProgress()}%
                    </span>
                  </div>
                  <div class="h-1.5 w-full bg-zinc-800 rounded-full overflow-hidden">
                    <div
                      class="h-full bg-nord-cyan transition-all duration-75 ease-out rounded-full"
                      style={{ width: `${tickProgress()}%` }}
                      data-testid="tick-progress-bar"
                    />
                  </div>
                </div>

                {/* Launch Actions */}
                <div class="pt-4 border-t border-white/5 flex items-center justify-between">
                  <div class="flex flex-col text-xs text-zinc-400">
                    <span>Память JVM: <strong class="text-zinc-200 font-mono">2048 - 4096 МБ</strong></span>
                    <span class="text-zinc-500 text-[11px] mt-0.5">Adoptium OpenJDK 21 x64 (Clean environment)</span>
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

          {/* VIEW 2: Mod Catalog (Modrinth / CurseForge) */}
          <Show when={currentNav() === "mods_catalog"}>
            <div class="max-w-4xl mx-auto">
              <ModCatalog
                activeInstanceId={activeInstance().id}
                gameVersion={activeInstance().game_version}
                loader={activeInstance().loader}
              />
            </div>
          </Show>

          {/* VIEW 3: Installed Mods Manager */}
          <Show when={currentNav() === "mods_manager"}>
            <div class="max-w-4xl mx-auto">
              <InstalledModsManager instanceId={activeInstance().id} />
            </div>
          </Show>

          {/* VIEW 4: Accounts Management */}
          <Show when={currentNav() === "accounts"}>
            <div class="max-w-3xl mx-auto">
              <AccountManager />
            </div>
          </Show>

          {/* VIEW 5: Settings & Updates */}
          <Show when={currentNav() === "settings"}>
            <div class="max-w-3xl mx-auto space-y-6">
              <CurseForgeKeyCard />
              <UpdatePanel
                channel="stable"
                onUpdateAvailable={(info) => setAvailableUpdate(info)}
              />
            </div>
          </Show>
        </div>
      </main>
    </div>
  );
};