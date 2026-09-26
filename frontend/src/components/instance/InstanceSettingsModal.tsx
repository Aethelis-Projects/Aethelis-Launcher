import { Component, createSignal, createEffect, For, Show } from "solid-js";
import { X, Settings, Cpu, Layers, AlertTriangle, Check, Sliders, Package, Download, FolderOpen } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO, UpdateInstanceRequest, JavaInstallationDTO } from "../../bindings/ipc_types";
import { InstalledModsManager } from "../mods/InstalledModsManager";
import { ModCatalog } from "../mods/ModCatalog";

export function getRecommendedJavaMajor(version: string, manifestMajor?: number): number {
  if (manifestMajor && manifestMajor > 0) {
    return manifestMajor;
  }
  if (!version) return 21;
  const clean = version.replace(/^v/, "");
  const parts = clean.split(".").map((p) => {
    const m = p.match(/^\d+/);
    return m ? parseInt(m[0], 10) : 0;
  });
  const major = parts[0] || 0;
  const minor = parts[1] || 0;
  // 26.1+ -> 25
  if (major > 26 || (major === 26 && minor >= 1)) {
    return 25;
  }
  // 1.20.5 - 26.0 -> 21
  if (major === 26 && minor === 0) {
    return 21;
  }
  if (major === 1) {
    if (minor > 20 || (minor === 20 && (parts[2] || 0) >= 5)) {
      return 21;
    }
    if (minor >= 18) {
      return 17;
    }
    if (minor === 17) {
      return 16;
    }
    return 8;
  }
  if (major > 1 && major < 26) {
    return 21;
  }
  return 21;
}

export type SettingsTab = "general" | "java" | "memory" | "args" | "mods";

interface InstanceSettingsModalProps {
  instance: InstanceDTO;
  isOpen: boolean;
  onClose: () => void;
  onSaved: (updated: InstanceDTO) => void;
  onOpenJavaManager?: () => void;
  initialTab?: SettingsTab;
}

