import { Component, createSignal, createResource, For, Show, onMount, onCleanup } from "solid-js";
import { Download, Search, Check, Loader2, Layers, Globe, AlertCircle, ChevronDown, AlertTriangle } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { ModItemDTO, ModSource, ModFileDTO, ModInstallProgressDTO } from "../../bindings/ipc_types";

interface ModCatalogProps {
  activeInstanceId: string;
  gameVersion: string;
  loader: string;
  onModInstalled?: (mod: ModItemDTO) => void;
}

const CATEGORIES = [
  { id: "", label: "Все категории" },
  { id: "optimization", label: "Оптимизация" },
  { id: "technology", label: "Технологии" },
  { id: "magic", label: "Магия" },
  { id: "utility", label: "Утилиты" },
  { id: "adventure", label: "Приключения" },
  { id: "decoration", label: "Декорации" },
];

const MODRINTH_SORTS = [
  { id: "relevance", label: "По релевантности" },
  { id: "downloads", label: "По загрузкам" },
  { id: "updated", label: "По обновлению" },
  { id: "newest", label: "Новые" },
];

const CURSEFORGE_SORTS = [
  { id: "relevance", label: "По релевантности" },
  { id: "popularity", label: "По популярности" },
  { id: "updated", label: "По обновлению" },
  { id: "downloads", label: "По загрузкам" },
];

