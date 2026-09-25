import { Component, createSignal, createResource, createEffect, on, For, Show } from "solid-js";
import { Package, Trash2, Power, Search, RefreshCw, Download, Loader2, X } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { InstalledModDTO, ModUpdateItemDTO } from "../../bindings/ipc_types";
import { ModUpdatesDiffModal } from "./ModUpdatesDiffModal";

interface InstalledModsManagerProps {
  instanceId: string;
}

export const InstalledModsManager: Component<InstalledModsManagerProps> = (props) => {
  const [search, setSearch] = createSignal("");
  const [mods, { refetch }] = createResource(
    () => props.instanceId,
    (id) => launcherAPI.listInstalledMods(id)
  );

  // Baseline tracking for session snapshot
  const [baseline, setBaseline] = createSignal<Map<string, boolean> | null>(null);
  const [updates, setUpdates] = createSignal<ModUpdateItemDTO[]>([]);
  const [checkingUpdates, setCheckingUpdates] = createSignal(false);
  const [updatingAll, setUpdatingAll] = createSignal(false);
  const [updatingFiles, setUpdatingFiles] = createSignal<Set<string>>(new Set());
  const [updateError, setUpdateError] = createSignal<string | null>(null);
  const [showDiffModal, setShowDiffModal] = createSignal(false);
  const [duplicateToast, setDuplicateToast] = createSignal<{
    files: string[];
    visible: boolean;
  } | null>(null);
  const [reconcileNoticeDismissed, setReconcileNoticeDismissed] = createSignal(false);

  const normName = (fn: string) => fn.replace(/\.disabled$/, "");

  const handleUndoDuplicateHeal = async () => {
    const toast = duplicateToast();
    if (!toast) return;
    for (const f of toast.files) {
      const disabledName = f.endsWith(".disabled") ? f : `${f}.disabled`;
      await launcherAPI.toggleMod({
        instance_id: props.instanceId,
        file_name: disabledName,
        enable: true,
      });
    }
    setReconcileNoticeDismissed(true);
    setDuplicateToast(null);
    refetch();
  };

  // Initialize baseline when mods are first loaded for an instance
  createEffect(() => {
    const list = mods();
    if (list && baseline() === null) {
      const map = new Map<string, boolean>();
      for (const m of list) {
        map.set(normName(m.file_name), m.enabled);
      }
      setBaseline(map);
    }
  });

  // ReconcileNotice: Show duplicate self-heal toast on mount if disabled duplicate files exist
  createEffect(
    on(
      mods,
      (list) => {
        if (!list || reconcileNoticeDismissed()) return;

        const activeModIds = new Set<string>();
        for (const m of list) {
          if (m.enabled && !m.file_name.endsWith(".disabled") && m.mod_id) {
            activeModIds.add(m.mod_id);
          }
        }

        const disabledDups: string[] = [];
        for (const m of list) {
          if (m.file_name.endsWith(".disabled") && m.mod_id && activeModIds.has(m.mod_id)) {
            disabledDups.push(normName(m.file_name));
          }
        }

        if (disabledDups.length > 0 && !duplicateToast()) {
          setDuplicateToast({
            files: Array.from(new Set(disabledDups)),
            visible: true,
          });
        }
      },
      { defer: false }
    )
  );

  // Reset baseline & updates when instance changes
  createEffect(
    on(
      () => props.instanceId,
      () => {
        setBaseline(null);
        setUpdates([]);
        setUpdateError(null);
        setDuplicateToast(null);
        setReconcileNoticeDismissed(false);
      }
    )
  );

  const sessionDiff = () => {
    const base = baseline();
    const curr = mods() || [];
    if (!base) {
      return { added: 0, removed: 0, toggled: 0 };
    }
    const currMap = new Map<string, boolean>();
    let added = 0;
    let toggled = 0;
    for (const m of curr) {
      const key = normName(m.file_name);
      currMap.set(key, m.enabled);
      if (!base.has(key)) {
        added++;
      } else if (base.get(key) !== m.enabled) {
        toggled++;
      }
    }
    let removed = 0;
    for (const [key] of base) {
      if (!currMap.has(key)) {
        removed++;
      }
    }
    return { added, removed, toggled };
  };

  const handleCheckUpdates = async () => {
    setCheckingUpdates(true);
    setUpdateError(null);
    try {
      const res = await launcherAPI.checkModUpdates(props.instanceId);
      setUpdates(res || []);
    } catch (err: unknown) {
      setUpdateError(err instanceof Error ? err.message : "Ошибка проверки обновлений");
    } finally {
      setCheckingUpdates(false);
    }
  };

  const handleUpdateMod = async (update: ModUpdateItemDTO) => {
    setUpdatingFiles((prev) => new Set([...prev, update.file_name]));
    setUpdateError(null);
    try {
      const res = await launcherAPI.updateMod({
        instance_id: props.instanceId,
        mod_id: update.mod_id,
        old_file_name: update.file_name,
        source: update.source,
        target_version_id: update.latest_version_id,
      });
      if (res.disabled_duplicates && res.disabled_duplicates.length > 0) {
        setDuplicateToast({
          files: res.disabled_duplicates,
          visible: true,
        });
      }
      setUpdates((prev) => prev.filter((u) => u.file_name !== update.file_name));
      refetch();
    } catch (err: unknown) {
      setUpdateError(err instanceof Error ? err.message : "Ошибка обновления мода");
    } finally {
      setUpdatingFiles((prev) => {
        const next = new Set(prev);
        next.delete(update.file_name);
        return next;
      });
    }
  };

  const handleUpdateAll = async () => {
    const list = [...updates()];
    if (list.length === 0) return;
    setUpdatingAll(true);
    setUpdateError(null);
    const allDisabled: string[] = [];
    try {
      for (const u of list) {
        const res = await launcherAPI.updateMod({
          instance_id: props.instanceId,
          mod_id: u.mod_id,
          old_file_name: u.file_name,
          source: u.source,
          target_version_id: u.latest_version_id,
        });
        if (res.disabled_duplicates && res.disabled_duplicates.length > 0) {
          allDisabled.push(...res.disabled_duplicates);
        }
      }
      if (allDisabled.length > 0) {
        setDuplicateToast({
          files: Array.from(new Set(allDisabled)),
          visible: true,
        });
      }
      setUpdates([]);
      refetch();
    } catch (err: unknown) {
      setUpdateError(err instanceof Error ? err.message : "Ошибка при обновлении всех модов");
    } finally {
      setUpdatingAll(false);
    }
  };

  const filteredMods = () => {
    const list = mods() || [];
    const q = search().toLowerCase().trim();
    if (!q) return list;
    return list.filter(
      (m) =>
        m.name.toLowerCase().includes(q) ||
        m.file_name.toLowerCase().includes(q)
    );
  };

  const handleToggle = async (mod: InstalledModDTO) => {
    await launcherAPI.toggleMod({
      instance_id: props.instanceId,
      file_name: mod.file_name,
      enable: !mod.enabled,
    });
    refetch();
  };

  const handleDelete = async (mod: InstalledModDTO) => {
    if (confirm(`Удалить модификацию ${mod.name}?`)) {
      await launcherAPI.deleteMod({
        instance_id: props.instanceId,
        file_name: mod.file_name,
      });
      refetch();
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  return (
    <div class="space-y-4">
      <div class="flex flex-col gap-3 border-b border-zinc-800 pb-4">
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-sm font-semibold uppercase tracking-wider text-zinc-100 flex items-center gap-2">
              <Package class="w-4 h-4 text-[#00D4B2]" />
              Установленные модификации ({mods()?.length || 0})
            </h2>
            <p class="text-xs text-zinc-400 mt-0.5">
              Управление файлами и переключение активности модов
            </p>
          </div>

          <div
            class="text-[11px] font-mono px-2.5 py-1 rounded border border-zinc-700 bg-zinc-800/80 text-zinc-300"
            data-testid="session-diff"
          >
            Изменено в этой сессии: (+{sessionDiff().added}, -{sessionDiff().removed}, ~{sessionDiff().toggled})
          </div>
        </div>

        <div class="flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <button
              type="button"
              onClick={handleCheckUpdates}
              disabled={checkingUpdates()}
              data-testid="check-updates-btn"
              class="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium rounded border border-zinc-700 bg-zinc-800 text-zinc-200 hover:border-[#00D4B2]/50 hover:text-white transition-colors disabled:opacity-50"
            >
              <Show when={checkingUpdates()} fallback={<RefreshCw class="w-3.5 h-3.5 text-[#00D4B2]" />}>
                <Loader2 class="w-3.5 h-3.5 text-[#00D4B2] animate-spin" />
              </Show>
              <span>{checkingUpdates() ? "Проверка..." : "Проверить обновления"}</span>
            </button>

            <Show when={updates().length > 0}>
              <div class="flex items-center gap-2" data-testid="updates-banner">
                <button
                  type="button"
                  onClick={() => setShowDiffModal(true)}
                  data-testid="updates-diff-chip-btn"
                  title="Посмотреть подробности обновлений"
                  class="text-xs font-mono px-2 py-0.5 rounded bg-[#00D4B2]/10 border border-[#00D4B2]/40 text-[#00D4B2] font-semibold hover:bg-[#00D4B2]/20 hover:border-[#00D4B2] transition-colors cursor-pointer"
                >
                  {updates().length} обновлений доступно
                </button>
                <button
                  type="button"
                  onClick={handleUpdateAll}
                  disabled={updatingAll()}
                  data-testid="update-all-btn"
                  class="flex items-center gap-1 px-2.5 py-1 text-xs font-semibold rounded bg-[#00D4B2] text-zinc-950 hover:bg-[#00D4B2]/90 transition-colors disabled:opacity-50"
                >
                  <Show when={updatingAll()} fallback={<Download class="w-3.5 h-3.5" />}>
                    <Loader2 class="w-3.5 h-3.5 animate-spin" />
                  </Show>
                  <span>{updatingAll() ? "Обновление..." : "Обновить все"}</span>
                </button>
              </div>
            </Show>
          </div>

          <div class="relative w-64">
            <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-500" />
            <input
              type="text"
              value={search()}
              onInput={(e) => setSearch(e.currentTarget.value)}
              placeholder="Фильтр модов..."
              class="w-full bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-8 py-1 text-xs text-zinc-100 placeholder-zinc-500 outline-none font-mono"
            />
          </div>
        </div>

        <Show when={updateError()}>
          <div class="px-3 py-1.5 rounded border border-red-500/30 bg-red-500/10 text-red-400 text-xs font-mono flex items-center justify-between">
            <span>{updateError()}</span>
            <button type="button" onClick={() => setUpdateError(null)} class="text-zinc-400 hover:text-zinc-200">
              <X class="w-3.5 h-3.5" />
            </button>
          </div>
        </Show>

        <Show when={duplicateToast()?.visible}>
          <div
            class="px-3 py-2 rounded border border-amber-500/30 bg-amber-500/10 text-amber-300 text-xs font-mono flex items-center justify-between gap-3"
            data-testid="duplicate-heal-toast"
          >
            <div class="flex items-center gap-2">
              <Package class="w-4 h-4 text-amber-400 shrink-0" />
              <span>
                Отключены устаревшие дубликаты ({duplicateToast()?.files.length}):{" "}
                <span class="text-amber-200 font-semibold">
                  {duplicateToast()?.files.join(", ")}
                </span>
              </span>
            </div>
            <div class="flex items-center gap-2">
              <button
                type="button"
                onClick={handleUndoDuplicateHeal}
                data-testid="undo-duplicate-heal-btn"
                class="px-2 py-0.5 rounded bg-amber-500/20 hover:bg-amber-500/30 text-amber-200 border border-amber-500/40 text-xs font-sans font-medium transition-colors"
              >
                Отменить
              </button>
              <button
                type="button"
                onClick={() => {
                  setDuplicateToast(null);
                  setReconcileNoticeDismissed(true);
                }}
                class="text-amber-400 hover:text-amber-200 p-0.5"
                title="Скрыть"
              >
                <X class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </Show>
      </div>

      <div class="border border-zinc-800 rounded divide-y divide-zinc-800/80 bg-zinc-900/40">
        <Show when={filteredMods().length === 0}>
          <div class="text-center py-10 text-zinc-500 text-xs font-mono">
            {mods()?.length === 0
              ? "В этом инстансе пока нет установленных модов."
              : "Нет модов, соответствующих фильтру."}
          </div>
        </Show>

        <For each={filteredMods()}>
          {(mod) => (
            <div class="flex items-center justify-between p-3 hover:bg-zinc-800/30 transition-colors">
              <div class="flex items-center gap-3 min-w-0">
                <button
                  type="button"
                  onClick={() => handleToggle(mod)}
                  title={mod.enabled ? "Отключить мод" : "Включить мод"}
                  class={`w-7 h-7 rounded border flex items-center justify-center transition-colors shrink-0 ${
                    mod.enabled
                      ? "bg-[#00D4B2]/10 border-[#00D4B2]/40 text-[#00D4B2] hover:bg-[#00D4B2]/20"
                      : "bg-zinc-800 border-zinc-700 text-zinc-500 hover:text-zinc-300"
                  }`}
                >
                  <Power class="w-3.5 h-3.5" />
                </button>

                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <span
                      class={`text-xs font-medium truncate ${
                        mod.enabled ? "text-zinc-100" : "text-zinc-500 line-through"
                      }`}
                    >
                      {mod.name}
                    </span>
                    <Show when={mod.version}>
                      <span class="text-[10px] font-mono text-zinc-500">
                        v{mod.version}
                      </span>
                    </Show>
                    <Show when={mod.source}>
                      <span class="text-[10px] font-mono px-1.5 py-0.5 rounded border bg-zinc-800/80 border-zinc-700 text-zinc-400 capitalize">
                        {mod.source}
                      </span>
                    </Show>
                    <Show when={mod.release_type}>
                      <span
                        class={`text-[10px] font-mono px-1.5 py-0.5 rounded border uppercase ${
                          mod.release_type === "release"
                            ? "bg-emerald-500/10 border-emerald-500/30 text-emerald-400"
                            : mod.release_type === "beta"
                            ? "bg-blue-500/10 border-blue-500/30 text-blue-400"
                            : "bg-amber-500/10 border-amber-500/30 text-amber-400"
                        }`}
                      >
                        {mod.release_type}
                      </span>
                    </Show>
                  </div>
                  <div class="text-[10px] font-mono text-zinc-500 truncate mt-0.5">
                    {mod.file_name} • {formatBytes(mod.size_bytes)}
                  </div>
                </div>
              </div>

              <div class="flex items-center gap-3 shrink-0">
                <Show when={updates().find((u) => u.file_name === mod.file_name || (Boolean(mod.mod_id) && u.mod_id === mod.mod_id))}>
                  {(update) => {
                    const isUpdating = () => updatingFiles().has(update().file_name);
                    return (
                      <div class="flex items-center gap-1.5" data-testid={`update-badge-${mod.file_name}`}>
                        <span class="text-[10px] font-mono px-1.5 py-0.5 rounded border bg-[#00D4B2]/10 border-[#00D4B2]/40 text-[#00D4B2]">
                          Обновление: v{update().latest_version}
                        </span>
                        <button
                          type="button"
                          onClick={() => handleUpdateMod(update())}
                          disabled={isUpdating()}
                          title="Обновить мод"
                          class="flex items-center gap-1 px-2 py-0.5 text-[10px] font-semibold rounded bg-[#00D4B2] text-zinc-950 hover:bg-[#00D4B2]/90 transition-colors disabled:opacity-50"
                        >
                          <Show when={isUpdating()} fallback={<Download class="w-3 h-3" />}>
                            <Loader2 class="w-3 h-3 animate-spin" />
                          </Show>
                          <span>{isUpdating() ? "..." : "Обновить"}</span>
                        </button>
                      </div>
                    );
                  }}
                </Show>

                <span
                  class={`text-[10px] font-mono px-2 py-0.5 rounded border ${
                    mod.enabled
                      ? "bg-[#00D4B2]/10 border-[#00D4B2]/30 text-[#00D4B2]"
                      : "bg-zinc-800 border-zinc-700 text-zinc-500"
                  }`}
                >
                  {mod.enabled ? "Активен" : "Отключен"}
                </span>

                <button
                  type="button"
                  onClick={() => handleDelete(mod)}
                  title="Удалить файл"
                  class="p-1.5 text-zinc-500 hover:text-red-400 hover:bg-red-500/10 rounded transition-colors"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
          )}
        </For>
      </div>

      <ModUpdatesDiffModal
        isOpen={showDiffModal()}
        onClose={() => setShowDiffModal(false)}
        installedMods={mods() || []}
        updates={updates()}
        onUpdateMod={handleUpdateMod}
        onUpdateAll={handleUpdateAll}
        updatingFiles={updatingFiles()}
        updatingAll={updatingAll()}
      />
    </div>
  );
};