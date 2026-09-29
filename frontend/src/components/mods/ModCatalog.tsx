import { Component, createSignal, createResource, For, Show, onMount, onCleanup } from "solid-js";
import { Download, Search, Check, Loader2, Layers, Globe, AlertCircle, ChevronDown, AlertTriangle, FileText, X, Database } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { ModItemDTO, ModSource, ModFileDTO, ModInstallProgressDTO, ProjectType, WorldDTO } from "../../bindings/ipc_types";
import { renderMarkdownLite } from "../common/MarkdownLite";

interface ModCatalogProps {
  activeInstanceId: string;
  gameVersion?: string;
  loader?: string;
  onModInstalled?: (mod: ModItemDTO) => void;
}

interface CatalogFiltersState {
  project_type?: ProjectType;
  category?: string;
  loader?: string;
  game_version?: string;
  sort?: string;
  search_query?: string;
}

const loadSavedFilters = (): CatalogFiltersState => {
  try {
    const raw = typeof localStorage !== "undefined" ? localStorage.getItem("nord_catalog_filters") : null;
    if (raw) return JSON.parse(raw);
  } catch (_e) {
    void _e;
  }
  return {};
};

const savedFilters = loadSavedFilters();

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
  const initialProjectType: ProjectType =
    savedFilters.project_type === "resourcepack" || savedFilters.project_type === "shader" || savedFilters.project_type === "datapack"
      ? savedFilters.project_type
      : "mod";

  const [projectType, setProjectType] = createSignal<ProjectType>(initialProjectType);
  const [query, setQuery] = createSignal(savedFilters.search_query || "");
  const [source, setSource] = createSignal<ModSource>("modrinth");
  const [selectedCategory, setSelectedCategory] = createSignal(savedFilters.category || "");
  const [selectedSort, setSelectedSort] = createSignal(savedFilters.sort || "relevance");
  const [installingId, setInstallingId] = createSignal<string | null>(null);
  const [installProgress, setInstallProgress] = createSignal<ModInstallProgressDTO | null>(null);
  const [installedIds, setInstalledIds] = createSignal<Set<string>>(new Set());
  const [searchError, setSearchError] = createSignal<string>("");
  const [searchReason, setSearchReason] = createSignal<string>("");
  const [rateLimitCountdown, setRateLimitCountdown] = createSignal<number>(0);
  const [installError, setInstallError] = createSignal<string>("");
  const [hasBuiltinKey, setHasBuiltinKey] = createSignal(false);
  const [totalCount, setTotalCount] = createSignal(0);

  // v0.7.2 G4: catalog owns its filters; instance context only seeds them.
  const [gvc, setGvc] = createSignal(savedFilters.game_version || props.gameVersion || "");
  const [loaderFilter, setLoaderFilter] = createSignal(savedFilters.loader || props.loader || "");
  const [projectTags, setProjectTags] = createSignal<{ id: string; label: string }[]>([]);

  // Versions dropdown state
  const [expandedModId, setExpandedModId] = createSignal<string | null>(null);
  const [loadingVersions, setLoadingVersions] = createSignal(false);
  const [modVersions, setModVersions] = createSignal<Record<string, ModFileDTO[]>>({});
  const [expandedChangelogIds, setExpandedChangelogIds] = createSignal<Set<string>>(new Set());

  const toggleChangelog = (fileId: string) => {
    setExpandedChangelogIds((prev) => {
      const next = new Set(prev);
      if (next.has(fileId)) {
        next.delete(fileId);
      } else {
        next.add(fileId);
      }
      return next;
    });
  };

  let pollTimer: ReturnType<typeof setInterval> | null = null;
  let countdownTimer: ReturnType<typeof setInterval> | null = null;
  let querySaveTimeout: ReturnType<typeof setTimeout> | null = null;

  const saveFilters = (delta?: Partial<CatalogFiltersState>) => {
    try {
      if (typeof localStorage === "undefined") return;
      const state: CatalogFiltersState = {
        project_type: delta?.project_type ?? projectType(),
        category: delta?.category ?? selectedCategory(),
        sort: delta?.sort ?? selectedSort(),
        search_query: delta?.search_query ?? query(),
      };
      localStorage.setItem("nord_catalog_filters", JSON.stringify(state));
    } catch (_e) {
      void _e;
    }
  };

  const handleProjectTypeChange = (type: ProjectType) => {
    if (projectType() === type) return;
    setProjectType(type);
    saveFilters({ project_type: type });
    refreshTags();
  };

  const handleLoaderFilterChange = (value: string) => {
    setLoaderFilter(value);
    saveFilters({ loader: value });
  };

  const handleCategoryChange = (catId: string) => {
    setSelectedCategory(catId);
    saveFilters({ category: catId });
  };

  const handleSortChange = (sortId: string) => {
    setSelectedSort(sortId);
    saveFilters({ sort: sortId });
  };

  const handleQueryInput = (val: string) => {
    setQuery(val);
    if (querySaveTimeout) clearTimeout(querySaveTimeout);
    querySaveTimeout = setTimeout(() => {
      saveFilters({ search_query: val });
    }, 300);
  };

  const sortOptions = () => (source() === "curseforge" ? CURSEFORGE_SORTS : MODRINTH_SORTS);

  const switchSource = (newSource: ModSource) => {
    if (projectType() !== "mod" && newSource !== "modrinth") return;
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
    refreshTags();
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
    if (querySaveTimeout) {
      clearTimeout(querySaveTimeout);
      querySaveTimeout = null;
    }
  });

  onMount(async () => {
    try {
      const builtin = await launcherAPI.hasBuiltinCurseForgeKey();
      setHasBuiltinKey(builtin);
    } catch (_err: unknown) {
      // Safe fallback
    }
    refreshTags();
  });

  const refreshTags = async () => {
    try {
      const tags = await launcherAPI.listProjectTags({
        provider: source(),
        project_type: projectType(),
      });
      // CF labels are matched client-side by name; Modrinth filters by tag id.
      setProjectTags(tags.map((t) => ({ id: source() === "curseforge" ? t.label : t.id, label: t.label })));
    } catch (_err: unknown) {
      void _err;
      setProjectTags([]); // offline etc: curated fallback below
    }
  };

  const categoryList = () =>
    projectTags().length > 0
      ? projectTags().map((t) => ({ id: t.id, label: t.label }))
      : [{ id: "", label: "Все категории" }, ...CATEGORIES.filter((c) => c.id !== "").map((c) => ({ id: c.id, label: c.label }))];

  const [mods, { refetch }] = createResource(
    () => ({
      pt: projectType(),
      q: query(),
      s: source(),
      gv: gvc(),
      l: loaderFilter(),
      sort: selectedSort(),
      category: selectedCategory(),
    }),
    async ({ pt, q, s, gv, l, sort, category }) => {
      setSearchError("");
      setSearchReason("");
      if (countdownTimer) {
        clearInterval(countdownTimer);
        countdownTimer = null;
      }
      setRateLimitCountdown(0);

      // Honest gate (v0.7.2 G4): CF public search has no datapack class.
      if (s === "curseforge" && pt === "datapack") {
        setTotalCount(0);
        return [];
      }

      try {
        const res = await launcherAPI.searchMods({
          project_type: pt,
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
          game_version: gvc(),
          loader: loaderFilter(),
        });
        setModVersions((prev) => ({ ...prev, [mod.id]: files }));
      } catch (_err) {
        // non-fatal
      } finally {
        setLoadingVersions(false);
      }
    }
  };

  // Datapack world selection state
  const [datapackInstallTarget, setDatapackInstallTarget] = createSignal<{ mod: ModItemDTO; versionId?: string } | null>(null);
  const [datapackWorlds, setDatapackWorlds] = createSignal<WorldDTO[]>([]);
  const [selectedWorldNames, setSelectedWorldNames] = createSignal<string[]>([]);
  const [loadingWorldsForInstall, setLoadingWorldsForInstall] = createSignal(false);
  const [datapackInstallError, setDatapackInstallError] = createSignal("");
  const [installingDatapack, setInstallingDatapack] = createSignal(false);

  const openDatapackInstallModal = async (mod: ModItemDTO, versionId?: string) => {
    setDatapackInstallTarget({ mod, versionId });
    setDatapackInstallError("");
    setLoadingWorldsForInstall(true);
    try {
      const list = await launcherAPI.listInstanceWorlds(props.activeInstanceId);
      setDatapackWorlds(list);
      setSelectedWorldNames(list.map((w) => w.name));
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setDatapackInstallError(msg || "Failed to load worlds");
    } finally {
      setLoadingWorldsForInstall(false);
    }
  };

  const closeDatapackInstallModal = () => {
    if (installingDatapack()) return;
    setDatapackInstallTarget(null);
    setDatapackWorlds([]);
    setSelectedWorldNames([]);
    setDatapackInstallError("");
  };

  const toggleWorldSelection = (worldName: string) => {
    setSelectedWorldNames((prev) =>
      prev.includes(worldName) ? prev.filter((w) => w !== worldName) : [...prev, worldName]
    );
  };

  const selectAllWorlds = () => {
    setSelectedWorldNames(datapackWorlds().map((w) => w.name));
  };

  const deselectAllWorlds = () => {
    setSelectedWorldNames([]);
  };

  const confirmDatapackInstall = async (unassigned = false) => {
    const target = datapackInstallTarget();
    if (!target) return;
    setInstallingDatapack(true);
    setDatapackInstallError("");
    try {
      const targetWorlds = unassigned ? [] : selectedWorldNames();
      await launcherAPI.installDatapack({
        instance_id: props.activeInstanceId,
        world_names: targetWorlds,
        mod_id: target.mod.id,
        version_id: target.versionId,
      });
      setInstalledIds((prev) => new Set([...prev, target.mod.id]));
      if (props.onModInstalled) {
        props.onModInstalled(target.mod);
      }
      closeDatapackInstallModal();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setDatapackInstallError(msg || "Failed to install datapack");
    } finally {
      setInstallingDatapack(false);
    }
  };

  const handleInstall = async (mod: ModItemDTO, versionId?: string) => {
    if (projectType() === "datapack") {
      openDatapackInstallModal(mod, versionId);
      return;
    }
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
      if (versionId || projectType() !== "mod") {
        await launcherAPI.installMod(props.activeInstanceId, mod, versionId, projectType());
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
      {/* Header, Type Switcher & Source switcher */}
      <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 border-b border-zinc-800 pb-4">
        <div>
          <h2 class="text-sm font-semibold uppercase tracking-wider text-zinc-100 flex items-center gap-2">
            <Layers class="w-4 h-4 text-[#00D4B2]" />
            Каталог модификаций
          </h2>
          <p class="text-xs text-zinc-400 mt-0.5">
            Поиск {projectType() === "resourcepack" ? "ресурспаков" : projectType() === "shader" ? "шейдеров" : projectType() === "datapack" ? "датапаков" : "проверенных модификаций"} для {loaderFilter() || "всех лоадеров"} {gvc() || ""}
            <Show when={totalCount() > 0}>
              <span class="ml-1 text-zinc-500 font-mono">({totalCount()})</span>
            </Show>
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          {/* Project Type Segment Tabs */}
          <div class="flex items-center bg-zinc-900 border border-zinc-800 rounded p-0.5 text-xs font-mono" data-testid="project-type-tabs">
            <button
              type="button"
              onClick={() => handleProjectTypeChange("mod")}
              class={`px-3 py-1 rounded transition-colors cursor-pointer ${
                projectType() === "mod"
                  ? "bg-[#00D4B2] text-zinc-950 font-medium"
                  : "text-zinc-400 hover:text-zinc-100"
              }`}
              data-testid="project-type-mod"
            >
              Моды
            </button>
            <button
              type="button"
              onClick={() => handleProjectTypeChange("resourcepack")}
              class={`px-3 py-1 rounded transition-colors cursor-pointer ${
                projectType() === "resourcepack"
                  ? "bg-[#00D4B2] text-zinc-950 font-medium"
                  : "text-zinc-400 hover:text-zinc-100"
              }`}
              data-testid="project-type-resourcepack"
            >
              Ресурспаки
            </button>
            <button
              type="button"
              onClick={() => handleProjectTypeChange("shader")}
              class={`px-3 py-1 rounded transition-colors cursor-pointer ${
                projectType() === "shader"
                  ? "bg-[#00D4B2] text-zinc-950 font-medium"
                  : "text-zinc-400 hover:text-zinc-100"
              }`}
              data-testid="project-type-shader"
            >
              Шейдеры
            </button>
            <button
              type="button"
              onClick={() => handleProjectTypeChange("datapack")}
              class={`px-3 py-1 rounded transition-colors cursor-pointer ${
                projectType() === "datapack"
                  ? "bg-[#00D4B2] text-zinc-950 font-medium"
                  : "text-zinc-400 hover:text-zinc-100"
              }`}
              data-testid="project-type-datapack"
            >
              Датапаки
            </button>
          </div>

          {/* Source Toggle Tabs or Only Modrinth Badge */}
          <Show
            when={!(projectType() === "datapack" && source() === "curseforge")}
            fallback={
              <div
                class="flex items-center gap-1.5 px-3 py-1 rounded bg-zinc-900 border border-zinc-800 text-xs font-mono text-[#00D4B2]"
                data-testid="only-modrinth-badge"
              >
                <Globe class="w-3 h-3" />
                <span>Только Modrinth</span>
              </div>
            }
          >
            <div class="flex items-center bg-zinc-900 border border-zinc-800 rounded p-0.5 text-xs font-mono">
              <button
                type="button"
                onClick={() => switchSource("modrinth")}
                class={`px-3 py-1 rounded transition-colors flex items-center gap-1.5 cursor-pointer ${
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
                class={`px-3 py-1 rounded transition-colors flex items-center gap-1.5 cursor-pointer ${
                  source() === "curseforge"
                    ? "bg-[#00D4B2] text-zinc-950 font-medium"
                    : "text-zinc-400 hover:text-zinc-100"
                }`}
              >
                <Globe class="w-3 h-3" />
                CurseForge
              </button>
            </div>
          </Show>
        </div>
      </div>

      {/* Loader Filter + Category Chips Bar (v0.7.2 G4) */}
      <div class="flex items-center gap-2 flex-wrap text-xs">
        <select
          value={loaderFilter()}
          onChange={(e) => handleLoaderFilterChange(e.currentTarget.value)}
          class="px-2 py-1 rounded-md bg-zinc-900 border border-zinc-800 text-zinc-300 text-xs focus:outline-none focus:border-[#00D4B2] cursor-pointer"
          data-testid="catalog-loader-filter"
          title="Фильтр по лоадеру (серверный для Modrinth и CurseForge)"
        >
          <option value="">Все лоадеры</option>
          <option value="forge">Forge</option>
          <option value="fabric">Fabric</option>
          <option value="quilt">Quilt</option>
          <option value="neoforge">NeoForge</option>
        </select>
        <input
          type="text"
          value={gvc()}
          onInput={(e) => {
            setGvc(e.currentTarget.value.trim());
            saveFilters({ game_version: e.currentTarget.value.trim() });
          }}
          placeholder="Версия игры (напр. 1.21.1)"
          class="px-2 py-1 w-44 rounded-md bg-zinc-900 border border-zinc-800 text-zinc-300 text-xs font-mono focus:outline-none focus:border-[#00D4B2]"
          data-testid="catalog-version-filter"
          title="Фильтр по версии игры (серверный для Modrinth и CurseForge)"
        />
        <Show when={source() === "curseforge"}>
          <span
            class="px-2 py-1 rounded-md bg-zinc-900/80 border border-zinc-800 text-zinc-500 text-[10px] font-mono"
            data-testid="catalog-cf-tags-note"
          >
            Теги CF — справочные (поиск по имени); фильтры версии/лоадера — серверные
          </span>
        </Show>
        <Show when={source() === "curseforge" && projectType() === "datapack"}>
          <div
            class="w-full p-3 rounded-lg bg-zinc-900 border border-zinc-800 text-xs text-zinc-400 flex items-center justify-between gap-3"
            data-testid="catalog-cf-datapack-gate"
          >
            <span>CurseForge не отдаёт датапаки через публичный API поиска — для датапаков доступен Modrinth.</span>
            <button
              type="button"
              onClick={() => switchSource("modrinth")}
              class="px-3 py-1 rounded-md bg-[#00D4B2]/15 hover:bg-[#00D4B2]/25 border border-[#00D4B2]/40 text-[#00D4B2] text-[11px] font-semibold whitespace-nowrap transition-colors cursor-pointer"
              data-testid="catalog-cf-datapack-switch"
            >
              Искать датапаки на Modrinth
            </button>
          </div>
        </Show>
      </div>

      <div class="flex items-center gap-1.5 overflow-x-auto pb-1 text-xs">
        <For each={categoryList()}>
          {(cat) => (
            <button
              type="button"
              onClick={() => handleCategoryChange(cat.id)}
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
            onInput={(e) => handleQueryInput(e.currentTarget.value)}
            placeholder={`Поиск в ${source() === "modrinth" ? "Modrinth" : "CurseForge"}...`}
            class="w-full bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-9 py-2 text-xs text-zinc-100 placeholder-zinc-500 outline-none transition-colors"
          />
        </div>

        <select
          value={selectedSort()}
          onChange={(e) => handleSortChange(e.currentTarget.value)}
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
                          const isMatch =
                            (!!gvc() && file.game_versions.includes(gvc())) &&
                            (!!loaderFilter() && file.loaders.some((l) => l.toLowerCase() === loaderFilter().toLowerCase()));

                          return (
                            <div class="py-2 border-b border-zinc-800/40 last:border-b-0 space-y-1.5 text-xs">
                              <div class="flex items-center justify-between">
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
                                      не проверено под {gvc() || "эту версию"}
                                    </span>
                                  </Show>
                                </div>

                                <div class="flex items-center gap-1.5 shrink-0 ml-2">
                                  {/* Changelog Toggle (M2) */}
                                  <Show when={file.changelog}>
                                    <button
                                      type="button"
                                      onClick={() => toggleChangelog(file.id)}
                                      class="px-2 py-1 rounded text-[11px] font-mono text-zinc-400 hover:text-zinc-200 bg-white/5 hover:bg-white/10 flex items-center gap-1 transition-colors cursor-pointer"
                                      title="История изменений"
                                      data-testid={`toggle-changelog-${file.id}`}
                                    >
                                      <FileText class="w-3 h-3 text-[#00D4B2]" />
                                      <span>Изменения</span>
                                      <ChevronDown class={`w-2.5 h-2.5 transition-transform ${expandedChangelogIds().has(file.id) ? "rotate-180" : ""}`} />
                                    </button>
                                  </Show>

                                  <button
                                    type="button"
                                    disabled={isInstalling()}
                                    onClick={() => handleInstall(mod, file.id)}
                                    class="px-2 py-1 rounded text-[11px] font-medium bg-zinc-800 hover:bg-[#00D4B2] hover:text-zinc-950 text-zinc-300 transition-colors cursor-pointer"
                                    data-testid={`install-version-${file.id}`}
                                  >
                                    Установить
                                  </button>
                                </div>
                              </div>

                              {/* Expandable Changelog Viewer (M2, G2) */}
                              <Show when={expandedChangelogIds().has(file.id) && file.changelog}>
                                <div
                                  class="p-2.5 rounded-lg bg-black/40 border border-white/5 text-[11px] font-mono text-zinc-300 leading-relaxed max-h-40 overflow-y-auto"
                                  data-testid={`changelog-viewer-${file.id}`}
                                >
                                  {renderMarkdownLite(file.changelog)}
                                </div>
                              </Show>
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

      {/* Datapack Install-to-World Modal */}
      <Show when={datapackInstallTarget()}>
        <div
          class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm"
          data-testid="datapack-install-modal"
        >
          <div class="w-full max-w-md bg-nord-surface border border-white/10 rounded-xl shadow-2xl p-5 space-y-4">
            <div class="flex items-center justify-between border-b border-white/5 pb-3">
              <div class="flex items-center gap-2">
                <Database class="w-5 h-5 text-nord-cyan" />
                <h3 class="text-sm font-semibold text-zinc-100">Установка датапака</h3>
              </div>
              <button
                type="button"
                onClick={closeDatapackInstallModal}
                class="p-1 rounded text-zinc-400 hover:text-zinc-200 cursor-pointer"
                data-testid="datapack-modal-close-btn"
              >
                <X class="w-4 h-4" />
              </button>
            </div>

            <div>
              <p class="text-xs text-zinc-300">
                Выберите миры, в которые необходимо установить датапак <strong class="text-white">{datapackInstallTarget()?.mod.name}</strong>:
              </p>
            </div>

            <Show when={datapackInstallError()}>
              <div class="p-2.5 rounded bg-nord-rose/10 border border-nord-rose/20 text-xs text-nord-rose flex items-center gap-2 font-mono">
                <AlertCircle class="w-4 h-4 shrink-0" />
                <span>{datapackInstallError()}</span>
              </div>
            </Show>

            <Show
              when={!loadingWorldsForInstall()}
              fallback={<div class="py-6 text-center text-xs text-zinc-500 font-mono">Загрузка миров...</div>}
            >
              <Show
                when={datapackWorlds().length > 0}
                fallback={
                  <div class="p-4 rounded-lg bg-zinc-900 border border-zinc-800 text-xs text-zinc-300 space-y-2">
                    <p>
                      В этом инстансе пока нет миров. Датапак будет сохранен в папку датапаков инстанса (<code class="text-nord-cyan font-mono">datapacks/</code>).
                    </p>
                    <p class="text-zinc-400">
                      При создании нового мира вы сможете скопировать его в мир.
                    </p>
                  </div>
                }
              >
                <div class="flex items-center justify-between text-xs text-zinc-400">
                  <span>Доступные миры:</span>
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={selectAllWorlds}
                      class="text-nord-cyan hover:underline cursor-pointer"
                    >
                      Выбрать все
                    </button>
                    <span>|</span>
                    <button
                      type="button"
                      onClick={deselectAllWorlds}
                      class="text-zinc-400 hover:text-zinc-200 cursor-pointer"
                    >
                      Снять выбор
                    </button>
                  </div>
                </div>

                <div class="space-y-1.5 max-h-48 overflow-y-auto pr-1">
                  <For each={datapackWorlds()}>
                    {(w) => {
                      const isChecked = () => selectedWorldNames().includes(w.name);
                      return (
                        <label
                          class={`flex items-center justify-between p-2.5 rounded-lg border text-xs cursor-pointer transition-colors ${
                            isChecked()
                              ? "bg-zinc-800/80 border-nord-cyan/40 text-zinc-100"
                              : "bg-zinc-900/60 border-zinc-800 text-zinc-400 hover:bg-zinc-900"
                          }`}
                        >
                          <div class="flex items-center gap-2.5">
                            <input
                              type="checkbox"
                              checked={isChecked()}
                              onChange={() => toggleWorldSelection(w.name)}
                              class="rounded border-zinc-700 text-nord-cyan focus:ring-0 cursor-pointer"
                              data-testid={`datapack-world-checkbox-${w.name}`}
                            />
                            <span class="font-medium text-zinc-200">{w.display_name}</span>
                          </div>
                          <span class="text-[11px] font-mono text-zinc-500">
                            {w.datapack_count} датапак.
                          </span>
                        </label>
                      );
                    }}
                  </For>
                </div>
              </Show>
            </Show>

            <div class="flex items-center justify-end gap-2 pt-2 border-t border-white/5">
              <Show when={datapackWorlds().length > 0}>
                <button
                  type="button"
                  onClick={() => confirmDatapackInstall(true)}
                  disabled={installingDatapack()}
                  class="px-3 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-zinc-300 text-xs font-mono transition-colors cursor-pointer"
                  data-testid="datapack-install-unassigned-btn"
                >
                  Без привязки
                </button>
              </Show>
              <button
                type="button"
                onClick={() => confirmDatapackInstall(datapackWorlds().length === 0)}
                disabled={installingDatapack() || (datapackWorlds().length > 0 && selectedWorldNames().length === 0)}
                class="px-4 py-1.5 rounded-lg bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark text-xs font-bold transition-all cursor-pointer disabled:opacity-50 flex items-center gap-1.5"
                data-testid="datapack-confirm-install-btn"
              >
                <Show when={installingDatapack()}>
                  <Loader2 class="w-3.5 h-3.5 animate-spin" />
                </Show>
                <span>
                  {datapackWorlds().length === 0
                    ? "Установить в инстанс"
                    : `Установить (${selectedWorldNames().length})`}
                </span>
              </button>
            </div>
          </div>
        </div>
      </Show>
    </div>
  );
};