export const ModCatalog: Component<ModCatalogProps> = (props) => {
  const [query, setQuery] = createSignal("");
  const [source, setSource] = createSignal<ModSource>("modrinth");
  const [selectedCategory, setSelectedCategory] = createSignal("");
  const [selectedSort, setSelectedSort] = createSignal("relevance");
  const [installingId, setInstallingId] = createSignal<string | null>(null);
  const [installProgress, setInstallProgress] = createSignal<ModInstallProgressDTO | null>(null);
  const [installedIds, setInstalledIds] = createSignal<Set<string>>(new Set());
  const [searchError, setSearchError] = createSignal<string>("");
  const [searchReason, setSearchReason] = createSignal<string>("");
  const [rateLimitCountdown, setRateLimitCountdown] = createSignal<number>(0);
  const [installError, setInstallError] = createSignal<string>("");
  const [hasBuiltinKey, setHasBuiltinKey] = createSignal(false);
  const [totalCount, setTotalCount] = createSignal(0);

  // Versions dropdown state
  const [expandedModId, setExpandedModId] = createSignal<string | null>(null);
  const [loadingVersions, setLoadingVersions] = createSignal(false);
  const [modVersions, setModVersions] = createSignal<Record<string, ModFileDTO[]>>({});

  let pollTimer: ReturnType<typeof setInterval> | null = null;
  let countdownTimer: ReturnType<typeof setInterval> | null = null;

  const sortOptions = () => (source() === "curseforge" ? CURSEFORGE_SORTS : MODRINTH_SORTS);

  const switchSource = (newSource: ModSource) => {
    if (source() === newSource) return;
    if (countdownTimer) {
      clearInterval(countdownTimer);
      countdownTimer = null;
    }
    setRateLimitCountdown(0);
    setSearchReason("");
    setSearchError("");

    const validSorts = newSource === "curseforge" ? CURSEFORGE_SORTS : MODRINTH_SORTS;
    if (!validSorts.some((s) => s.id === selectedSort())) {
      setSelectedSort("relevance");
    }
    setSource(newSource);
  };

  onCleanup(() => {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    if (countdownTimer) {
      clearInterval(countdownTimer);
      countdownTimer = null;
    }
  });

  onMount(async () => {
    try {
      const builtin = await launcherAPI.hasBuiltinCurseForgeKey();
      setHasBuiltinKey(builtin);
    } catch (_err: unknown) {
      // Safe fallback
    }
  });

  const [mods, { refetch }] = createResource(
    () => ({
      q: query(),
      s: source(),
      gv: props.gameVersion,
      l: props.loader,
      sort: selectedSort(),
      category: selectedCategory(),
    }),
    async ({ q, s, gv, l, sort, category }) => {
      setSearchError("");
      setSearchReason("");
      if (countdownTimer) {
        clearInterval(countdownTimer);
        countdownTimer = null;
      }
      setRateLimitCountdown(0);

      try {
        const res = await launcherAPI.searchMods({
          query: q,
          source: s,
          game_version: gv,
          loader: l,
          sort: sort,
          category: category || undefined,
          limit: 20,
          offset: 0,
        });

        if (res.reason) {
          setSearchReason(res.reason);
          setTotalCount(0);
          if (res.reason === "rate_limited") {
            const wait = res.retry_after_seconds && res.retry_after_seconds > 0 ? res.retry_after_seconds : 5;
            setRateLimitCountdown(wait);
            countdownTimer = setInterval(() => {
              setRateLimitCountdown((prev) => {
                if (prev <= 1) {
                  if (countdownTimer) {
                    clearInterval(countdownTimer);
                    countdownTimer = null;
                  }
                  refetch();
                  return 0;
                }
                return prev - 1;
              });
            }, 1000);
          }
          return [];
        }

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

  const toggleVersions = async (mod: ModItemDTO) => {
    if (expandedModId() === mod.id) {
      setExpandedModId(null);
      return;
    }
    setExpandedModId(mod.id);
    if (!modVersions()[mod.id]) {
      setLoadingVersions(true);
      try {
        const files = await launcherAPI.listModVersions({
          mod_id: mod.id,
          source: source(),
          game_version: props.gameVersion,
          loader: props.loader,
        });
        setModVersions((prev) => ({ ...prev, [mod.id]: files }));
      } catch (_err) {
        // non-fatal
      } finally {
        setLoadingVersions(false);
      }
    }
  };

  const handleInstall = async (mod: ModItemDTO, versionId?: string) => {
    setInstallingId(mod.id);
    setInstallError("");
    setInstallProgress({
      task_id: `install-${mod.id}`,
      instance_id: props.activeInstanceId,
      mod_id: mod.id,
      file_name: "",
      status: "resolving_dependencies",
      bytes_read: 0,
      total_bytes: 0,
      percentage: 10,
    });

    if (pollTimer) clearInterval(pollTimer);
    pollTimer = setInterval(async () => {
      try {
        const status = await launcherAPI.getModInstallStatus(props.activeInstanceId);
        if (status) {
          setInstallProgress(status);
        }
      } catch (_err) {
        // quiet poll error
      }
    }, 1000);

    try {
      if (versionId) {
        await launcherAPI.installMod(props.activeInstanceId, mod, versionId);
      } else {
        await launcherAPI.installMod(props.activeInstanceId, mod);
      }
      setInstalledIds((prev) => new Set([...prev, mod.id]));
      if (props.onModInstalled) {
        props.onModInstalled(mod);
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setInstallError(msg || "Failed to install mod");
    } finally {
      if (pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
      setInstallingId(null);
      setInstallProgress(null);
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (!bytes) return "0 B";
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
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
            onClick={() => switchSource("modrinth")}
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
            onClick={() => switchSource("curseforge")}
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

      {/* Category Chips Bar */}
      <div class="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs">
        <For each={CATEGORIES}>
          {(cat) => (
            <button
              type="button"
              onClick={() => setSelectedCategory(cat.id)}
              class={`px-2.5 py-1 rounded-full border text-xs whitespace-nowrap transition-colors cursor-pointer ${
                selectedCategory() === cat.id
                  ? "bg-[#00D4B2]/15 text-[#00D4B2] border-[#00D4B2]/40 font-medium"
                  : "bg-zinc-900 border-zinc-800 text-zinc-400 hover:text-zinc-200 hover:border-zinc-700"
              }`}
              data-testid={`category-chip-${cat.id || "all"}`}
            >
              {cat.label}
            </button>
          )}
        </For>
      </div>

      {/* Search Bar & Sort Dropdown */}
      <div class="flex items-center gap-3">
        <div class="relative flex-1">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-zinc-500" />
          <input
            type="text"
            value={query()}
            onInput={(e) => setQuery(e.currentTarget.value)}
            placeholder={`Поиск в ${source() === "modrinth" ? "Modrinth" : "CurseForge"}...`}
            class="w-full bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-9 py-2 text-xs text-zinc-100 placeholder-zinc-500 outline-none transition-colors"
          />
        </div>

        <select
          value={selectedSort()}
          onChange={(e) => setSelectedSort(e.currentTarget.value)}
          class="bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-3 py-2 text-xs text-zinc-300 outline-none cursor-pointer"
          data-testid="mods-sort-select"
        >
          <For each={sortOptions()}>
            {(s) => <option value={s.id}>{s.label}</option>}
          </For>
        </select>
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

        <Show when={!mods.loading && searchReason() === "rate_limited"}>
          <div
            class="p-4 rounded bg-amber-500/10 border border-amber-500/20 text-xs text-amber-300 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 font-mono"
            data-testid="mods-rate-limit-banner"
          >
            <div class="flex items-center gap-2">
              <AlertTriangle class="w-4 h-4 text-amber-400 shrink-0" />
              <span>
                CurseForge ограничил частоту запросов — повторим через {rateLimitCountdown()} с
              </span>
            </div>
            <Show when={source() === "curseforge"}>
              <button
                type="button"
                onClick={() => switchSource("modrinth")}
                class="px-3 py-1 bg-amber-400/20 hover:bg-amber-400/30 text-amber-200 border border-amber-400/30 rounded text-xs transition-colors cursor-pointer whitespace-nowrap"
                data-testid="fallback-to-modrinth-btn"
              >
                Искать это же на Modrinth
              </button>
            </Show>
          </div>
        </Show>

        <Show when={!mods.loading && searchReason() === "key_invalid"}>
          <div
            class="p-4 rounded bg-red-500/10 border border-red-500/20 text-xs text-red-400 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 font-mono"
            data-testid="mods-key-invalid-banner"
          >
            <div class="flex items-center gap-2">
              <AlertCircle class="w-4 h-4 text-red-400 shrink-0" />
              <span>
                Ключ каталога отклонён сервером — это внутренняя проблема, обновите лаунчер
              </span>
            </div>
            <Show when={source() === "curseforge"}>
              <button
                type="button"
                onClick={() => switchSource("modrinth")}
                class="px-3 py-1 bg-red-400/20 hover:bg-red-400/30 text-red-200 border border-red-400/30 rounded text-xs transition-colors cursor-pointer whitespace-nowrap"
                data-testid="fallback-to-modrinth-btn"
              >
                Искать это же на Modrinth
              </button>
            </Show>
          </div>
        </Show>

        <Show when={!mods.loading && searchReason() === "unreachable"}>
          <div
            class="p-4 rounded bg-red-500/10 border border-red-500/20 text-xs text-red-400 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 font-mono"
            data-testid="mods-unreachable-banner"
          >
            <div class="flex items-center gap-2">
              <AlertCircle class="w-4 h-4 text-red-400 shrink-0" />
              <span>
                Сервер каталога недоступен — проверьте подключение к сети
              </span>
            </div>
            <Show when={source() === "curseforge"}>
              <button
                type="button"
                onClick={() => switchSource("modrinth")}
                class="px-3 py-1 bg-red-400/20 hover:bg-red-400/30 text-red-200 border border-red-400/30 rounded text-xs transition-colors cursor-pointer whitespace-nowrap"
                data-testid="fallback-to-modrinth-btn"
              >
                Искать это же на Modrinth
              </button>
            </Show>
          </div>
        </Show>

        <Show when={!mods.loading && !searchReason() && searchError()}>
          <div
            class="p-4 rounded bg-red-500/10 border border-red-500/20 text-xs text-red-400 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 font-mono"
            data-testid="mods-search-error-banner"
          >
            <div class="flex items-center gap-2">
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
            <Show when={source() === "curseforge"}>
              <button
                type="button"
                onClick={() => switchSource("modrinth")}
                class="px-3 py-1 bg-red-400/20 hover:bg-red-400/30 text-red-200 border border-red-400/30 rounded text-xs transition-colors cursor-pointer whitespace-nowrap"
                data-testid="fallback-to-modrinth-btn"
              >
                Искать это же на Modrinth
              </button>
            </Show>
          </div>
        </Show>

        <Show when={!mods.loading && !searchReason() && !searchError() && (!mods() || mods()!.length === 0)}>
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
            const isExpanded = () => expandedModId() === mod.id;
            const files = () => modVersions()[mod.id] || [];

            return (
              <div class="bg-zinc-900/60 border border-zinc-800/80 hover:border-zinc-700 rounded p-3.5 transition-colors">
                <div class="flex items-start justify-between gap-4">
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

                  <div class="shrink-0 flex items-center gap-2 pt-0.5">
                    {/* Versions toggle button */}
                    <button
                      type="button"
                      onClick={() => toggleVersions(mod)}
                      class="px-2.5 py-1.5 rounded text-xs border border-zinc-800 hover:border-zinc-700 text-zinc-300 hover:text-white bg-zinc-800/40 transition-colors flex items-center gap-1 cursor-pointer"
                      title="Выбрать версию"
                      data-testid={`mod-versions-toggle-${mod.id}`}
                    >
                      <ChevronDown class={`w-3.5 h-3.5 transition-transform ${isExpanded() ? "rotate-180" : ""}`} />
                      <span>Версии</span>
                    </button>

                    {/* Primary auto-install button */}
                    <button
                      type="button"
                      disabled={isInstalling() || isInstalled()}
                      onClick={() => handleInstall(mod)}
                      class={`px-3 py-1.5 rounded text-xs font-medium transition-all flex items-center gap-1.5 ${
                        isInstalled()
                          ? "bg-zinc-800 text-zinc-400 cursor-default"
                          : isInstalling()
                          ? "bg-zinc-800 text-zinc-300"
                          : "bg-[#00D4B2] hover:bg-[#00b89a] text-zinc-950 active:scale-95 cursor-pointer"
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

                {/* Progress bar during install */}
                <Show when={isInstalling() && installProgress()}>
                  <div class="mt-3 pt-3 border-t border-zinc-800" data-testid="mod-install-progress-bar">
                    <div class="flex items-center justify-between text-[11px] font-mono text-zinc-400 mb-1">
                      <span class="capitalize">{installProgress()?.status}...</span>
                      <span>{installProgress()?.percentage}%</span>
                    </div>
                    <div class="w-full bg-zinc-800 rounded-full h-1.5 overflow-hidden">
                      <div
                        class="bg-[#00D4B2] h-full transition-all duration-300 rounded-full"
                        style={{ width: `${installProgress()?.percentage || 0}%` }}
                      />
                    </div>
                  </div>
                </Show>

                {/* Versions Dropdown Drawer */}
                <Show when={isExpanded()}>
                  <div class="mt-3 pt-3 border-t border-zinc-800 space-y-2">
                    <div class="flex items-center justify-between text-xs text-zinc-400 font-medium">
                      <span>Доступные файлы ({files().length})</span>
                      <Show when={loadingVersions()}>
                        <span class="flex items-center gap-1 text-[11px] text-[#00D4B2] font-mono">
                          <Loader2 class="w-3 h-3 animate-spin" /> Загрузка версий...
                        </span>
                      </Show>
                    </div>

                    <div class="max-h-60 overflow-y-auto space-y-1.5 divide-y divide-zinc-800/40">
                      <For each={files()}>
                        {(file) => {
                          const isMatch = file.game_versions.includes(props.gameVersion) &&
                            file.loaders.some((l) => l.toLowerCase() === props.loader.toLowerCase());

                          return (
                            <div class="flex items-center justify-between pt-1.5 text-xs">
                              <div class="flex items-center gap-2 min-w-0">
                                {/* Release type badge */}
                                <span
                                  class={`text-[10px] font-mono px-1.5 py-0.5 rounded border uppercase ${
                                    file.release_type === "release"
                                      ? "bg-emerald-500/10 border-emerald-500/30 text-emerald-400"
                                      : file.release_type === "beta"
                                      ? "bg-blue-500/10 border-blue-500/30 text-blue-400"
                                      : "bg-amber-500/10 border-amber-500/30 text-amber-400"
                                  }`}
                                >
                                  {file.release_type}
                                </span>

                                <span class="font-mono text-zinc-200 truncate">
                                  {file.display_name || file.file_name}
                                </span>

                                <span class="text-[10px] font-mono text-zinc-500 shrink-0">
                                  {formatFileSize(file.file_size)}
                                </span>

                                {/* Fallback warning badge */}
                                <Show when={!isMatch}>
                                  <span
                                    class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-amber-500/10 border border-amber-500/30 text-amber-300 flex items-center gap-1 shrink-0"
                                    data-testid="fallback-warning-badge"
                                  >
                                    <AlertTriangle class="w-3 h-3 text-amber-400" />
                                    не проверено под {props.gameVersion}
                                  </span>
                                </Show>
                              </div>

                              <button
                                type="button"
                                disabled={isInstalling()}
                                onClick={() => handleInstall(mod, file.id)}
                                class="px-2 py-1 rounded text-[11px] font-medium bg-zinc-800 hover:bg-[#00D4B2] hover:text-zinc-950 text-zinc-300 transition-colors cursor-pointer shrink-0 ml-2"
                                data-testid={`install-version-${file.id}`}
                              >
                                Установить
                              </button>
                            </div>
                          );
                        }}
                      </For>
                    </div>
                  </div>
                </Show>
              </div>
            );
          }}
        </For>
      </div>
    </div>
  );
};