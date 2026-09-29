import { Component, createSignal, createEffect, For, Show } from "solid-js";
import { X, Cpu, Layers, AlertTriangle, Check, Sliders, Package, Download, FolderOpen, Dices, Database, ShieldCheck } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { InstanceDTO, UpdateInstanceRequest, JavaInstallationDTO } from "../../bindings/ipc_types";
import type { IntegrityResultDTO, PerformancePresetDTO } from "../../bindings/ipc_types";
import { InstalledModsManager } from "../mods/InstalledModsManager";
import { DatapackManager } from "./DatapackManager";
import { PRESET_AVATARS, getRandomAvatar } from "../../assets/avatars";

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

export type SettingsTab = "general" | "performance" | "mods" | "datapacks" | "integrity";

interface InstanceSettingsModalProps {
  instance: InstanceDTO;
  isOpen: boolean;
  onClose: () => void;
  onSaved: (updated: InstanceDTO) => void;
  onOpenJavaManager?: () => void;
  onOpenCatalog?: () => void;
  initialTab?: SettingsTab;
}

export const InstanceSettingsModal: Component<InstanceSettingsModalProps> = (props) => {
  const [activeTab, setActiveTab] = createSignal<SettingsTab>(props.initialTab || "general");
  const [name, setName] = createSignal("");
  const [group, setGroup] = createSignal("");
  const [iconPath, setIconPath] = createSignal("");
  const [javaPath, setJavaPath] = createSignal("");
  const [skipJavaCheck, setSkipJavaCheck] = createSignal(false);
  const [minMemoryMb, setMinMemoryMb] = createSignal(2048);
  const [maxMemoryMb, setMaxMemoryMb] = createSignal(4096);
  const [customJvmArgs, setCustomJvmArgs] = createSignal("");
  const [saving, setSaving] = createSignal(false);
  const [error, setError] = createSignal("");
  const [availableRuntimes, setAvailableRuntimes] = createSignal<JavaInstallationDTO[]>([]);
  const [installingJava, setInstallingJava] = createSignal(false);
  const [perfPreset, setPerfPreset] = createSignal<PerformancePresetDTO | null>(null);
  const [discordEnabled, setDiscordEnabled] = createSignal(false);

  const handleToggleDiscord = async () => {
    const next = !discordEnabled();
    try {
      await launcherAPI.setDiscordRpcEnabled(next);
      setDiscordEnabled(next);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg || "Не удалось переключить Discord-присутствие");
    }
  };
  const [integrityResult, setIntegrityResult] = createSignal<IntegrityResultDTO | null>(null);
  const [integrityBusy, setIntegrityBusy] = createSignal<"check" | "repair" | "">("");

  // v0.7.2: standalone "recommended Java" plate is gone; the unified tab only
  // offers to install a missing recommended runtime.
  const recommendedJava = () => getRecommendedJavaMajor(props.instance.game_version);
  const recommendedMissing = () =>
    !!props.instance.game_version &&
    !availableRuntimes().some((r) => r.major_version === recommendedJava());
  const handleInstallRecommendedJava = async () => {
    setInstallingJava(true);
    try {
      await launcherAPI.downloadJavaRuntime(recommendedJava());
      setTimeout(() => { loadRuntimes().catch(() => {}); setInstallingJava(false); }, 1500);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg || "Не удалось запустить установку Java");
      setInstallingJava(false);
    }
  };

  // Sync state whenever modal opens or instance changes
  createEffect(() => {
    if (props.isOpen && props.instance) {
      setActiveTab(props.initialTab || "general");
      setName(props.instance.name || "");
      setGroup(props.instance.group || "");
      setIconPath(props.instance.icon_path || "");
      setJavaPath(props.instance.java_path || "");
      setSkipJavaCheck(props.instance.skip_java_check || false);
      setMinMemoryMb(props.instance.min_ram_mb || 2048);
      setMaxMemoryMb(props.instance.max_ram_mb || 4096);
      setCustomJvmArgs(props.instance.jvm_args ? props.instance.jvm_args.join(" ") : "");
      setError("");
      launcherAPI.getPerformancePreset().then(setPerfPreset).catch(() => setPerfPreset(null));
      launcherAPI
        .getDiscordRpcStatus()
        .then((st) => setDiscordEnabled(st?.enabled ?? false))
        .catch(() => setDiscordEnabled(false));

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
        group: group().trim(),
        icon_path: iconPath().trim() || undefined,
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

  const AIKAR_PREFIXES = [
    "-XX:+UseG1GC", "-XX:+ParallelRefProcEnabled", "-XX:MaxGCPauseMillis", "-XX:+UnlockExperimentalVMOptions",
    "-XX:+DisableExplicitGC", "-XX:+AlwaysPreTouch", "-XX:G1NewSizePercent", "-XX:G1MaxNewSizePercent",
    "-XX:G1HeapRegionSize", "-XX:G1ReservePercent", "-XX:G1HeapWastePercent", "-XX:G1MixedGCCountTarget",
    "-XX:InitiatingHeapOccupancyPercent", "-XX:G1MixedGCLiveThresholdPercent", "-XX:G1RSetUpdatingPauseTimePercent",
    "-XX:SurvivorRatio", "-XX:+PerfDisableSharedMem", "-XX:MaxTenuringThreshold",
  ];

  const currentJvmArgList = (): string[] => (customJvmArgs().trim() ? customJvmArgs().trim().split(/\s+/) : []);

  const isAikarPresetActive = (): boolean => {
    const preset = perfPreset();
    if (!preset || preset.aikar_args.length === 0) return false;
    const current = new Set(currentJvmArgList());
    return preset.aikar_args.every((flag) => current.has(flag));
  };

  const handleApplyAikarPreset = () => {
    const preset = perfPreset();
    if (!preset) return;
    const rest = currentJvmArgList().filter((a) => !AIKAR_PREFIXES.some((p) => a.startsWith(p)));
    const merged = [...preset.aikar_args, ...rest];
    setCustomJvmArgs(merged.join(" "));
    const suggested = preset.suggested_ram_mb;
    if (minMemoryMb() === 0 || maxMemoryMb() === 0) {
      applyMemoryPreset(suggested, suggested);
    }
    setError("");
  };

  const handleResetAikarPreset = () => {
    const rest = currentJvmArgList().filter((a) => !AIKAR_PREFIXES.some((p) => a.startsWith(p)));
    setCustomJvmArgs(rest.join(" "));
  };

  const handleRunIntegrity = async (mode: "check" | "repair") => {
    setIntegrityBusy(mode);
    setError("");
    try {
      const res = mode === "repair"
        ? await launcherAPI.repairInstanceFiles(props.instance.id)
        : await launcherAPI.checkInstanceFiles(props.instance.id);
      setIntegrityResult(res);
      if (mode === "repair" && res.problems_count === 0 && res.repaired_count > 0) {
        setError("");
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg || "Не удалось проверить файлы");
    } finally {
      setIntegrityBusy("");
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
        <div class={`w-full ${activeTab() === "mods" || activeTab() === "datapacks" ? "max-w-4xl" : "max-w-2xl"} bg-nord-surface border border-white/10 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[90vh] transition-all`}>
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
              onClick={() => setActiveTab("performance")}
              class={`py-3 px-4 flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
                activeTab() === "performance"
                  ? "border-nord-cyan text-nord-cyan font-semibold"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
              data-testid="tab-performance"
            >
              <Cpu class="w-4 h-4" />
              <span>Производительность и Java</span>
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
            <button
              type="button"
              onClick={() => setActiveTab("datapacks")}
              class={`py-3 px-4 flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
                activeTab() === "datapacks"
                  ? "border-nord-cyan text-nord-cyan font-semibold"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
              data-testid="tab-datapacks"
            >
              <Database class="w-4 h-4" />
              <span>Датапаки</span>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab("integrity")}
              class={`py-3 px-4 flex items-center gap-2 border-b-2 transition-all cursor-pointer ${
                activeTab() === "integrity"
                  ? "border-nord-cyan text-nord-cyan font-semibold"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
              data-testid="tab-integrity"
            >
              <ShieldCheck class="w-4 h-4" />
              <span>Файлы</span>
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
                <div class="grid grid-cols-2 gap-4">
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
                  <div>
                    <label class="block text-zinc-300 font-semibold mb-1.5">
                      Группа
                    </label>
                    <input
                      type="text"
                      value={group()}
                      onInput={(e) => setGroup(e.currentTarget.value)}
                      placeholder="Например: SMP, Vanilla, Моды"
                      class="w-full px-3 py-2 rounded-lg bg-zinc-900 border border-white/10 text-white font-medium focus:outline-none focus:border-nord-cyan"
                      data-testid="settings-group-input"
                    />
                  </div>
                </div>

                {/* Avatar Picker & Randomizer (UX1) */}
                <div class="p-3.5 rounded-xl bg-zinc-900/60 border border-white/5 space-y-3" data-testid="avatar-picker-section">
                  <div class="flex items-center justify-between">
                    <div>
                      <label class="block text-zinc-300 font-semibold text-xs">
                        Иконка сборки
                      </label>
                      <p class="text-[11px] text-zinc-400">
                        Выберите аватар или бросьте кости для случайного выбора
                      </p>
                    </div>

                    <button
                      type="button"
                      onClick={() => {
                        const random = getRandomAvatar();
                        setIconPath(random.dataUrl);
                      }}
                      class="px-2.5 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-nord-cyan border border-white/10 text-xs font-mono font-medium flex items-center gap-1.5 transition-colors cursor-pointer"
                      title="Случайная иконка"
                      data-testid="random-avatar-btn"
                    >
                      <Dices class="w-3.5 h-3.5" />
                      <span>Случайно</span>
                    </button>
                  </div>

                  {/* Preset Avatar Grid */}
                  <div class="grid grid-cols-4 sm:grid-cols-8 gap-2" data-testid="avatar-grid">
                    <For each={PRESET_AVATARS}>
                      {(avatar) => (
                        <button
                          type="button"
                          onClick={() => setIconPath(avatar.dataUrl)}
                          class={`p-1.5 rounded-lg border transition-all flex flex-col items-center justify-center gap-1 cursor-pointer ${
                            iconPath() === avatar.dataUrl
                              ? "bg-nord-cyan/15 border-[#00D4B2] ring-1 ring-[#00D4B2]"
                              : "bg-zinc-800/60 border-white/5 hover:border-white/20"
                          }`}
                          title={avatar.name}
                          data-testid={`avatar-preset-${avatar.id}`}
                        >
                          <img src={avatar.dataUrl} alt={avatar.name} class="w-6 h-6 object-contain" />
                          <span class="text-[9px] font-mono text-zinc-400 truncate max-w-full">
                            {avatar.name}
                          </span>
                        </button>
                      )}
                    </For>
                  </div>
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
            <Show when={activeTab() === "performance"}>
              <div class="space-y-4">
                <p class="text-[11px] text-zinc-500">Единственная вкладка тюнинга: Java, память, флаги JVM и пресеты — всё здесь.</p>
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

                <Show when={recommendedMissing()}>
                      <Show when={recommendedMissing()}>
                        <button
                          type="button"
                          onClick={handleInstallRecommendedJava}
                          disabled={installingJava()}
                          class="w-full py-1.5 rounded-lg bg-nord-cyan/15 hover:bg-nord-cyan/25 border border-nord-cyan/30 text-nord-cyan text-[11px] font-semibold flex items-center justify-center gap-1.5 transition-colors cursor-pointer disabled:opacity-50"
                          data-testid="install-recommended-java-button"
                        >
                          <Download class="w-3.5 h-3.5" />
                          <span>{installingJava() ? "Установка..." : `Установить рекомендуемую Java ${recommendedJava()}`}</span>
                        </button>
                      </Show>
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

                <div class="p-4 rounded-xl bg-zinc-900/60 border border-white/10 space-y-3">
                  <div class="flex items-center justify-between">
                    <span class="text-zinc-200 font-semibold text-sm">Пресеты производительности</span>
                    <Show when={isAikarPresetActive()}>
                      <span class="text-[11px] px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 font-medium" data-testid="aikar-active-badge">
                        Применён
                      </span>
                    </Show>
                  </div>
                  <Show when={perfPreset()}>
                    <p class="text-xs text-zinc-400 leading-relaxed">
                      Рекомендация по памяти для этого ПК: <span class="font-mono text-nord-cyan">{perfPreset()!.suggested_ram_mb} МБ</span>. Флаги Aikar G1GC — канонический набор настройки сборщика мусора (источник: aikar.co/mcflags); применять только с G1, не совмещать с -XX:+UseZGC.
                    </p>
                  </Show>
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={handleApplyAikarPreset}
                      disabled={!perfPreset() || perfPreset()!.aikar_args.length === 0}
                      class="py-2 px-4 rounded-lg bg-[#00D4B2] text-zinc-950 text-xs font-semibold hover:bg-[#00e6c3] disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
                      data-testid="aikar-apply-btn"
                    >
                      Применить Aikar G1
                    </button>
                    <button
                      type="button"
                      onClick={handleResetAikarPreset}
                      class="py-2 px-4 rounded-lg bg-zinc-800 border border-white/10 text-zinc-300 text-xs font-medium hover:bg-zinc-700 transition-colors cursor-pointer"
                      data-testid="aikar-reset-btn"
                    >
                      Сбросить флаги
                    </button>
                  </div>
                  <p class="text-[10px] text-zinc-500 leading-relaxed">
                    Флаги добавляются в пользовательские аргументы JVM — итог виден ниже на этой же вкладке и сохраняется кнопкой «Сохранить».
                  </p>
                </div>

                <div class="p-3.5 rounded-xl bg-black/20 border border-white/5 flex items-center justify-between gap-3">
                  <div>
                    <span class="text-xs font-semibold text-white">Показывать статус в Discord</span>
                    <p class="text-[11px] text-zinc-400">
                      Локальный показ статуса игры в Discord (launch-local IPC, без сетевых запросов).
                    </p>
                  </div>
                  <button
                    type="button"
                    role="switch"
                    aria-checked={discordEnabled()}
                    onClick={handleToggleDiscord}
                    class={`relative h-6 w-11 rounded-full transition-colors cursor-pointer ${
                      discordEnabled() ? "bg-nord-cyan" : "bg-zinc-700"
                    }`}
                    data-testid="perf-discord-toggle"
                  >
                    <span
                      class={`absolute top-0.5 h-5 w-5 rounded-full bg-white transition-all ${
                        discordEnabled() ? "left-[22px]" : "left-0.5"
                      }`}
                    />
                  </button>
                </div>
              </div>
            </Show>

            {/* Memory (merged) */}
            <Show when={activeTab() === "performance"}>
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

            {/* Arguments (merged) */}
            <Show when={activeTab() === "performance"}>
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

            {/* TAB 5: Mods - installed manager only (catalog moved to its own page, v0.7.2 G3) */}
            <Show when={activeTab() === "mods"}>
              <div class="space-y-4">
                <div class="flex items-center justify-between">
                  <span class="text-zinc-200 font-semibold text-sm">Управление установленными модификациями</span>
                  <Show when={props.onOpenCatalog}>
                    <button
                      type="button"
                      onClick={() => {
                        props.onClose();
                        props.onOpenCatalog?.();
                      }}
                      class="text-[11px] text-nord-cyan hover:underline cursor-pointer"
                      data-testid="settings-open-catalog-link"
                    >
                      Открыть каталог
                    </button>
                  </Show>
                </div>
                <InstalledModsManager instanceId={props.instance.id} />
              </div>
            </Show>

            {/* TAB 6: Datapacks */}
            <Show when={activeTab() === "datapacks"}>
              <DatapackManager instanceId={props.instance.id} />
            </Show>
            {/* TAB 8: Integrity (D'2) */}
            <Show when={activeTab() === "integrity"}>
              <div class="space-y-4">
                <div class="p-4 rounded-xl bg-zinc-900/60 border border-white/10 space-y-3">
                  <span class="block text-zinc-200 font-semibold text-sm">Целостность файлов игры</span>
                  <p class="text-xs text-zinc-400 leading-relaxed">
                    Сверяет кэш Mojang (version JSON, client.jar, библиотеки, нативы, индексы и объекты ассетов) с официальными SHA-1 и докачивает только повреждённое. Библиотеки лоадера и моды проверяются своим механизмом (self-heal) и сюда не входят.
                  </p>
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => handleRunIntegrity("check")}
                      disabled={integrityBusy() !== ""}
                      class="py-2 px-4 rounded-lg bg-zinc-800 border border-white/10 text-zinc-100 text-xs font-semibold hover:bg-zinc-700 disabled:opacity-50 transition-colors cursor-pointer"
                      data-testid="integrity-check-btn"
                    >
                      {integrityBusy() === "check" ? "Проверка..." : "Проверить файлы"}
                    </button>
                    <button
                      type="button"
                      onClick={() => handleRunIntegrity("repair")}
                      disabled={integrityBusy() !== ""}
                      class="py-2 px-4 rounded-lg bg-[#00D4B2] text-zinc-950 text-xs font-semibold hover:bg-[#00e6c3] disabled:opacity-50 transition-colors cursor-pointer"
                      data-testid="integrity-repair-btn"
                    >
                      {integrityBusy() === "repair" ? "Починка..." : "Проверить и починить"}
                    </button>
                  </div>
                  <Show when={integrityResult()}>
                    <div class="p-3 rounded-lg bg-zinc-950/60 border border-white/5 text-xs font-mono space-y-1.5" data-testid="integrity-result">
                      <p class="text-zinc-300">
                        Проверено файлов: <span class="text-nord-cyan">{integrityResult()!.checked_count}</span>
                        <Show when={integrityResult()!.repaired_count > 0}>
                          {" "}| Починено: <span class="text-emerald-400">{integrityResult()!.repaired_count}</span>
                        </Show>
                        {" "}| Проблем: <span class={integrityResult()!.problems_count > 0 ? "text-nord-rose" : "text-emerald-400"}>{integrityResult()!.problems_count}</span>
                      </p>
                      <Show when={integrityResult()!.virtual_assets_skipped}>
                        <p class="text-zinc-500">Виртуальные ассеты (старые версии) пропущены — чинятся пересозданием через Provision.</p>
                      </Show>
                      <Show when={integrityResult()!.problems_count > 0}>
                        <ul class="space-y-0.5 pt-1">
                          <For each={integrityResult()!.items}>
                            {(item) => (
                              <li class="text-nord-rose truncate">{item.path} - {item.reason}</li>
                            )}
                          </For>
                        </ul>
                        <Show when={integrityResult()!.problems_capped}>
                          <p class="text-zinc-500">Список усечён до 25 позиций; посчитаны все.</p>
                        </Show>
                      </Show>
                    </div>
                  </Show>
                </div>
              </div>
            </Show>
          </div>

          {/* Footer Actions */}
          <div class="flex items-center justify-end gap-3 px-6 py-4 border-t border-white/5 bg-nord-dark/40">
            <Show when={activeTab() !== "mods" && activeTab() !== "datapacks"}>
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
            <Show when={activeTab() === "mods" || activeTab() === "datapacks"}>
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
