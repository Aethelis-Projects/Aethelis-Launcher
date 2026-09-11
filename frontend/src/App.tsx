import { Component, createSignal, onCleanup, onMount } from "solid-js";
import { LayoutGrid, Download, Settings, User, Terminal, Cpu } from "lucide-solid";
import { LaunchButton, LaunchButtonState } from "./components/common/LaunchButton";
import { ModSearchInput } from "./components/common/ModSearchInput";
import { InstanceCard } from "./components/instance/InstanceCard";
import { InstanceDTO } from "./bindings/ipc_types";

export const App: Component = () => {
  // Navigation
  const [currentNav, setCurrentNav] = createSignal("instances");

  // Instances list state
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

  // Fine-grained 100-tick progress simulation for M0 Spike benchmark
  const [tickProgress, setTickProgress] = createSignal(0);
  const [tickCount, setTickCount] = createSignal(0);
  const [renderCount] = createSignal(1); // SolidJS components render ONCE!

  let timer: any;
  onMount(() => {
    // 100 ticks per second (every 10ms) to test fine-grained reactive updates
    timer = setInterval(() => {
      setTickProgress((prev) => (prev >= 100 ? 0 : prev + 1));
      setTickCount((prev) => prev + 1);
    }, 10);
  });

  onCleanup(() => {
    clearInterval(timer);
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
      // Simulate launch success
      setLaunchState("success");
      setInstances((prev) =>
        prev.map((inst) =>
          inst.id === selectedInstanceId() ? { ...inst, state: "running" } : inst
        )
      );

      setTimeout(() => {
        setLaunchState("default");
      }, 2500);
    }, 1200);
  };

  return (
    <div class="flex h-screen w-screen bg-nord-dark text-zinc-100 font-sans overflow-hidden">
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
                  ? "bg-nord-cyan text-nord-dark shadow-[0_0_10px_rgba(0,212,178,0.2)]"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Сборки"
              data-testid="nav-instances"
            >
              <LayoutGrid class="w-5 h-5" />
            </button>

            <button
              type="button"
              onClick={() => setCurrentNav("downloads")}
              class={`w-10 h-10 rounded-lg flex items-center justify-center transition-all ${
                currentNav() === "downloads"
                  ? "bg-nord-cyan text-nord-dark"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Загрузки"
              data-testid="nav-downloads"
            >
              <Download class="w-5 h-5" />
            </button>

            <button
              type="button"
              onClick={() => setCurrentNav("settings")}
              class={`w-10 h-10 rounded-lg flex items-center justify-center transition-all ${
                currentNav() === "settings"
                  ? "bg-nord-cyan text-nord-dark"
                  : "text-zinc-400 hover:text-white hover:bg-white/5"
              }`}
              title="Настройки"
              data-testid="nav-settings"
            >
              <Settings class="w-5 h-5" />
            </button>
          </nav>
        </div>

        <div class="flex flex-col items-center gap-3">
          <div
            class="w-9 h-9 rounded-full bg-zinc-800 border border-white/10 flex items-center justify-center text-zinc-300"
            title="Оффлайн игрок"
          >
            <User class="w-4 h-4" />
          </div>
        </div>
      </aside>

      {/* Main Workspace (Workbench Layout) */}
      <main class="flex-1 flex flex-col min-w-0 bg-nord-dark overflow-hidden">
        {/* Top Command Bar */}
        <header class="h-14 px-6 flex items-center justify-between border-b border-white/5 bg-nord-surface/40 backdrop-blur-md z-10">
          <div class="w-80">
            <ModSearchInput
              value={searchQuery()}
              matchCount={filteredInstances().length}
              onSearch={setSearchQuery}
              onClear={() => setSearchQuery("")}
            />
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
        <div class="flex-1 p-6 flex gap-6 overflow-y-auto">
          {/* Active Instance Cockpit (Hero Section) */}
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

            {/* High-frequency Stream Progress Indicator (Spike test) */}
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

          {/* Instances Drawer / List */}
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
      </main>
    </div>
  );
};