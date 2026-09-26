import { Component, createSignal, Show, onMount, For } from "solid-js";
import { FolderDown, FolderOpen, Loader2, CheckCircle2, AlertCircle, X, Check, HardDrive, Cpu } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type {
  InstanceDTO,
  MinecraftImportSummaryDTO,
  PrismImportSummaryDTO,
} from "../../bindings/ipc_types";

interface ImportInstanceModalProps {
  isOpen: boolean;
  onClose: () => void;
  onImported?: (instance: InstanceDTO) => void;
}

export const ImportInstanceModal: Component<ImportInstanceModalProps> = (props) => {
  const [sourceType, setSourceType] = createSignal<"minecraft" | "prism">("minecraft");
  const [customPath, setCustomPath] = createSignal("");
  const [instanceName, setInstanceName] = createSignal("");
  const [gameVersion, setGameVersion] = createSignal("");
  const [loader, setLoader] = createSignal("vanilla");

  // Content selection checkboxes
  const [copySaves, setCopySaves] = createSignal(true);
  const [copyResourcePacks, setCopyResourcePacks] = createSignal(true);
  const [copyScreenshots, setCopyScreenshots] = createSignal(true);
  const [copyMods, setCopyMods] = createSignal(true);
  const [copyOptions, setCopyOptions] = createSignal(true);
  const [copyServers, setCopyServers] = createSignal(true);

  // Status
  const [isScanning, setIsScanning] = createSignal(false);
  const [scanError, setScanError] = createSignal("");
  const [officialSummary, setOfficialSummary] = createSignal<MinecraftImportSummaryDTO | null>(null);
  const [prismSummary, setPrismSummary] = createSignal<PrismImportSummaryDTO | null>(null);

  const [isImporting, setIsImporting] = createSignal(false);
  const [importError, setImportError] = createSignal("");
  const [isCompleted, setIsCompleted] = createSignal(false);

  const resetState = () => {
    setCustomPath("");
    setInstanceName("");
    setGameVersion("");
    setLoader("vanilla");
    setScanError("");
    setOfficialSummary(null);
    setPrismSummary(null);
    setIsScanning(false);
    setIsImporting(false);
    setImportError("");
    setIsCompleted(false);
  };

  const handleClose = () => {
    resetState();
    props.onClose();
  };

  const scanOfficial = async (dirPath?: string) => {
    setIsScanning(true);
    setScanError("");
    try {
      const summary = await launcherAPI.scanOfficialMinecraft({ dir_path: dirPath || undefined });
      setOfficialSummary(summary);
      setGameVersion(summary.default_version || "1.21.1");
      setInstanceName(`Minecraft ${summary.default_version || "1.21.1"}`);
      setLoader(summary.mod_count > 0 ? "fabric" : "vanilla");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setScanError(`Не удалось просканировать .minecraft: ${msg}`);
      setOfficialSummary(null);
    } finally {
      setIsScanning(false);
    }
  };

  const scanPrism = async (dirPath: string) => {
    const trimmed = dirPath.trim();
    if (!trimmed) {
      setScanError("Укажите путь к папке инстанса Prism или MultiMC");
      return;
    }
    setIsScanning(true);
    setScanError("");
    try {
      const summary = await launcherAPI.scanPrismInstance({ dir_path: trimmed });
      setPrismSummary(summary);
      setInstanceName(summary.instance_name || "Imported Prism");
      setGameVersion(summary.game_version || "1.21.1");
      setLoader(summary.loader || "vanilla");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setScanError(`Не удалось просканировать инстанс: ${msg}`);
      setPrismSummary(null);
    } finally {
      setIsScanning(false);
    }
  };

  const handleSourceTabChange = (type: "minecraft" | "prism") => {
    setSourceType(type);
    setScanError("");
    setImportError("");
    setIsCompleted(false);
    if (type === "minecraft") {
      if (!officialSummary()) {
        scanOfficial(customPath() || undefined);
      }
    } else {
      if (customPath()) {
        scanPrism(customPath());
      }
    }
  };

  onMount(() => {
    if (props.isOpen && sourceType() === "minecraft") {
      scanOfficial();
    }
  });

  const handleStartImport = async () => {
    setIsImporting(true);
    setImportError("");

    try {
      let imported: InstanceDTO;
      if (sourceType() === "minecraft") {
        imported = await launcherAPI.importOfficialMinecraft({
          source_dir: customPath() || officialSummary()?.path || "",
          instance_name: instanceName() || "Official Minecraft",
          game_version: gameVersion() || "1.21.1",
          loader: loader() || "vanilla",
          copy_saves: copySaves(),
          copy_resource_packs: copyResourcePacks(),
          copy_screenshots: copyScreenshots(),
          copy_mods: copyMods(),
          copy_options: copyOptions(),
          copy_servers: copyServers(),
        });
      } else {
        imported = await launcherAPI.importPrismInstance({
          source_dir: customPath(),
          instance_name: instanceName() || "Imported Prism Instance",
          copy_saves: copySaves(),
          copy_resource_packs: copyResourcePacks(),
          copy_screenshots: copyScreenshots(),
          copy_mods: copyMods(),
          copy_options: copyOptions(),
          copy_servers: copyServers(),
        });
      }

      setIsCompleted(true);
      if (props.onImported) {
        props.onImported(imported);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setImportError(`Ошибка импорта: ${msg}`);
    } finally {
      setIsImporting(false);
    }
  };

  return (
    <Show when={props.isOpen}>
      <div
        class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 animate-in fade-in duration-200"
        data-testid="import-instance-modal"
      >
        <div class="bg-zinc-950 border border-zinc-800 rounded-xl w-full max-w-lg overflow-hidden shadow-2xl flex flex-col">
          {/* Header */}
          <div class="flex items-center justify-between p-4 border-b border-zinc-800">
            <div class="flex items-center gap-2">
              <FolderDown class="w-5 h-5 text-[#00D4B2]" />
              <h2 class="text-sm font-semibold uppercase tracking-wider text-zinc-100">
                Импорт инстанса
              </h2>
            </div>
            <button
              type="button"
              onClick={handleClose}
              class="p-1 rounded text-zinc-400 hover:text-white hover:bg-zinc-800 transition-colors cursor-pointer"
              data-testid="close-import-modal-btn"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          <div class="p-6 space-y-5 flex-1 overflow-y-auto max-h-[80vh]">
            <Show when={isCompleted()}>
              <div
                class="py-8 flex flex-col items-center justify-center text-center space-y-3"
                data-testid="import-complete-view"
              >
                <div class="w-12 h-12 rounded-full bg-[#00D4B2]/10 border border-[#00D4B2]/30 flex items-center justify-center">
                  <CheckCircle2 class="w-6 h-6 text-[#00D4B2]" />
                </div>
                <h3 class="text-base font-medium text-white">Инстанс успешно импортирован</h3>
                <p class="text-xs text-zinc-400 max-w-sm">
                  Все выбранные миры, ресурспаки, моды и настройки скопированы в изолированную директорию.
                </p>
                <button
                  type="button"
                  onClick={handleClose}
                  class="mt-4 px-4 py-2 bg-[#00D4B2] hover:bg-[#00b89a] text-zinc-950 rounded text-xs font-semibold transition-colors cursor-pointer"
                  data-testid="import-done-btn"
                >
                  Готово
                </button>
              </div>
            </Show>

            <Show when={!isCompleted()}>
              {/* Source Switcher Tabs */}
              <div class="flex items-center bg-zinc-900 border border-zinc-800 rounded p-1 text-xs font-mono">
                <button
                  type="button"
                  onClick={() => handleSourceTabChange("minecraft")}
                  class={`flex-1 py-1.5 rounded transition-colors flex items-center justify-center gap-2 cursor-pointer ${
                    sourceType() === "minecraft"
                      ? "bg-[#00D4B2] text-zinc-950 font-medium"
                      : "text-zinc-400 hover:text-zinc-100"
                  }`}
                  data-testid="source-tab-minecraft"
                >
                  <HardDrive class="w-3.5 h-3.5" />
                  <span>.minecraft</span>
                </button>
                <button
                  type="button"
                  onClick={() => handleSourceTabChange("prism")}
                  class={`flex-1 py-1.5 rounded transition-colors flex items-center justify-center gap-2 cursor-pointer ${
                    sourceType() === "prism"
                      ? "bg-[#00D4B2] text-zinc-950 font-medium"
                      : "text-zinc-400 hover:text-zinc-100"
                  }`}
                  data-testid="source-tab-prism"
                >
                  <Cpu class="w-3.5 h-3.5" />
                  <span>Prism / MultiMC</span>
                </button>
              </div>

              {/* Source Directory Input */}
              <div class="space-y-1.5">
                <label class="text-xs font-mono text-zinc-400">
                  {sourceType() === "minecraft"
                    ? "Путь к папке .minecraft (оставьте пустым для автоопределения)"
                    : "Путь к папке инстанса Prism / MultiMC"}
                </label>
                <div class="flex items-center gap-2">
                  <input
                    type="text"
                    value={customPath()}
                    onInput={(e) => setCustomPath(e.currentTarget.value)}
                    placeholder={
                      sourceType() === "minecraft"
                        ? officialSummary()?.path || "%APPDATA%/.minecraft"
                        : "C:\\PrismLauncher\\instances\\MyPack"
                    }
                    class="flex-1 bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-3 py-2 text-xs text-zinc-100 placeholder-zinc-600 outline-none font-mono"
                    data-testid="import-source-path-input"
                  />
                  <button
                    type="button"
                    disabled={isScanning()}
                    onClick={() => {
                      if (sourceType() === "minecraft") {
                        scanOfficial(customPath() || undefined);
                      } else {
                        scanPrism(customPath());
                      }
                    }}
                    class="px-3 py-2 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 rounded text-xs font-mono transition-colors flex items-center gap-1.5 cursor-pointer disabled:opacity-50"
                    data-testid="scan-btn"
                  >
                    <Show when={isScanning()} fallback={<FolderOpen class="w-3.5 h-3.5 text-[#00D4B2]" />}>
                      <Loader2 class="w-3.5 h-3.5 animate-spin text-[#00D4B2]" />
                    </Show>
                    <span>Сканировать</span>
                  </button>
                </div>
              </div>

              {/* Scan Error */}
              <Show when={scanError()}>
                <div
                  class="p-3 rounded bg-red-500/10 border border-red-500/20 text-xs text-red-400 flex items-center gap-2 font-mono"
                  data-testid="import-scan-error"
                >
                  <AlertCircle class="w-4 h-4 shrink-0" />
                  <span>{scanError()}</span>
                </div>
              </Show>

              {/* Content Preview & Settings */}
              <Show when={(sourceType() === "minecraft" && officialSummary()) || (sourceType() === "prism" && prismSummary())}>
                <div class="space-y-4 pt-2 border-t border-zinc-800/80">
                  {/* Summary Card */}
                  <div class="p-3 bg-zinc-900/60 border border-zinc-800/80 rounded-lg space-y-2 text-xs font-mono">
                    <div class="text-[11px] text-zinc-400 uppercase tracking-wider font-semibold">
                      Обнаруженные компоненты
                    </div>
                    <div class="grid grid-cols-2 gap-2 text-zinc-300">
                      <div>
                        Миры:{" "}
                        <span class="text-white font-medium">
                          {sourceType() === "minecraft" ? officialSummary()?.world_count : prismSummary()?.world_count}
                        </span>
                      </div>
                      <div>
                        Ресурспаки:{" "}
                        <span class="text-white font-medium">
                          {sourceType() === "minecraft" ? officialSummary()?.resource_packs : prismSummary()?.resource_packs}
                        </span>
                      </div>
                      <div>
                        Скриншоты:{" "}
                        <span class="text-white font-medium">
                          {sourceType() === "minecraft" ? officialSummary()?.screenshots : prismSummary()?.screenshots}
                        </span>
                      </div>
                      <div>
                        Моды:{" "}
                        <span class="text-white font-medium">
                          {sourceType() === "minecraft" ? officialSummary()?.mod_count : prismSummary()?.mod_count}
                        </span>
                      </div>
                    </div>
                  </div>

                  {/* Instance Destination Options */}
                  <div class="space-y-3">
                    <div class="space-y-1">
                      <label class="text-xs font-mono text-zinc-400">Название нового инстанса</label>
                      <input
                        type="text"
                        value={instanceName()}
                        onInput={(e) => setInstanceName(e.currentTarget.value)}
                        class="w-full bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-3 py-2 text-xs text-zinc-100 outline-none font-mono"
                        data-testid="import-instance-name-input"
                      />
                    </div>

                    <div class="grid grid-cols-2 gap-3">
                      <div class="space-y-1">
                        <label class="text-xs font-mono text-zinc-400">Версия игры</label>
                        <Show
                          when={sourceType() === "minecraft" && officialSummary()?.versions && officialSummary()!.versions.length > 0}
                          fallback={
                            <input
                              type="text"
                              value={gameVersion()}
                              onInput={(e) => setGameVersion(e.currentTarget.value)}
                              class="w-full bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-3 py-2 text-xs text-zinc-100 outline-none font-mono"
                              data-testid="import-game-version-input"
                            />
                          }
                        >
                          <select
                            value={gameVersion()}
                            onChange={(e) => setGameVersion(e.currentTarget.value)}
                            class="w-full bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-3 py-2 text-xs text-zinc-100 outline-none font-mono cursor-pointer"
                            data-testid="import-game-version-select"
                          >
                            <For each={officialSummary()!.versions}>
                              {(v) => <option value={v}>{v}</option>}
                            </For>
                          </select>
                        </Show>
                      </div>

                      <div class="space-y-1">
                        <label class="text-xs font-mono text-zinc-400">Загрузчик</label>
                        <select
                          value={loader()}
                          onChange={(e) => setLoader(e.currentTarget.value)}
                          class="w-full bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-3 py-2 text-xs text-zinc-100 outline-none font-mono cursor-pointer"
                          data-testid="import-loader-select"
                        >
                          <option value="vanilla">Vanilla</option>
                          <option value="fabric">Fabric</option>
                          <option value="quilt">Quilt</option>
                          <option value="forge">Forge</option>
                          <option value="neoforge">NeoForge</option>
                        </select>
                      </div>
                    </div>

                    {/* Component Checkboxes */}
                    <div class="space-y-2 pt-2">
                      <div class="text-[11px] font-mono text-zinc-400 uppercase tracking-wider">
                        Что импортировать
                      </div>
                      <div class="grid grid-cols-2 gap-2 text-xs font-mono">
                        <label class="flex items-center gap-2 text-zinc-300 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={copySaves()}
                            onChange={(e) => setCopySaves(e.currentTarget.checked)}
                            class="rounded border-zinc-800 text-[#00D4B2] focus:ring-0"
                            data-testid="checkbox-copy-saves"
                          />
                          <span>Миры (saves)</span>
                        </label>
                        <label class="flex items-center gap-2 text-zinc-300 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={copyResourcePacks()}
                            onChange={(e) => setCopyResourcePacks(e.currentTarget.checked)}
                            class="rounded border-zinc-800 text-[#00D4B2] focus:ring-0"
                            data-testid="checkbox-copy-resourcepacks"
                          />
                          <span>Ресурспаки</span>
                        </label>
                        <label class="flex items-center gap-2 text-zinc-300 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={copyScreenshots()}
                            onChange={(e) => setCopyScreenshots(e.currentTarget.checked)}
                            class="rounded border-zinc-800 text-[#00D4B2] focus:ring-0"
                            data-testid="checkbox-copy-screenshots"
                          />
                          <span>Скриншоты</span>
                        </label>
                        <label class="flex items-center gap-2 text-zinc-300 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={copyMods()}
                            onChange={(e) => setCopyMods(e.currentTarget.checked)}
                            class="rounded border-zinc-800 text-[#00D4B2] focus:ring-0"
                            data-testid="checkbox-copy-mods"
                          />
                          <span>Модификации</span>
                        </label>
                        <label class="flex items-center gap-2 text-zinc-300 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={copyOptions()}
                            onChange={(e) => setCopyOptions(e.currentTarget.checked)}
                            class="rounded border-zinc-800 text-[#00D4B2] focus:ring-0"
                            data-testid="checkbox-copy-options"
                          />
                          <span>Настройки (options)</span>
                        </label>
                        <label class="flex items-center gap-2 text-zinc-300 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={copyServers()}
                            onChange={(e) => setCopyServers(e.currentTarget.checked)}
                            class="rounded border-zinc-800 text-[#00D4B2] focus:ring-0"
                            data-testid="checkbox-copy-servers"
                          />
                          <span>Сервера (servers)</span>
                        </label>
                      </div>
                    </div>
                  </div>
                </div>
              </Show>

              {/* Import Error */}
              <Show when={importError()}>
                <div
                  class="p-3 rounded bg-red-500/10 border border-red-500/20 text-xs text-red-400 flex items-center gap-2 font-mono"
                  data-testid="import-error-banner"
                >
                  <AlertCircle class="w-4 h-4 shrink-0" />
                  <span>{importError()}</span>
                </div>
              </Show>
            </Show>
          </div>

          {/* Footer */}
          <Show when={!isCompleted()}>
            <div class="p-4 border-t border-zinc-800 flex items-center justify-end gap-3 bg-zinc-900/30">
              <button
                type="button"
                onClick={handleClose}
                class="px-4 py-2 text-xs font-mono text-zinc-400 hover:text-white transition-colors cursor-pointer"
              >
                Отмена
              </button>
              <button
                type="button"
                disabled={isImporting() || (!officialSummary() && !prismSummary())}
                onClick={handleStartImport}
                class="px-4 py-2 bg-[#00D4B2] hover:bg-[#00b89a] text-zinc-950 font-semibold rounded text-xs transition-colors flex items-center gap-2 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                data-testid="start-import-btn"
              >
                <Show when={isImporting()} fallback={<Check class="w-4 h-4" />}>
                  <Loader2 class="w-4 h-4 animate-spin" />
                </Show>
                <span>{isImporting() ? "Импортирование..." : "Импортировать"}</span>
              </button>
            </div>
          </Show>
        </div>
      </div>
    </Show>
  );
};
