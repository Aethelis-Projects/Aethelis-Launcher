import { Component, createSignal, onMount, Show } from "solid-js";
import { Key, Eye, EyeOff, Save, CheckCircle2, AlertCircle, ShieldCheck, ChevronDown } from "lucide-solid";
import { launcherAPI } from "../../services/api";

export const CurseForgeKeyCard: Component = () => {
  const [key, setKey] = createSignal("");
  const [showKey, setShowKey] = createSignal(false);
  const [saving, setSaving] = createSignal(false);
  const [statusMessage, setStatusMessage] = createSignal<string>("");
  const [statusType, setStatusType] = createSignal<"success" | "error" | "">("");
  const [hasBuiltinKey, setHasBuiltinKey] = createSignal(false);
  const [isSpoilerOpen, setIsSpoilerOpen] = createSignal(false);

  onMount(async () => {
    try {
      const builtin = await launcherAPI.hasBuiltinCurseForgeKey();
      setHasBuiltinKey(builtin);
    } catch (_err: unknown) {
      // Safe fallback
    }

    try {
      const res = await launcherAPI.getSettings();
      if (res && res.settings) {
        const savedKey = res.settings["curseforge_api_key"] || "";
        const hasSaved = Boolean(savedKey) || res.settings["has_curseforge_api_key"] === "true";
        if (savedKey) {
          setKey(savedKey);
        }
        if (hasSaved) {
          setIsSpoilerOpen(true);
        }
      }
    } catch (_err: unknown) {
      // Safe fallback if settings cannot be read
    }
  });

  const handleSave = async (e: Event) => {
    e.preventDefault();
    setSaving(true);
    setStatusMessage("");
    setStatusType("");

    try {
      await launcherAPI.setSetting("curseforge_api_key", key().trim());
      setStatusType("success");
      setStatusMessage("API-ключ CurseForge успешно сохранен.");
    } catch (_err: unknown) {
      setStatusType("error");
      setStatusMessage("Не удалось сохранить API-ключ.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div class="bg-nord-surface/80 border border-white/5 rounded-xl p-5 backdrop-blur-md shadow-lg space-y-4">
      <div class="flex items-center justify-between border-b border-white/5 pb-3">
        <div class="flex items-center gap-3">
          <div class="w-8 h-8 rounded-lg bg-nord-cyan/10 border border-nord-cyan/30 flex items-center justify-center text-nord-cyan">
            <Key class="w-4 h-4" />
          </div>
          <div>
            <h3 class="text-sm font-semibold text-zinc-100">CurseForge API Key (BYOK)</h3>
            <p class="text-xs text-zinc-400">
              Персональный ключ для поиска и загрузки модификаций с CurseForge. Modrinth доступен без ключа.
            </p>
          </div>
        </div>

        <Show when={hasBuiltinKey()}>
          <div
            class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-nord-emerald/10 border border-nord-emerald/30 text-xs text-nord-emerald font-medium shrink-0"
            data-testid="cf-builtin-badge"
          >
            <ShieldCheck class="w-3.5 h-3.5" />
            <span>Встроенный ключ активен</span>
          </div>
        </Show>
      </div>

      <Show when={hasBuiltinKey()}>
        <div class="pt-1">
          <button
            type="button"
            onClick={() => setIsSpoilerOpen(!isSpoilerOpen())}
            data-testid="cf-custom-key-spoiler-toggle"
            class="inline-flex items-center gap-1.5 text-xs text-nord-cyan hover:text-nord-cyan-hover transition-colors font-medium focus:outline-none"
          >
            <ChevronDown class={`w-3.5 h-3.5 transition-transform ${isSpoilerOpen() ? "rotate-180" : ""}`} />
            <span>Использовать свой ключ</span>
          </button>
        </div>
      </Show>

      <Show when={!hasBuiltinKey() || isSpoilerOpen()}>
        <form onSubmit={handleSave} class="space-y-3 pt-1">
          <div class="space-y-1.5">
            <label class="block text-xs font-medium text-zinc-300">
              API Key
            </label>
            <div class="relative flex items-center">
              <input
                type={showKey() ? "text" : "password"}
                value={key()}
                onInput={(e) => setKey(e.currentTarget.value)}
                placeholder="32-значный hex-ключ CurseForge (например, a1b2c3d4...)"
                data-testid="cf-key-input"
                class="w-full px-3 py-2 pr-10 bg-nord-dark/80 border border-white/10 rounded-lg text-xs text-zinc-100 font-mono placeholder:text-zinc-600 focus:outline-none focus:border-nord-cyan transition-colors"
              />
              <button
                type="button"
                onClick={() => setShowKey(!showKey())}
                data-testid="cf-key-toggle-visibility"
                title={showKey() ? "Скрыть ключ" : "Показать ключ"}
                class="absolute right-2 text-zinc-500 hover:text-zinc-300 transition-colors p-1"
              >
                {showKey() ? <EyeOff class="w-4 h-4" /> : <Eye class="w-4 h-4" />}
              </button>
            </div>
          </div>

          <div class="flex items-center justify-between pt-1">
            <div class="flex items-center gap-1.5 text-xs">
              <Show when={statusType() === "success"}>
                <CheckCircle2 class="w-3.5 h-3.5 text-nord-emerald" />
                <span class="text-nord-emerald font-medium" data-testid="cf-key-status">
                  {statusMessage()}
                </span>
              </Show>
              <Show when={statusType() === "error"}>
                <AlertCircle class="w-3.5 h-3.5 text-nord-rose" />
                <span class="text-nord-rose font-medium" data-testid="cf-key-status">
                  {statusMessage()}
                </span>
              </Show>
            </div>

            <button
              type="submit"
              disabled={saving()}
              data-testid="cf-key-save-btn"
              class="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-nord-cyan hover:bg-nord-cyan-hover text-nord-dark font-semibold text-xs transition-colors disabled:opacity-50"
            >
              <Save class="w-3.5 h-3.5" />
              <span>{saving() ? "Сохранение..." : "Сохранить"}</span>
            </button>
          </div>
        </form>
      </Show>
    </div>
  );
};
