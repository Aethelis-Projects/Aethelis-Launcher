// ModpacksView — v0.7.2 G5: "Модпаки" storefront page.
//
// Modrinth-only browsing of .mrpack-ready projects with server-side
// game-version/loader/category filters, per-card version picker and
// one-click import through the URL import pipeline (ImportMrPackFromURL).
// CurseForge's search API does not expose modpacks, which is stated
// honestly in the card instead of pretending the tab exists.
import {
  For,
  Show,
  createEffect,
  createMemo,
  createSignal,
  onCleanup,
} from "solid-js";
import {
  Package,
  Search,
  Download,
  Loader2,
  CheckCircle2,
  X,
  AlertCircle,
  FolderDown,
  Info,
} from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type {
  ModItemDTO,
  MrPackVersionDTO,
  GameVersionDTO,
  MrPackImportStatusDTO,
} from "../../bindings/ipc_types";

interface ModpacksViewProps {
  /** Opens the unified import modal (CurseForge .zip path). */
  onOpenUnifiedImport?: (tab?: "mrpack" | "curseforge") => void;
  onImported?: (instanceId: string) => void;
}

type CardState = {
  phase: "idle" | "versions" | "importing" | "done" | "error";
  versions: MrPackVersionDTO[];
  status?: MrPackImportStatusDTO;
  error?: string;
};

