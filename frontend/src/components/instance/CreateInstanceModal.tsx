// CreateInstanceModal — v0.7.2 G10 instance-creation wizard (freesm-inspired
// flow, own implementation): name/group -> Minecraft version (release /
// snapshot, live Mojang catalog) -> loader (vanilla/fabric/quilt/forge/
// neoforge) with the recommended/latest-stable version preselected.
import {
  Show,
  For,
  createEffect,
  createSignal,
  type JSX,
} from "solid-js";
import { X, Gamepad2, Loader2, Package, CheckCircle2, Info } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { GameVersionDTO, LoaderResolutionDTO } from "../../bindings/ipc_types";
import type { InstanceDTO } from "../../bindings/ipc_types";

interface CreateInstanceModalProps {
  isOpen: boolean;
  onClose: () => void;
  onCreated?: (inst: InstanceDTO) => void;
  existingGroups?: string[];
}

const LOADERS = [
  { id: "vanilla", label: "Vanilla" },
  { id: "fabric", label: "Fabric" },
  { id: "quilt", label: "Quilt" },
  { id: "forge", label: "Forge" },
  { id: "neoforge", label: "NeoForge" },
] as const;

export function CreateInstanceModal(props: CreateInstanceModalProps): JSX.Element | null {
  const [name, setName] = createSignal("");
  const [group, setGroup] = createSignal("");
  const [channel, setChannel] = createSignal<"release" | "snapshot">("release");
  const [mcVersions, setMcVersions] = createSignal<GameVersionDTO[]>([]);
  const [gameVersion, setGameVersion] = createSignal("");
  const [loader, setLoader] = createSignal<string>("fabric");
  const [loaderRes, setLoaderRes] = createSignal<LoaderResolutionDTO | null>(null);
  const [loaderVersion, setLoaderVersion] = createSignal("");
  const [loadingVersions, setLoadingVersions] = createSignal(false);
  const [loadingLoader, setLoadingLoader] = createSignal(false);
  const [creating, setCreating] = createSignal(false);
  const [error, setError] = createSignal("");

  createEffect(() => {
    if (!props.isOpen) return;
    setLoadingVersions(true);
    setError("");
    void (async () => {
      try {
        const vers = await launcherAPI.listMinecraftVersions({ channel: channel() });
        setMcVersions(vers);
        if (vers.length > 0 && !vers.some((v) => v.id === gameVersion())) {
          setGameVersion(vers[0].id);
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setLoadingVersions(false);
      }
    })();
  });

  createEffect(() => {
    const gv = gameVersion();
    const ld = loader();
    if (!props.isOpen || !gv || ld === "vanilla") {
      setLoaderRes(null);
      setLoaderVersion("");
      return;
    }
    setLoadingLoader(true);
    void (async () => {
      try {
        const res = await launcherAPI.listLoaderVersions({ game_version: gv, loader: ld });
        setLoaderRes(res);
        setLoaderVersion(res.default || "");
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setLoadingLoader(false);
      }
    })();
  });

  const canCreate = () =>
    name().trim().length > 0 && gameVersion().length > 0 && !creating();

  const handleCreate = async () => {
    if (!canCreate()) return;
    setCreating(true);
    setError("");
    try {
      const inst = await launcherAPI.createInstanceWithLoader({
        name: name().trim(),
        game_version: gameVersion(),
        loader: loader(),
        loader_version: loader() === "vanilla" ? undefined : loaderVersion().trim() || undefined,
      });
      props.onCreated?.(inst);
      reset();
      props.onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const reset = () => {
    setName("");
    setGroup("");
    setError("");
    setLoaderRes(null);
    setLoaderVersion("");
  };

  return (
    <Show when={props.isOpen}>
      <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/80 backdrop-blur-sm" onClick={props.onClose} />
        <div class="relative w-full max-w-md bg-nord-card border border-white/10 rounded-2xl shadow-2xl overflow-hidden">
          {/* Header */}
          <div class="flex items-center justify-between px-5 py-4 border-b border-white/5">
            <h3 class="text-sm font-bold text-white flex items-center gap-2">
              <Gamepad2 class="w-4 h-4 text-nord-cyan" />
              Новая сборка
            </h3>
            <button
              type="button"
              onClick={props.onClose}
              class="text-zinc-500 hover:text-white cursor-pointer transition-colors"
              data-testid="create-instance-close-btn"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          <div class="p-5 space-y-4 max-h-[70vh] overflow-y-auto">
            {/* Step 1 — identity */}
            <section class="space-y-2" data-testid="wizard-step-identity">
              <p class="text-[10px] uppercase tracking-widest text-zinc-500 font-mono">Шаг 1 · Имя</p>
              <input
                type="text"
                value={name()}
                onInput={(e) => setName(e.currentTarget.value)}
                placeholder="Название сборки"
                class="w-full px-3 py-2 bg-zinc-900 border border-zinc-800 rounded-lg text-xs text-white placeholder-zinc-600 focus:outline-none focus:border-nord-cyan/50"
                data-testid="wizard-name-input"
              />
              <input
                type="text"
                value={group()}
                onInput={(e) => setGroup(e.currentTarget.value)}
                placeholder="Группа (необязательно)"
                list="wizard-group-suggestions"
                class="w-full px-3 py-2 bg-zinc-900 border border-zinc-800 rounded-lg text-xs text-white placeholder-zinc-600 focus:outline-none focus:border-nord-cyan/50"
                data-testid="wizard-group-input"
              />
              <datalist id="wizard-group-suggestions">
                <For each={props.existingGroups ?? []}>{(g) => <option value={g} />}</For>
              </datalist>
            </section>

            {/* Step 2 — Minecraft version */}
            <section class="space-y-2" data-testid="wizard-step-version">
              <p class="text-[10px] uppercase tracking-widest text-zinc-500 font-mono">Шаг 2 · Версия Minecraft</p>
              <div class="flex items-center gap-1 bg-zinc-900 border border-zinc-800 rounded-lg p-0.5 text-xs font-mono w-fit">
                <For
                  each={[
                    { id: "release", label: "Релизы" },
                    { id: "snapshot", label: "Снапшоты" },
                  ] as const}
                >
                  {(c) => (
                    <button
                      type="button"
                      onClick={() => setChannel(c.id)}
                      class={`px-3 py-1 rounded-md transition-colors cursor-pointer ${
                        channel() === c.id ? "bg-nord-cyan text-nord-dark font-medium" : "text-zinc-400 hover:text-zinc-100"
                      }`}
                      data-testid={`wizard-channel-${c.id}`}
                    >
                      {c.label}
                    </button>
                  )}
                </For>
              </div>
              <div class="relative">
                <select
                  value={gameVersion()}
                  onChange={(e) => setGameVersion(e.currentTarget.value)}
                  class="w-full px-3 py-2 bg-zinc-900 border border-zinc-800 rounded-lg text-xs text-white focus:outline-none appearance-none cursor-pointer"
                  data-testid="wizard-version-select"
                >
                  <Show when={loadingVersions()}>
                    <option value="">Загрузка каталога версий…</option>
                  </Show>
                  <For each={mcVersions()}>{(v) => <option value={v.id}>{v.id}</option>}</For>
                </select>
                <Show when={loadingVersions()}>
                  <Loader2 class="absolute right-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 animate-spin text-zinc-500" />
                </Show>
              </div>
            </section>

            {/* Step 3 — loader */}
            <section class="space-y-2" data-testid="wizard-step-loader">
              <p class="text-[10px] uppercase tracking-widest text-zinc-500 font-mono">Шаг 3 · Мод-загрузчик</p>
              <div class="flex flex-wrap gap-1" data-testid="wizard-loader-tabs">
                <For each={LOADERS}>
                  {(l) => (
                    <button
                      type="button"
                      onClick={() => setLoader(l.id as string)}
                      class={`px-2.5 py-1.5 rounded-lg border text-[11px] font-medium transition-colors cursor-pointer ${
                        loader() === l.id
                          ? "bg-nord-cyan/10 text-nord-cyan border-nord-cyan/30"
                          : "bg-zinc-900 text-zinc-400 border-zinc-800 hover:text-zinc-200"
                      }`}
                      data-testid={`wizard-loader-${l.id}`}
                    >
                      {l.label}
                    </button>
                  )}
                </For>
              </div>

              <Show when={loader() !== "vanilla"}>
                <div class="flex items-center gap-2">
                  <select
                    value={loaderVersion()}
                    onChange={(e) => setLoaderVersion(e.currentTarget.value)}
                    class="flex-1 px-3 py-2 bg-zinc-900 border border-zinc-800 rounded-lg text-xs text-white focus:outline-none cursor-pointer"
                    data-testid="wizard-loader-version-select"
                  >
                    <Show when={loadingLoader()}>
                      <option value="">Резолв версий лоадера…</option>
                    </Show>
                    <Show when={!loadingLoader() && (loaderRes()?.options.length ?? 0) === 0}>
                      <option value="">{loaderRes()?.note ? "только вручную" : "нет данных — можно оставить пусто"}</option>
                    </Show>
                    <For each={loaderRes()?.options ?? []}>{(v) => <option value={v}>{v}</option>}</For>
                  </select>
                  <input
                    type="text"
                    value={loaderVersion()}
                    onInput={(e) => setLoaderVersion(e.currentTarget.value)}
                    placeholder="или вручную"
                    class="w-28 px-3 py-2 bg-zinc-900 border border-zinc-800 rounded-lg text-xs text-white placeholder-zinc-600 focus:outline-none focus:border-nord-cyan/50"
                    data-testid="wizard-loader-version-manual"
                  />
                </div>
                <Show when={loaderRes()?.default && !loadingLoader()}>
                  <p class="flex items-center gap-1.5 text-[10px] text-zinc-500" data-testid="wizard-loader-note">
                    <Package class="w-3 h-3 text-nord-cyan shrink-0" />
                    {loaderRes()!.source === "forge" ? "recommended" : "stable"} по умолчанию: {loaderRes()!.default}
                    {loaderRes()!.note ? ` · ${loaderRes()!.note}` : ""}
                  </p>
                </Show>
              </Show>
              <Show when={loader() === "vanilla"}>
                <p class="flex items-center gap-1.5 text-[10px] text-zinc-500">
                  <Info class="w-3 h-3 text-nord-cyan shrink-0" />
                  Vanilla — без загрузчика; моды позже можно поставить через Каталог ( datapacks / plugins не применяются ).
                </p>
              </Show>
            </section>

            <Show when={error()}>
              <div class="flex items-center gap-2 text-xs text-red-400 bg-red-950/40 border border-red-900/50 rounded-lg px-3 py-2" data-testid="wizard-error">
                <X class="w-3.5 h-3.5 shrink-0" />
                <span>{error()}</span>
              </div>
            </Show>
          </div>

          {/* Footer */}
          <div class="px-5 py-4 border-t border-white/5 flex items-center justify-end gap-3 bg-zinc-900/30">
            <button
              type="button"
              onClick={props.onClose}
              class="px-4 py-2 text-xs font-mono text-zinc-400 hover:text-white transition-colors cursor-pointer"
              data-testid="wizard-cancel-btn"
            >
              Отмена
            </button>
            <button
              type="button"
              disabled={!canCreate()}
              onClick={() => void handleCreate()}
              class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg bg-nord-cyan text-nord-dark text-xs font-semibold hover:bg-nord-cyan/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors cursor-pointer"
              data-testid="wizard-create-btn"
            >
              <Show when={creating()} fallback={<CheckCircle2 class="w-3.5 h-3.5" />}>
                <Loader2 class="w-3.5 h-3.5 animate-spin" />
              </Show>
              Создать сборку
            </button>
          </div>
        </div>
      </div>
    </Show>
  );
}

export default CreateInstanceModal;
