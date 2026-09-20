import { Component, createSignal, createResource, For, Show, onMount } from "solid-js";
import { Download, Search, Check, Loader2, Layers, Globe, AlertCircle } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { ModItemDTO, ModSource } from "../../bindings/ipc_types";

interface ModCatalogProps {
  activeInstanceId: string;
  gameVersion: string;
  loader: string;
  onModInstalled?: (mod: ModItemDTO) => void;
}

export const ModCatalog: Component<ModCatalogProps> = (props) => {
  const [query, setQuery] = createSignal("");
  const [source, setSource] = createSignal<ModSource>("modrinth");
  const [installingId, setInstallingId] = createSignal<string | null>(null);
  const [installedIds, setInstalledIds] = createSignal<Set<string>>(new Set());
  const [searchError, setSearchError] = createSignal<string>("");
  const [installError, setInstallError] = createSignal<string>("");
  const [hasBuiltinKey, setHasBuiltinKey] = createSignal(false);
  const [totalCount, setTotalCount] = createSignal(0);

  onMount(async () => {
    try {
      const builtin = await launcherAPI.hasBuiltinCurseForgeKey();
      setHasBuiltinKey(builtin);
    } catch (_err: unknown) {
      // Safe fallback
    }
  });

  const [mods] = createResource(
    () => ({ q: query(), s: source(), gv: props.gameVersion, l: props.loader }),
    async ({ q, s, gv, l }) => {
      setSearchError("");
      try {
        const res = await launcherAPI.searchMods({
          query: q,
          source: s,
          game_version: gv,
          loader: l,
          limit: 20,
          offset: 0,
        });
        setTotalCount(res.total_count);
        return res.items;
      } catch (err: unknown) {
        const msg = err instanceof Error ? err.message : String(err);
        setSearchError(msg || "Failed to search mods");
        setTotalCount(0);
        return [];
      }
    }
  );

  const handleInstall = async (mod: ModItemDTO) => {
    setInstallingId(mod.id);
    setInstallError("");
    try {
      await launcherAPI.installMod(props.activeInstanceId, mod);
      setInstalledIds((prev) => new Set([...prev, mod.id]));
      if (props.onModInstalled) {
        props.onModInstalled(mod);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setInstallError(msg || "Failed to install mod");
    } finally {
      setInstallingId(null);
    }
  };

  return (
    <div class="space-y-4">
      {/* Header & Source switcher */}
      <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 border-b border-zinc-800 pb-4">
        <div>
          <h2 class="text-sm font-semibold uppercase tracking-wider text-zinc-100 flex items-center gap-2">
            <Layers class="w-4 h-4 text-[#00D4B2]" />
            Каталог модификаций
          </h2>
          <p class="text-xs text-zinc-400 mt-0.5">
            Поиск проверенных модификаций для {props.loader} {props.gameVersion}
            <Show when={totalCount() > 0}>
              <span class="ml-1 text-zinc-500 font-mono">({totalCount()})</span>
            </Show>
          </p>
        </div>

        {/* Source Toggle Tabs */}
        <div class="flex items-center bg-zinc-900 border border-zinc-800 rounded p-0.5 text-xs font-mono">
          <button
            type="button"
            onClick={() => setSource("modrinth")}
            class={`px-3 py-1 rounded transition-colors flex items-center gap-1.5 ${
              source() === "modrinth"
                ? "bg-[#00D4B2] text-zinc-950 font-medium"
                : "text-zinc-400 hover:text-zinc-100"
            }`}
          >
            <Globe class="w-3 h-3" />
            Modrinth
          </button>
          <button
            type="button"
            onClick={() => setSource("curseforge")}
            class={`px-3 py-1 rounded transition-colors flex items-center gap-1.5 ${
              source() === "curseforge"
                ? "bg-[#00D4B2] text-zinc-950 font-medium"
                : "text-zinc-400 hover:text-zinc-100"
            }`}
          >
            <Globe class="w-3 h-3" />
            CurseForge
          </button>
        </div>
      </div>

      {/* Search Bar */}
      <div class="relative">
        <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
        <input
          type="text"
          value={query()}
          onInput={(e) => setQuery(e.currentTarget.value)}
          placeholder={`Поиск в ${source() === "modrinth" ? "Modrinth" : "CurseForge"}...`}
          class="w-full bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-9 py-2 text-xs text-zinc-100 placeholder-zinc-500 outline-none transition-colors"
        />
      </div>

      <Show when={installError()}>
        <div
          class="p-3 rounded bg-amber-500/10 border border-amber-500/20 text-xs text-amber-300 flex items-center justify-between gap-2 font-mono"
          data-testid="mods-install-error-banner"
        >
          <div class="flex items-center gap-2">
            <AlertCircle class="w-4 h-4 text-amber-400 shrink-0" />
            <span>{installError()}</span>
          </div>
          <button
            type="button"
            onClick={() => setInstallError("")}
            class="text-[10px] text-zinc-400 hover:text-zinc-200 underline cursor-pointer"
          >
            Закрыть
          </button>
        </div>
      </Show>

      {/* Mod Cards List */}
      <div class="space-y-2">
        <Show when={mods.loading}>
          <div
            class="flex items-center justify-center py-12 text-zinc-400 gap-2 text-xs font-mono"
            data-testid="mods-loading-indicator"
          >
            <Loader2 class="w-4 h-4 animate-spin text-[#00D4B2]" />
            Поиск модификаций...
          </div>
        </Show>

        <Show when={!mods.loading && searchError()}>
          <div
            class="p-4 rounded bg-red-500/10 border border-red-500/20 text-xs text-red-400 flex items-center gap-2 font-mono"
            data-testid="mods-search-error-banner"
          >
            <AlertCircle class="w-4 h-4 text-red-400 shrink-0" />
            <Show
              when={searchError().includes("CF_RATE_LIMITED:") || searchError().includes("401") || searchError().includes("403")}
              fallback={<span>{`Ошибка поиска модов: ${searchError()}`}</span>}
            >
              <span>
                {hasBuiltinKey()
                  ? "CurseForge временно ограничил запросы — попробуйте позже"
                  : import.meta.env.DEV
                  ? "Встроенный ключ каталога временно недоступен. [DEV] Проверьте cf.key или настройте в Dev Settings."
                  : "Встроенный ключ каталога временно недоступен"}
              </span>
            </Show>
          </div>
        </Show>

        <Show when={!mods.loading && !searchError() && (!mods() || mods()!.length === 0)}>
          <div
            class="text-center py-12 border border-dashed border-zinc-800 rounded text-xs text-zinc-500"
            data-testid="mods-empty-state"
          >
            Модификации по запросу не найдены.
          </div>
        </Show>

        <For each={mods()}>
          {(mod) => {
            const isInstalling = () => installingId() === mod.id;
            const isInstalled = () => installedIds().has(mod.id);

            return (
              <div class="bg-zinc-900/60 border border-zinc-800/80 hover:border-zinc-700 rounded p-3.5 transition-colors flex items-start justify-between gap-4">
                <div class="flex items-start gap-3 min-w-0">
                  <div class="w-10 h-10 rounded bg-zinc-800 border border-zinc-700/60 flex items-center justify-center shrink-0 overflow-hidden">
                    <Show
                      when={mod.icon_url}
                      fallback={<Layers class="w-5 h-5 text-zinc-500" />}
                    >
                      <img
                        src={mod.icon_url}
                        alt={mod.name}
                        class="w-full h-full object-cover"
                        onError={(e) => {
                          e.currentTarget.style.display = "none";
                        }}
                      />
                    </Show>
                  </div>

                  <div class="min-w-0">
                    <div class="flex items-center gap-2">
                      <h3 class="text-xs font-medium text-zinc-100 truncate">
                        {mod.name}
                      </h3>
                      <span class="text-[10px] font-mono text-zinc-500">
                        от {mod.author}
                      </span>
                    </div>
                    <p class="text-xs text-zinc-400 mt-1 line-clamp-2 leading-relaxed">
                      {mod.summary}
                    </p>
                    <div class="flex items-center gap-2 mt-2">
                      <span class="text-[10px] font-mono text-zinc-500 flex items-center gap-1">
                        <Download class="w-3 h-3" />
                        {mod.downloads.toLocaleString()}
                      </span>
                      <For each={mod.categories.slice(0, 3)}>
                        {(cat) => (
                          <span class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-zinc-800/60 text-zinc-400 border border-zinc-800">
                            {cat}
                          </span>
                        )}
                      </For>
                    </div>
                  </div>
                </div>

                <div class="shrink-0 pt-0.5">
                  <button
                    type="button"
                    disabled={isInstalling() || isInstalled()}
                    onClick={() => handleInstall(mod)}
                    class={`px-3 py-1.5 rounded text-xs font-medium transition-all flex items-center gap-1.5 ${
                      isInstalled()
                        ? "bg-zinc-800 text-zinc-400 cursor-default"
                        : isInstalling()
                        ? "bg-zinc-800 text-zinc-300"
                        : "bg-[#00D4B2] hover:bg-[#00b89a] text-zinc-950 active:scale-95"
                    }`}
                  >
                    <Show when={isInstalling()}>
                      <Loader2 class="w-3.5 h-3.5 animate-spin" />
                      Загрузка...
                    </Show>
                    <Show when={isInstalled() && !isInstalling()}>
                      <Check class="w-3.5 h-3.5 text-[#00D4B2]" />
                      Установлен
                    </Show>
                    <Show when={!isInstalled() && !isInstalling()}>
                      <Download class="w-3.5 h-3.5" />
                      Установить
                    </Show>
                  </button>
                </div>
              </div>
            );
          }}
        </For>
      </div>
    </div>
  );
};