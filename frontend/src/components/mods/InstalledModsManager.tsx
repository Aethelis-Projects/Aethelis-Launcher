import { Component, createSignal, createResource, For, Show } from "solid-js";
import { Package, Trash2, Power, Search } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { InstalledModDTO } from "../../bindings/ipc_types";

interface InstalledModsManagerProps {
  instanceId: string;
}

export const InstalledModsManager: Component<InstalledModsManagerProps> = (props) => {
  const [search, setSearch] = createSignal("");
  const [mods, { refetch }] = createResource(
    () => props.instanceId,
    (id) => launcherAPI.listInstalledMods(id)
  );

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
      <div class="flex items-center justify-between border-b border-zinc-800 pb-4">
        <div>
          <h2 class="text-sm font-semibold uppercase tracking-wider text-zinc-100 flex items-center gap-2">
            <Package class="w-4 h-4 text-[#00D4B2]" />
            Установленные модификации ({mods()?.length || 0})
          </h2>
          <p class="text-xs text-zinc-400 mt-0.5">
            Управление файлами и переключение активности модов
          </p>
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
    </div>
  );
};