export function ModpacksView(props: ModpacksViewProps) {
  const [query, setQuery] = createSignal("");
  const [submitted, setSubmitted] = createSignal("");
  const [gameVersion, setGameVersion] = createSignal("");
  const [loader, setLoader] = createSignal("");
  const [category, setCategory] = createSignal("");
  const [results, setResults] = createSignal<ModItemDTO[]>([]);
  const [total, setTotal] = createSignal(0);
  const [searching, setSearching] = createSignal(false);
  const [searchError, setSearchError] = createSignal("");
  const [mcVersions, setMcVersions] = createSignal<GameVersionDTO[]>([]);
  const [tags, setTags] = createSignal<{ id: string; label: string }[]>([]);
  const [cardStates, setCardStates] = createSignal<Record<string, CardState>>({});

  const loaders = ["fabric", "quilt", "forge", "neoforge"];

  createEffect(() => {
    void (async () => {
      try {
        setMcVersions(await launcherAPI.listMinecraftVersions({ channel: "release" }));
      } catch (_e) {
        void _e;
      }
      try {
        setTags(await launcherAPI.listProjectTags({ provider: "modrinth", project_type: "modpack" }));
      } catch (_e) {
        void _e;
      }
    })();
  });

  const runSearch = async () => {
    setSearching(true);
    setSearchError("");
    setCardStates({});
    try {
      const res = await launcherAPI.searchMods({
        query: submitted(),
        game_version: gameVersion(),
        loader: loader(),
        source: "modrinth",
        limit: 24,
        offset: 0,
        category: category(),
        project_type: "modpack" as never,
      });
      setResults(res.items ?? []);
      setTotal(res.total_count ?? 0);
    } catch (e) {
      setSearchError(e instanceof Error ? e.message : String(e));
      setResults([]);
      setTotal(0);
    } finally {
      setSearching(false);
    }
  };

  createEffect(() => {
    void submitted();
    void runSearch();
  });

  const patchCard = (slug: string, next: Partial<CardState>) =>
    setCardStates((prev) => ({ ...prev, [slug]: { ...(prev[slug] ?? { phase: "idle", versions: [] }), ...next } as CardState }));

  const openVersions = async (mod: ModItemDTO) => {
    patchCard(mod.slug, { phase: "versions", versions: [], error: undefined });
    try {
      const vers = await launcherAPI.listMrPackVersions({
        project_slug: mod.slug,
        game_version: gameVersion() || undefined,
        loader: loader() || undefined,
      });
      const mrpackOnly = vers.filter((v) => v.filename.toLowerCase().endsWith(".mrpack"));
      patchCard(mod.slug, { phase: "versions", versions: mrpackOnly });
    } catch (e) {
      patchCard(mod.slug, {
        phase: "error",
        error: e instanceof Error ? e.message : String(e),
      });
    }
  };

  const timers = new Map<string, number>();
  onCleanup(() => {
    for (const t of timers.values()) window.clearInterval(t);
    timers.clear();
  });

  const importPack = async (mod: ModItemDTO, ver: MrPackVersionDTO) => {
    patchCard(mod.slug, { phase: "importing", status: undefined });
    try {
      const instPromise = launcherAPI.importMrPackFromURL({
        url: ver.url,
        instance_name: mod.name,
      });
      const timer = window.setInterval(async () => {
        try {
          const st = await launcherAPI.getMrPackURLImportStatus(mod.name);
          patchCard(mod.slug, { phase: "importing", status: st });
        } catch (_e) {
          void _e;
        }
      }, 600);
      timers.set(mod.slug, timer);
      await instPromise;
      window.clearInterval(timer);
      timers.delete(mod.slug);
      patchCard(mod.slug, {
        phase: "done",
        status: { status: "complete" } as never,
      });
      props.onImported?.("");
    } catch (e) {
      const t = timers.get(mod.slug);
      if (t) window.clearInterval(t);
      timers.delete(mod.slug);
      patchCard(mod.slug, {
        phase: "error",
        error: e instanceof Error ? e.message : String(e),
      });
    }
  };

  const formatDownloads = createMemo(() => (n: number) =>
    n >= 1_000_000 ? `${(n / 1_000_000).toFixed(1)}M` : n >= 1_000 ? `${(n / 1_000).toFixed(1)}k` : String(n),
  );

  return (
    <div class="p-6 space-y-5 h-full overflow-y-auto" data-testid="modpacks-view">
      {/* Header */}
      <div class="flex items-start justify-between gap-4">
        <div>
          <h2 class="text-2xl font-bold text-white flex items-center gap-2">
            <Package class="w-6 h-6 text-nord-cyan" />
            Модпаки
          </h2>
          <p class="text-xs text-zinc-500 mt-1">
            Каталог сборок Modrinth (.mrpack) — импорт в один клик с прогрессом загрузки
            <Show when={total() > 0}>
              <span class="ml-1 font-mono text-zinc-600">({total()})</span>
            </Show>
          </p>
        </div>
        <Show when={props.onOpenUnifiedImport}>
          <button
            type="button"
            onClick={() => props.onOpenUnifiedImport?.("curseforge")}
            class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-300 hover:text-white border border-white/10 text-xs font-medium transition-colors cursor-pointer shrink-0"
            data-testid="modpacks-open-unified-btn"
          >
            <FolderDown class="w-3.5 h-3.5 text-nord-cyan" />
            CurseForge / локальный импорт
          </button>
        </Show>
      </div>

      {/* Honest provider note */}
      <div
        class="flex items-start gap-2 text-xs text-zinc-400 bg-zinc-900/60 border border-zinc-800 rounded-lg px-3 py-2"
        data-testid="modpacks-provider-note"
      >
        <Info class="w-3.5 h-3.5 text-nord-cyan shrink-0 mt-0.5" />
        <span>
          Modrinth отдаёт сборки через API и формат .mrpack. CurseForge не публикует
          модпаки в поисковом API — их .zip скачивается на curseforge.com и импортируется
          во вкладке «CurseForge» unified-импорта.
        </span>
      </div>

      {/* Search + filters */}
      <div class="flex flex-wrap items-center gap-2" data-testid="modpacks-filters">
        <div class="relative flex-1 min-w-[220px]">
          <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-500" />
          <input
            type="text"
            value={query()}
            onInput={(e) => setQuery(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") setSubmitted(query());
            }}
            placeholder="Поиск сборок (Modrinth)…"
            class="w-full pl-8 pr-3 py-1.5 bg-zinc-900 border border-zinc-800 rounded-lg text-xs text-white placeholder-zinc-600 focus:outline-none focus:border-nord-cyan/50"
            data-testid="modpacks-search-input"
          />
        </div>
        <select
          value={gameVersion()}
          onChange={(e) => {
            setGameVersion(e.currentTarget.value);
            setSubmitted(query());
          }}
          class="bg-zinc-900 border border-zinc-800 rounded-lg px-2 py-1.5 text-xs text-zinc-300 focus:outline-none"
          data-testid="modpacks-filter-version"
        >
          <option value="">Любая версия MC</option>
          <For each={mcVersions().slice(0, 40)}>
            {(v) => <option value={v.id}>{v.id}</option>}
          </For>
        </select>
        <select
          value={loader()}
          onChange={(e) => {
            setLoader(e.currentTarget.value);
            setSubmitted(query());
          }}
          class="bg-zinc-900 border border-zinc-800 rounded-lg px-2 py-1.5 text-xs text-zinc-300 focus:outline-none"
          data-testid="modpacks-filter-loader"
        >
          <option value="">Любой лоадер</option>
          <For each={loaders}>{(l) => <option value={l}>{l}</option>}</For>
        </select>
        <select
          value={category()}
          onChange={(e) => {
            setCategory(e.currentTarget.value);
            setSubmitted(query());
          }}
          class="bg-zinc-900 border border-zinc-800 rounded-lg px-2 py-1.5 text-xs text-zinc-300 focus:outline-none"
          data-testid="modpacks-filter-category"
        >
          <option value="">Все категории</option>
          <For each={tags().slice(0, 60)}>
            {(t) => <option value={t.id}>{t.label}</option>}
          </For>
        </select>
        <button
          type="button"
          onClick={() => setSubmitted(query())}
          class="px-3 py-1.5 rounded-lg bg-nord-cyan/10 hover:bg-nord-cyan/20 text-nord-cyan border border-nord-cyan/20 text-xs font-medium transition-colors cursor-pointer"
          data-testid="modpacks-search-btn"
        >
          Найти
        </button>
      </div>

      {/* Error banner */}
      <Show when={searchError()}>
        <div
          class="flex items-center gap-2 text-xs text-red-400 bg-red-950/40 border border-red-900/50 rounded-lg px-3 py-2"
          data-testid="modpacks-error-banner"
        >
          <AlertCircle class="w-3.5 h-3.5 shrink-0" />
          <span>{searchError()}</span>
        </div>
      </Show>

      {/* Results */}
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">
        <Show when={!searching() && results().length === 0}>
          <div class="col-span-full text-center py-10 text-xs text-zinc-600" data-testid="modpacks-empty">
            Ничего не найдено — уточните фильтры
          </div>
        </Show>
        <For each={results()}>
          {(mod) => {
            const state = createMemo<CardState>(() => cardStates()[mod.slug] ?? { phase: "idle", versions: [] });
            return (
              <div
                class="bg-zinc-900/70 border border-zinc-800 rounded-xl p-3 flex flex-col gap-2"
                data-testid={`modpack-card-${mod.slug}`}
              >
                <div class="flex items-start gap-3">
                  <Show
                    when={mod.icon_url}
                    fallback={<div class="w-10 h-10 rounded-lg bg-zinc-800 flex items-center justify-center shrink-0"><Package class="w-5 h-5 text-zinc-600" /></div>}
                  >
                    <img src={mod.icon_url} alt="" class="w-10 h-10 rounded-lg object-cover shrink-0" />
                  </Show>
                  <div class="min-w-0 flex-1">
                    <p class="text-sm font-semibold text-white truncate">{mod.name}</p>
                    <p class="text-[11px] text-zinc-500 truncate">
                      {mod.author} · {formatDownloads()(mod.downloads)} загрузок
                    </p>
                    <p class="text-[11px] text-zinc-400 line-clamp-2 mt-1">{mod.summary}</p>
                  </div>
                  <Show when={state().phase === "idle"}>
                    <button
                      type="button"
                      onClick={() => void openVersions(mod)}
                      class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-nord-cyan/10 hover:bg-nord-cyan/20 text-nord-cyan border border-nord-cyan/20 text-[11px] font-medium transition-colors cursor-pointer shrink-0"
                      data-testid={`modpack-import-btn-${mod.slug}`}
                    >
                      <Download class="w-3 h-3" />
                      Импорт
                    </button>
                  </Show>
                  <Show when={state().phase === "versions" && state().versions.length === 0}>
                    <span class="text-[11px] text-zinc-500 shrink-0">нет .mrpack под фильтр</span>
                  </Show>
                  <Show when={state().phase === "done"}>
                    <span class="inline-flex items-center gap-1 text-[11px] text-emerald-400 shrink-0">
                      <CheckCircle2 class="w-3.5 h-3.5" /> Импортировано
                    </span>
                  </Show>
                </div>

                {/* Version picker */}
                <Show when={state().phase === "versions"}>
                  <div class="flex flex-col gap-1 border-t border-zinc-800 pt-2" data-testid={`modpack-version-list-${mod.slug}`}>
                    <For each={state().versions.slice(0, 6)}>
                      {(v) => (
                        <button
                          type="button"
                          onClick={() => void importPack(mod, v)}
                          class="flex items-center justify-between gap-2 px-2 py-1.5 rounded-lg bg-zinc-950/60 hover:bg-zinc-800/60 border border-zinc-800 text-left cursor-pointer transition-colors"
                          data-testid={`modpack-version-${mod.slug}-${v.version_id}`}
                        >
                          <span class="min-w-0">
                            <span class="block text-[11px] text-zinc-200 truncate">{v.name || v.filename}</span>
                            <span class="block text-[10px] text-zinc-500">
                              {v.game_version} · {v.loaders.join(", ")} · {v.version_type}
                            </span>
                          </span>
                          <Download class="w-3.5 h-3.5 text-nord-cyan shrink-0" />
                        </button>
                      )}
                    </For>
                    <button
                      type="button"
                      onClick={() => patchCard(mod.slug, { phase: "idle" })}
                      class="self-end inline-flex items-center gap-1 text-[10px] text-zinc-500 hover:text-zinc-300 cursor-pointer mt-1"
                    >
                      <X class="w-3 h-3" /> свернуть
                    </button>
                  </div>
                </Show>

                {/* Progress */}
                <Show when={state().phase === "importing"}>
                  <div class="flex items-center gap-2 text-[11px] text-zinc-400 border-t border-zinc-800 pt-2" data-testid={`modpack-progress-${mod.slug}`}>
                    <Loader2 class="w-3.5 h-3.5 animate-spin text-nord-cyan shrink-0" />
                    <Show when={state().status} fallback={<span>Загрузка .mrpack…</span>}>
                      {(st) => (
                        <span class="truncate">
                          {st().status === "downloading" ? "Скачивание пакета…" : "Распаковка и установка модов…"}{" "}
                          {state().status!.current_file ? `· ${state().status!.current_file}` : ""}
                          {state().status!.total_files > 0
                            ? ` · ${state().status!.files_done}/${state().status!.total_files}`
                            : ""}
                        </span>
                      )}
                    </Show>
                  </div>
                </Show>

                {/* Card error */}
                <Show when={state().phase === "error"}>
                  <div class="flex items-center gap-2 text-[11px] text-red-400 border-t border-zinc-800 pt-2">
                    <AlertCircle class="w-3.5 h-3.5 shrink-0" />
                    <span class="truncate">{state().error}</span>
                  </div>
                </Show>
              </div>
            );
          }}
        </For>
      </div>

      <Show when={searching()}>
        <div class="flex items-center justify-center gap-2 py-6 text-xs text-zinc-500">
          <Loader2 class="w-4 h-4 animate-spin" /> Поиск сборок…
        </div>
      </Show>
    </div>
  );
}

export default ModpacksView;
