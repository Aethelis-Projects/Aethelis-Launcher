import { Component, createSignal, createEffect, For, Show } from "solid-js";
import { FolderOpen, RefreshCw, Trash2, Database, AlertCircle, Info, Check } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { WorldDTO, DatapackDTO } from "../../bindings/ipc_types";

interface DatapackManagerProps {
  instanceId: string;
}

export const DatapackManager: Component<DatapackManagerProps> = (props) => {
  const [worlds, setWorlds] = createSignal<WorldDTO[]>([]);
  const [selectedWorld, setSelectedWorld] = createSignal<string>("");
  const [datapacks, setDatapacks] = createSignal<DatapackDTO[]>([]);
  const [loadingWorlds, setLoadingWorlds] = createSignal(true);
  const [loadingDatapacks, setLoadingDatapacks] = createSignal(false);
  const [actionError, setActionError] = createSignal("");
  const [actionSuccess, setActionSuccess] = createSignal("");
  const [togglingFile, setTogglingFile] = createSignal<string | null>(null);
  const [deletingFile, setDeletingFile] = createSignal<string | null>(null);

  const loadWorlds = async () => {
    setLoadingWorlds(true);
    setActionError("");
    try {
      const list = await launcherAPI.listInstanceWorlds(props.instanceId);
      setWorlds(list);
      if (list.length > 0 && (!selectedWorld() || !list.some((w) => w.name === selectedWorld()))) {
        setSelectedWorld(list[0].name);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setActionError(`Ошибка загрузки миров: ${msg}`);
    } finally {
      setLoadingWorlds(false);
    }
  };

  const loadDatapacks = async (worldName: string) => {
    if (!worldName) {
      setDatapacks([]);
      return;
    }
    setLoadingDatapacks(true);
    setActionError("");
    try {
      const list = await launcherAPI.listWorldDatapacks(props.instanceId, worldName);
      setDatapacks(list);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setActionError(`Ошибка загрузки датапаков: ${msg}`);
    } finally {
      setLoadingDatapacks(false);
    }
  };

  createEffect(() => {
    loadWorlds();
  });

  createEffect(() => {
    const w = selectedWorld();
    if (w) {
      loadDatapacks(w);
    }
  });

  const handleToggle = async (dp: DatapackDTO) => {
    const w = selectedWorld();
    if (!w) return;
    setTogglingFile(dp.file_name);
    setActionError("");
    setActionSuccess("");
    try {
      await launcherAPI.setDatapackEnabled({
        instance_id: props.instanceId,
        world_name: w,
        file_name: dp.file_name,
        enabled: !dp.enabled,
      });
      await loadDatapacks(w);
      await loadWorlds();
      setActionSuccess(`Датапак ${dp.name} ${!dp.enabled ? "включен" : "отключен"}`);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setActionError(`Не удалось изменить статус датапака: ${msg}`);
    } finally {
      setTogglingFile(null);
    }
  };

  const handleDelete = async (dp: DatapackDTO) => {
    const w = selectedWorld();
    if (!w) return;
    setDeletingFile(dp.file_name);
    setActionError("");
    setActionSuccess("");
    try {
      await launcherAPI.deleteDatapack({
        instance_id: props.instanceId,
        world_name: w,
        file_name: dp.file_name,
      });
      await loadDatapacks(w);
      await loadWorlds();
      setActionSuccess(`Датапак ${dp.name} удален`);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setActionError(`Не удалось удалить датапак: ${msg}`);
    } finally {
      setDeletingFile(null);
    }
  };

  const handleOpenFolder = async () => {
    const w = selectedWorld();
    setActionError("");
    try {
      const folderRel = w ? `saves/${w}/datapacks` : "datapacks";
      const targetDir = await launcherAPI.ensureInstanceDir(props.instanceId, folderRel);
      await launcherAPI.openPath(targetDir);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setActionError(`Не удалось открыть папку: ${msg}`);
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (!bytes) return "0 B";
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const formatLastPlayed = (ts: number): string => {
    if (!ts) return "Не играли";
    try {
      return new Date(ts).toLocaleString("ru-RU", {
        day: "2-digit",
        month: "2-digit",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {
      return "Неизвестно";
    }
  };

  return (
    <div class="space-y-4" data-testid="datapack-manager">
      {/* Notifications */}
      <Show when={actionError()}>
        <div
          class="p-3 rounded-lg bg-nord-rose/10 border border-nord-rose/20 text-xs text-nord-rose flex items-center gap-2 font-mono"
          data-testid="datapack-error-banner"
        >
          <AlertCircle class="w-4 h-4 shrink-0" />
          <span>{actionError()}</span>
        </div>
      </Show>

      <Show when={actionSuccess()}>
        <div
          class="p-3 rounded-lg bg-nord-cyan/10 border border-nord-cyan/20 text-xs text-nord-cyan flex items-center gap-2 font-mono"
          data-testid="datapack-success-banner"
        >
          <Check class="w-4 h-4 shrink-0" />
          <span>{actionSuccess()}</span>
        </div>
      </Show>

      {/* Guidance Alert Banner */}
      <div
        class="p-3 rounded-lg bg-zinc-900 border border-zinc-800 text-xs text-zinc-300 flex items-center justify-between gap-3"
        data-testid="datapack-reload-hint"
      >
        <div class="flex items-center gap-2">
          <Info class="w-4 h-4 text-nord-cyan shrink-0" />
          <span>
            Датапаки работают индивидуально для каждого мира. Для применения изменений в запущенной игре выполните команду <code class="px-1 py-0.5 bg-zinc-800 rounded font-mono text-[#00D4B2]">/reload</code>.
          </span>
        </div>
        <button
          type="button"
          onClick={handleOpenFolder}
          class="px-2.5 py-1 rounded bg-zinc-800 hover:bg-zinc-700 text-zinc-200 text-xs flex items-center gap-1.5 transition-colors cursor-pointer shrink-0"
          data-testid="datapack-open-folder-btn"
          title="Открыть папку датапаков на диске"
        >
          <FolderOpen class="w-3.5 h-3.5 text-nord-cyan" />
          <span>Папка</span>
        </button>
      </div>

      {/* World Selector and Refresh Header */}
      <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 bg-zinc-900/60 p-3 rounded-lg border border-zinc-800/80">
        <div class="flex items-center gap-3 w-full sm:w-auto">
          <label class="text-xs text-zinc-400 font-mono shrink-0 flex items-center gap-1.5">
            <Database class="w-3.5 h-3.5 text-nord-cyan" />
            <span>Мир:</span>
          </label>
          <Show
            when={!loadingWorlds()}
            fallback={<span class="text-xs text-zinc-500 font-mono">Загрузка миров...</span>}
          >
            <Show
              when={worlds().length > 0}
              fallback={<span class="text-xs text-zinc-400 font-mono italic">Миры не обнаружены</span>}
            >
              <select
                value={selectedWorld()}
                onChange={(e) => setSelectedWorld(e.currentTarget.value)}
                class="bg-zinc-950 border border-zinc-700 rounded px-2.5 py-1 text-xs text-zinc-200 font-mono focus:border-nord-cyan focus:outline-none cursor-pointer max-w-xs"
                data-testid="datapack-world-select"
              >
                <For each={worlds()}>
                  {(w) => (
                    <option value={w.name}>
                      {w.display_name} ({w.datapack_count} датапак., {formatLastPlayed(w.last_played)})
                    </option>
                  )}
                </For>
              </select>
            </Show>
          </Show>
        </div>

        <div class="flex items-center gap-2 self-end sm:self-center">
          <button
            type="button"
            onClick={() => {
              loadWorlds();
              if (selectedWorld()) {
                loadDatapacks(selectedWorld());
              }
            }}
            class="p-1.5 rounded bg-zinc-800 hover:bg-zinc-700 text-zinc-300 transition-colors cursor-pointer"
            data-testid="datapack-refresh-btn"
            title="Обновить список"
          >
            <RefreshCw class={`w-3.5 h-3.5 ${loadingWorlds() || loadingDatapacks() ? "animate-spin" : ""}`} />
          </button>
        </div>
      </div>

      {/* Main Datapacks List */}
      <Show
        when={worlds().length > 0}
        fallback={
          <div class="py-12 px-4 text-center rounded-xl bg-zinc-900/40 border border-zinc-800/60" data-testid="datapack-no-worlds">
            <Database class="w-10 h-10 text-zinc-600 mx-auto mb-3" />
            <h4 class="text-sm font-semibold text-zinc-300">В этом инстансе пока нет сохраненных миров</h4>
            <p class="text-xs text-zinc-400 mt-1 max-w-md mx-auto">
              Запустите игру и создайте мир, чтобы управлять его датапаками. Вы также можете установить датапаки заранее через каталог модификаций.
            </p>
          </div>
        }
      >
        <Show
          when={!loadingDatapacks()}
          fallback={<div class="py-8 text-center text-xs text-zinc-500 font-mono">Загрузка датапаков...</div>}
        >
          <Show
            when={datapacks().length > 0}
            fallback={
              <div class="py-8 px-4 text-center rounded-xl bg-zinc-900/40 border border-zinc-800/60" data-testid="datapack-empty-list">
                <Database class="w-8 h-8 text-zinc-600 mx-auto mb-2" />
                <h4 class="text-xs font-medium text-zinc-300">В выбранном мире нет установленных датапаков</h4>
                <p class="text-xs text-zinc-400 mt-0.5">
                  Перейдите во вкладку Каталог и выберите категорию «Датапаки» для установки.
                </p>
              </div>
            }
          >
            <div class="border border-zinc-800 rounded-lg overflow-hidden divide-y divide-zinc-800/60 bg-zinc-950/40" data-testid="datapack-list">
              <For each={datapacks()}>
                {(dp) => (
                  <div
                    class={`p-3 flex items-center justify-between gap-3 transition-colors ${
                      dp.enabled ? "bg-zinc-900/30" : "bg-zinc-950/60 opacity-70"
                    }`}
                    data-testid={`datapack-item-${dp.file_name}`}
                  >
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-2">
                        <span class="text-xs font-semibold text-zinc-200 truncate" title={dp.name}>
                          {dp.name}
                        </span>
                        <span
                          class={`px-1.5 py-0.2 rounded text-[10px] font-mono uppercase ${
                            dp.enabled
                              ? "bg-nord-cyan/15 text-nord-cyan border border-nord-cyan/30"
                              : "bg-zinc-800 text-zinc-400"
                          }`}
                          data-testid={`datapack-status-${dp.file_name}`}
                        >
                          {dp.enabled ? "Включен" : "Отключен"}
                        </span>
                      </div>
                      <div class="flex items-center gap-3 text-[11px] text-zinc-400 font-mono mt-0.5">
                        <span>{dp.file_name}</span>
                        <span>{formatFileSize(dp.size_bytes)}</span>
                      </div>
                    </div>

                    <div class="flex items-center gap-2 shrink-0">
                      {/* Toggle button */}
                      <button
                        type="button"
                        onClick={() => handleToggle(dp)}
                        disabled={togglingFile() === dp.file_name}
                        class={`px-2.5 py-1 rounded text-xs font-mono transition-colors cursor-pointer ${
                          dp.enabled
                            ? "bg-zinc-800 hover:bg-zinc-700 text-zinc-200"
                            : "bg-[#00D4B2] hover:bg-[#00D4B2]/80 text-zinc-950 font-medium"
                        }`}
                        data-testid={`datapack-toggle-${dp.file_name}`}
                      >
                        {dp.enabled ? "Отключить" : "Включить"}
                      </button>

                      {/* Delete button */}
                      <button
                        type="button"
                        onClick={() => handleDelete(dp)}
                        disabled={deletingFile() === dp.file_name}
                        class="p-1.5 rounded bg-zinc-900 hover:bg-nord-rose/20 text-zinc-400 hover:text-nord-rose transition-colors cursor-pointer"
                        data-testid={`datapack-delete-${dp.file_name}`}
                        title="Удалить датапак"
                      >
                        <Trash2 class="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Show>
      </Show>
    </div>
  );
};