export const InstanceSettingsModal: Component<InstanceSettingsModalProps> = (props) => {
  const [activeTab, setActiveTab] = createSignal<SettingsTab>(props.initialTab || "general");
  const [modSubTab, setModSubTab] = createSignal<"installed" | "catalog">("installed");
  const [name, setName] = createSignal("");
  const [javaPath, setJavaPath] = createSignal("");
  const [skipJavaCheck, setSkipJavaCheck] = createSignal(false);
  const [minMemoryMb, setMinMemoryMb] = createSignal(2048);
  const [maxMemoryMb, setMaxMemoryMb] = createSignal(4096);
  const [customJvmArgs, setCustomJvmArgs] = createSignal("");
  const [saving, setSaving] = createSignal(false);
  const [error, setError] = createSignal("");
  const [availableRuntimes, setAvailableRuntimes] = createSignal<JavaInstallationDTO[]>([]);
  const [installingJava, setInstallingJava] = createSignal(false);
  const [installedJavaToast, setInstalledJavaToast] = createSignal("");

  const recommendedJava = () => getRecommendedJavaMajor(props.instance.game_version);

  const matchingInstalledRuntime = () => {
    const rec = recommendedJava();
    return availableRuntimes().find((r) => r.major_version === rec);
  };

  const handleInstallRecommendedJava = async () => {
    const rec = recommendedJava();
    setInstallingJava(true);
    setError("");
    try {
      await launcherAPI.downloadJavaRuntime(rec);
      setInstalledJavaToast(`Загрузка Java ${rec} LTS запущена`);
      await loadRuntimes();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(`Ошибка запуска установки Java: ${msg}`);
    } finally {
      setInstallingJava(false);
    }
  };

  const handleSelectRecommendedJava = (path: string) => {
    setJavaPath(path);
  };

  // Sync state whenever modal opens or instance changes
  createEffect(() => {
    if (props.isOpen && props.instance) {
      setActiveTab(props.initialTab || "general");
      setName(props.instance.name || "");
      setJavaPath(props.instance.java_path || "");
      setSkipJavaCheck(props.instance.skip_java_check || false);
      setMinMemoryMb(props.instance.min_ram_mb || 2048);
      setMaxMemoryMb(props.instance.max_ram_mb || 4096);
      setCustomJvmArgs(props.instance.jvm_args ? props.instance.jvm_args.join(" ") : "");
      setError("");

      // Fetch runtimes for easy selection in Java tab
      loadRuntimes();
    }
  });

  const loadRuntimes = async () => {
    try {
      const runtimes = await launcherAPI.listJavaRuntimes();
      setAvailableRuntimes(runtimes);
    } catch (_err: unknown) {
      // non-fatal
    }
  };

  const handleSave = async () => {
    if (!name().trim()) {
      setError("Название сборки не может быть пустым");
      return;
    }
    if (minMemoryMb() > maxMemoryMb()) {
      setError("Минимальный объем памяти не может превышать максимальный");
      return;
    }

    setSaving(true);
    setError("");

    try {
      const hadJavaPath = !!props.instance.java_path;
      const willHaveJavaPath = !!javaPath().trim();
      const jvmArgsParsed = customJvmArgs().trim() ? customJvmArgs().trim().split(/\s+/) : [];

      const req: UpdateInstanceRequest = {
        id: props.instance.id,
        name: name().trim(),
        java_path: willHaveJavaPath ? javaPath().trim() : undefined,
        clear_java_path: hadJavaPath && !willHaveJavaPath,
        skip_java_check: skipJavaCheck(),
        min_ram_mb: minMemoryMb(),
        max_ram_mb: maxMemoryMb(),
        jvm_args: jvmArgsParsed,
      };

      const updated = await launcherAPI.updateInstance(req);
      props.onSaved(updated);
      props.onClose();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
    } finally {
      setSaving(false);
    }
  };

  const applyMemoryPreset = (min: number, max: number) => {
    setMinMemoryMb(min);
    setMaxMemoryMb(max);
  };

  return (
    <Show when={props.isOpen}>
      <div
        class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm"
        data-testid="instance-settings-modal"
      >
        <div class={`w-full ${activeTab() === "mods" ? "max-w-4xl" : "max-w-2xl"} bg-nord-surface border border-white/10 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh] transition-all`}>
          {/* Header */}
          <div class="flex items-center justify-between px-6 py-4 border-b border-white/5 bg-nord-dark/40">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-nord-cyan/10 text-nord-cyan border border-nord-cyan/20">
                <Sliders class="w-5 h-5" />
              </div>
              <div>
                <h2 class="text-base font-bold text-white tracking-tight">
                  Настройки сборки: {props.instance.name}
                </h2>
                <p class="text-xs font-mono text-zinc-400">
                  Minecraft {props.instance.game_version} ({props.instance.loader})
                </p>
              </div>
            </div>
            <button
              type="button"
              onClick={props.onClose}
              class="p-2 rounded-lg text-zinc-400 hover:text-white hover:bg-white/5 transition-colors cursor-pointer"
              title="Закрыть"
              data-testid="modal-close-button"
            >
              <X class="w-5 h-5" />
            </button>
          </div>

          {/* Navigation Tabs */}
          <div class="flex border-b border-white/5 px-6 bg-nord-dark/20 text-xs font-medium">
            <button
              type="button"
              onClick={() => setActiveTab("general")}
              class={`py-3 px-4 flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
                activeTab() === "general"
                  ? "border-nord-cyan text-nord-cyan font-semibold"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
              data-testid="tab-general"
            >
              <Layers class="w-4 h-4" />
              <span>Общие</span>
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("java")}
              class={`py-3 px-4 flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
                activeTab() === "java"
                  ? "border-nord-cyan text-nord-cyan font-semibold"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
              data-testid="tab-java"
            >
              <Cpu class="w-4 h-4" />
              <span>Java рантайм</span>
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("memory")}
              class={`py-3 px-4 flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
                activeTab() === "memory"
                  ? "border-nord-cyan text-nord-cyan font-semibold"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
              data-testid="tab-memory"
            >
              <Settings class="w-4 h-4" />
              <span>Память RAM</span>
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("args")}
              class={`py-3 px-4 flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
                activeTab() === "args"
                  ? "border-nord-cyan text-nord-cyan font-semibold"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
              data-testid="tab-args"
            >
              <AlertTriangle class="w-4 h-4" />
              <span>Аргументы JVM</span>
            </button>
            <button
              type="button"
              onClick={() => setActiveTab("mods")}
              class={`py-3 px-4 flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
                activeTab() === "mods"
                  ? "border-nord-cyan text-nord-cyan font-semibold"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
              data-testid="tab-mods"
            >
              <Package class="w-4 h-4" />
              <span>Моды</span>
            </button>
          </div>

          {/* Error Banner */}
          <Show when={error()}>
            <div
              class="mx-6 mt-4 p-3 rounded-lg bg-nord-rose/10 border border-nord-rose/20 text-xs text-nord-rose flex items-center gap-2 font-mono"
              data-testid="settings-error-banner"
            >
              <AlertTriangle class="w-4 h-4 shrink-0" />
              <span>{error()}</span>
            </div>
          </Show>

          {/* Tab Content */}
          <div class="flex-1 p-6 overflow-y-auto space-y-4 text-xs">
            {/* TAB 1: General */}
            <Show when={activeTab() === "general"}>
              <div class="space-y-4">
                <div>
                  <label class="block text-zinc-300 font-semibold mb-1.5">
                    Название сборки
                  </label>
                  <input
                    type="text"
                    value={name()}
                    onInput={(e) => setName(e.currentTarget.value)}
                    placeholder="Например: Survival 1.21"
                    class="w-full px-3 py-2 rounded-lg bg-zinc-900 border border-white/10 text-white font-medium focus:outline-none focus:border-nord-cyan"
                    data-testid="settings-name-input"
                  />
                </div>

                <div class="grid grid-cols-2 gap-4 pt-2">
                  <div class="p-3 rounded-xl bg-black/20 border border-white/5 space-y-1">
                    <span class="text-zinc-500 text-[11px] font-mono">Версия игры</span>
                    <p class="text-white font-mono font-bold text-sm">
                      Minecraft {props.instance.game_version}
                    </p>
                  </div>
                  <div class="p-3 rounded-xl bg-black/20 border border-white/5 space-y-1">
                    <span class="text-zinc-500 text-[11px] font-mono">Мод-загрузчик</span>
                    <p class="text-white font-mono font-bold text-sm capitalize">
                      {props.instance.loader} {props.instance.loader_version || ""}
                    </p>
                  </div>
                </div>

                {/* Java Recommendation Chip */}
                <div
                  class="p-3.5 rounded-xl bg-nord-cyan/5 border border-nord-cyan/20 flex items-center justify-between gap-3"
                  data-testid="recommended-java-chip"
                >
                  <div class="flex items-center gap-2.5">
                    <div class="p-1.5 rounded-lg bg-nord-cyan/10 text-nord-cyan">
                      <Cpu class="w-4 h-4" />
                    </div>
                    <div>
                      <p class="text-xs font-semibold text-white">
                        Рекомендуется для Minecraft {props.instance.game_version}: Java {recommendedJava()} LTS
                      </p>
                      <p class="text-[11px] text-zinc-400">
                        <Show
                          when={matchingInstalledRuntime()}
                          fallback={"Рантайм не установлен в системе"}
                        >
                          <Show
                            when={javaPath() === matchingInstalledRuntime()!.path || (!javaPath() && matchingInstalledRuntime()?.kind === "managed")}
                            fallback={`Установлен: ${matchingInstalledRuntime()!.path}`}
                          >
                            Рантайм установлен и активен
                          </Show>
                        </Show>
                      </p>
                    </div>
                  </div>

                  <div>
                    <Show
                      when={matchingInstalledRuntime()}
                      fallback={
                        <button
                          type="button"
                          onClick={handleInstallRecommendedJava}
                          disabled={installingJava()}
                          class="px-3 py-1.5 rounded-lg bg-nord-cyan/20 hover:bg-nord-cyan/30 text-nord-cyan border border-nord-cyan/40 font-semibold text-xs flex items-center gap-1.5 transition-colors cursor-pointer disabled:opacity-50"
                          data-testid="install-recommended-java-button"
                        >
                          <Download class="w-3.5 h-3.5" />
                          <span>{installingJava() ? "Установка..." : `Установить Java ${recommendedJava()}`}</span>
                        </button>
                      }
                    >
                      {(rt) => (
                        <Show
                          when={javaPath() === rt().path}
                          fallback={
                            <button
                              type="button"
                              onClick={() => handleSelectRecommendedJava(rt().path)}
                              class="px-3 py-1.5 rounded-lg bg-nord-cyan/20 hover:bg-nord-cyan/30 text-nord-cyan border border-nord-cyan/40 font-semibold text-xs flex items-center gap-1.5 transition-colors cursor-pointer"
                              data-testid="select-recommended-java-button"
                            >
                              <Check class="w-3.5 h-3.5" />
                              <span>Выбрать Java {recommendedJava()}</span>
                            </button>
                          }
                        >
                          <span class="px-2.5 py-1 rounded-md bg-nord-emerald/10 border border-nord-emerald/20 text-nord-emerald text-[11px] font-medium flex items-center gap-1">
                            <Check class="w-3 h-3" />
                            Активна
                          </span>
                        </Show>
                      )}
                    </Show>
                  </div>
                </div>

                <Show when={installedJavaToast()}>
                  <div
                    class="p-2.5 rounded-lg bg-nord-cyan/10 border border-nord-cyan/20 text-xs text-nord-cyan font-mono"
                    data-testid="java-action-toast"
                  >
                    {installedJavaToast()}
                  </div>
                </Show>

                {/* Instance Folder Opener */}
                <div class="flex items-center justify-between p-3.5 rounded-xl bg-black/20 border border-white/5">
                  <div>
                    <span class="text-xs font-semibold text-white">Каталог файлов инстанса</span>
                    <p class="text-[11px] text-zinc-400">
                      Открыть корневую папку сборки, конфигураций и сохранений в проводнике
                    </p>
                  </div>
                  <button
                    type="button"
                    onClick={() => launcherAPI.openPath(props.instance.id)}
                    class="px-3 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-200 border border-white/10 text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer"
                    data-testid="settings-open-folder-btn"
                  >
                    <FolderOpen class="w-3.5 h-3.5 text-nord-cyan" />
                    <span>Папка инстанса</span>
                  </button>
                </div>
              </div>
            </Show>

            {/* TAB 2: Java */}
            <Show when={activeTab() === "java"}>
              <div class="space-y-4">
                <div>
                  <div class="flex items-center justify-between mb-1.5">
                    <label class="text-zinc-300 font-semibold">
                      Путь к исполняемому файлу Java (JavaPath)
                    </label>
                    <Show when={javaPath()}>
                      <button
                        type="button"
                        onClick={() => setJavaPath("")}
                        class="text-[11px] text-nord-cyan hover:underline cursor-pointer"
                        data-testid="settings-clear-java-path"
                      >
                        Сбросить на авто-определение
                      </button>
                    </Show>
                  </div>
                  <input
                    type="text"
                    value={javaPath()}
                    onInput={(e) => setJavaPath(e.currentTarget.value)}
                    placeholder="Оставьте пустым для авто-выбора или укажите путь к java.exe"
                    class="w-full px-3 py-2 rounded-lg bg-zinc-900 border border-white/10 text-white font-mono text-xs focus:outline-none focus:border-nord-cyan"
                    data-testid="settings-java-path-input"
                  />
                  <p class="text-[11px] text-zinc-500 mt-1">
                    По умолчанию Nord Launcher автоматически подбирает и использует управляемый Adoptium JDK.
                  </p>
                </div>

                {/* Quick runtime selector from detected/managed list */}
                <Show when={availableRuntimes().length > 0}>
                  <div class="p-3 rounded-xl bg-black/20 border border-white/5 space-y-2">
                    <div class="flex items-center justify-between">
                      <span class="text-[11px] font-semibold text-zinc-400 uppercase tracking-wider">
                        Доступные рантаймы в системе
                      </span>
                      <Show when={props.onOpenJavaManager}>
                        <button
                          type="button"
                          onClick={() => {
                            props.onClose();
                            props.onOpenJavaManager?.();
                          }}
                          class="text-[11px] text-nord-cyan hover:underline cursor-pointer"
                        >
                          Открыть Java Manager
                        </button>
                      </Show>
                    </div>

                    <div class="space-y-1.5 max-h-36 overflow-y-auto pr-1">
                      <For each={availableRuntimes()}>
                        {(rt) => (
                          <div
                            onClick={() => setJavaPath(rt.path)}
                            class={`p-2 rounded-lg border text-xs flex items-center justify-between cursor-pointer transition-all ${
                              javaPath() === rt.path
                                ? "bg-nord-cyan/15 border-nord-cyan/40 text-white"
                                : "bg-zinc-900/60 border-white/5 text-zinc-300 hover:bg-white/5"
                            }`}
                          >
                            <div class="flex items-center gap-2 min-w-0">
                              <span class="px-1.5 py-0.5 rounded bg-white/10 font-mono text-[10px] font-bold">
                                Java {rt.major_version}
                              </span>
                              <span class="truncate font-mono text-[11px]">{rt.path}</span>
                            </div>
                            <span class="text-[10px] font-mono capitalize text-zinc-500 shrink-0 ml-2">
                              {rt.kind}
                            </span>
                          </div>
                        )}
                      </For>
                    </div>
                  </div>
                </Show>

                {/* Skip Java check toggle */}
                <div class="p-3 rounded-xl bg-nord-amber/5 border border-nord-amber/20 space-y-2">
                  <label class="flex items-start gap-3 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={skipJavaCheck()}
                      onChange={(e) => setSkipJavaCheck(e.currentTarget.checked)}
                      class="mt-0.5 rounded bg-zinc-900 border-white/20 text-nord-cyan focus:ring-0 focus:ring-offset-0"
                      data-testid="settings-skip-java-check-toggle"
                    />
                    <div>
                      <span class="font-semibold text-zinc-200">
                        Пропустить проверку совместимости Java (skip_java_check)
                      </span>
                      <p class="text-[11px] text-zinc-400 mt-0.5">
                        Отключает fail-closed проверку мажорной версии Java при запуске. Используйте только если уверены в совместимости стороннего рантайма с данной версией Minecraft.
                      </p>
                    </div>
                  </label>
                </div>
              </div>
            </Show>

            {/* TAB 3: Memory */}
            <Show when={activeTab() === "memory"}>
              <div class="space-y-4">
                {/* Presets */}
                <div>
                  <span class="block text-zinc-400 font-semibold mb-2">
                    Быстрые пресеты памяти
                  </span>
                  <div class="grid grid-cols-4 gap-2">
                    <button
                      type="button"
                      onClick={() => applyMemoryPreset(2048, 2048)}
                      class="py-2 px-3 rounded-lg bg-zinc-900 border border-white/10 hover:bg-white/10 font-mono text-center transition-colors cursor-pointer text-zinc-200"
                    >
                      2 GB
                    </button>
                    <button
                      type="button"
                      onClick={() => applyMemoryPreset(2048, 4096)}
                      class="py-2 px-3 rounded-lg bg-zinc-900 border border-white/10 hover:bg-white/10 font-mono text-center transition-colors cursor-pointer text-zinc-200"
                    >
                      4 GB
                    </button>
                    <button
                      type="button"
                      onClick={() => applyMemoryPreset(3072, 6144)}
                      class="py-2 px-3 rounded-lg bg-zinc-900 border border-white/10 hover:bg-white/10 font-mono text-center transition-colors cursor-pointer text-zinc-200"
                    >
                      6 GB
                    </button>
                    <button
                      type="button"
                      onClick={() => applyMemoryPreset(4096, 8192)}
                      class="py-2 px-3 rounded-lg bg-zinc-900 border border-white/10 hover:bg-white/10 font-mono text-center transition-colors cursor-pointer text-zinc-200"
                    >
                      8 GB
                    </button>
                  </div>
                </div>

                {/* Min Memory Slider & Input */}
                <div class="p-4 rounded-xl bg-black/20 border border-white/5 space-y-2">
                  <div class="flex items-center justify-between">
                    <label class="text-zinc-300 font-semibold">
                      Минимальная память (-Xms): {minMemoryMb()} МБ
                    </label>
                    <span class="text-zinc-500 font-mono text-[11px]">
                      {(minMemoryMb() / 1024).toFixed(1)} GB
                    </span>
                  </div>
                  <input
                    type="range"
                    min="512"
                    max="16384"
                    step="256"
                    value={minMemoryMb()}
                    onInput={(e) => setMinMemoryMb(Number(e.currentTarget.value))}
                    class="w-full accent-nord-cyan cursor-pointer"
                    data-testid="settings-min-ram-slider"
                  />
                  <input
                    type="number"
                    min="512"
                    max="32768"
                    step="256"
                    value={minMemoryMb()}
                    onInput={(e) => setMinMemoryMb(Number(e.currentTarget.value))}
                    class="w-32 px-2 py-1 rounded bg-zinc-900 border border-white/10 text-white font-mono text-xs"
                    data-testid="settings-min-ram-input"
                  />
                </div>

                {/* Max Memory Slider & Input */}
                <div class="p-4 rounded-xl bg-black/20 border border-white/5 space-y-2">
                  <div class="flex items-center justify-between">
                    <label class="text-zinc-300 font-semibold">
                      Максимальная память (-Xmx): {maxMemoryMb()} МБ
                    </label>
                    <span class="text-zinc-500 font-mono text-[11px]">
                      {(maxMemoryMb() / 1024).toFixed(1)} GB
                    </span>
                  </div>
                  <input
                    type="range"
                    min="1024"
                    max="32768"
                    step="512"
                    value={maxMemoryMb()}
                    onInput={(e) => setMaxMemoryMb(Number(e.currentTarget.value))}
                    class="w-full accent-nord-cyan cursor-pointer"
                    data-testid="settings-max-ram-slider"
                  />
                  <input
                    type="number"
                    min="1024"
                    max="65536"
                    step="512"
                    value={maxMemoryMb()}
                    onInput={(e) => setMaxMemoryMb(Number(e.currentTarget.value))}
                    class="w-32 px-2 py-1 rounded bg-zinc-900 border border-white/10 text-white font-mono text-xs"
                    data-testid="settings-max-ram-input"
                  />
                </div>
              </div>
            </Show>

            {/* TAB 4: Arguments */}
            <Show when={activeTab() === "args"}>
              <div class="space-y-4">
                <div>
                  <label class="block text-zinc-300 font-semibold mb-1.5">
                    Пользовательские флаги запуска JVM (custom_jvm_args)
                  </label>
                  <textarea
                    rows={4}
                    value={customJvmArgs()}
                    onInput={(e) => setCustomJvmArgs(e.currentTarget.value)}
                    placeholder="Например: -XX:+UseG1GC -XX:+ParallelRefProcEnabled"
                    class="w-full px-3 py-2 rounded-lg bg-zinc-900 border border-white/10 text-white font-mono text-xs focus:outline-none focus:border-nord-cyan resize-none"
                    data-testid="settings-jvm-args-input"
                  />
                </div>

                {/* Security Warning Callout (R4) */}
                <div class="p-3.5 rounded-xl bg-nord-amber/10 border border-nord-amber/30 text-nord-amber space-y-1">
                  <div class="flex items-center gap-2 font-bold text-xs">
                    <AlertTriangle class="w-4 h-4 shrink-0" />
                    <span>Предупреждение безопасности</span>
                  </div>
                  <p class="text-[11px] text-zinc-300 leading-relaxed">
                    Аргументы JVM передаются процессу Java без фильтрации. Не добавляйте непроверенные флаги или аргументы из ненадежных источников во избежание сбоев или уязвимостей.
                  </p>
                </div>
              </div>
            </Show>

            {/* TAB 5: Mods (Installed vs Catalog) */}
            <Show when={activeTab() === "mods"}>
              <div class="space-y-4">
                {/* Segmented Control */}
                <div class="flex items-center bg-zinc-900 border border-zinc-800 rounded-lg p-1 w-fit">
                  <button
                    type="button"
                    onClick={() => setModSubTab("installed")}
                    class={`px-4 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer ${
                      modSubTab() === "installed"
                        ? "bg-[#00D4B2] text-zinc-950 font-semibold shadow"
                        : "text-zinc-400 hover:text-zinc-100"
                    }`}
                    data-testid="mods-subtab-installed"
                  >
                    Установленные
                  </button>
                  <button
                    type="button"
                    onClick={() => setModSubTab("catalog")}
                    class={`px-4 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer ${
                      modSubTab() === "catalog"
                        ? "bg-[#00D4B2] text-zinc-950 font-semibold shadow"
                        : "text-zinc-400 hover:text-zinc-100"
                    }`}
                    data-testid="mods-subtab-catalog"
                  >
                    Каталог
                  </button>
                </div>

                <Show when={modSubTab() === "installed"}>
                  <InstalledModsManager instanceId={props.instance.id} />
                </Show>

                <Show when={modSubTab() === "catalog"}>
                  <ModCatalog
                    activeInstanceId={props.instance.id}
                    gameVersion={props.instance.game_version}
                    loader={props.instance.loader}
                  />
                </Show>
              </div>
            </Show>
          </div>

          {/* Footer Actions */}
          <div class="flex items-center justify-end gap-3 px-6 py-4 border-t border-white/5 bg-nord-dark/40">
            <Show when={activeTab() !== "mods"}>
              <button
                type="button"
                onClick={props.onClose}
                disabled={saving()}
                class="px-4 py-2 rounded-lg bg-white/5 hover:bg-white/10 text-xs font-medium text-zinc-300 transition-colors cursor-pointer"
                data-testid="settings-cancel-button"
              >
                Отмена
              </button>
              <button
                type="button"
                onClick={handleSave}
                disabled={saving()}
                class="px-5 py-2 rounded-lg bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark font-bold text-xs flex items-center gap-2 transition-all cursor-pointer disabled:opacity-50"
                data-testid="settings-save-button"
              >
                <Check class="w-4 h-4" />
                <span>{saving() ? "Сохранение..." : "Сохранить настройки"}</span>
              </button>
            </Show>
            <Show when={activeTab() === "mods"}>
              <button
                type="button"
                onClick={props.onClose}
                class="px-5 py-2 rounded-lg bg-white/10 hover:bg-white/20 text-xs font-semibold text-white transition-colors cursor-pointer"
                data-testid="settings-close-mods-button"
              >
                Закрыть
              </button>
            </Show>
          </div>
        </div>
      </div>
    </Show>
  );
};
