import { Component, createSignal, onMount, Show } from "solid-js";
import { MessageCircle, Loader2, CheckCircle2, XCircle, Save } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { DiscordRpcStatusDTO } from "../../bindings/ipc_types";

/**
 * D'5 Discord Rich Presence — opt-in card for the Settings view.
 * Honest by design: shows the exact presence string other users will see,
 * connection state, and a clear note that nothing is ever sent without an
 * application id (no silent failure).
 */
export const DiscordPresenceCard: Component = () => {
  const [enabled, setEnabled] = createSignal(false);
  const [status, setStatus] = createSignal<DiscordRpcStatusDTO | null>(null);
  const [appId, setAppId] = createSignal("");
  const [details, setDetails] = createSignal("Minecraft <версия>");
  const [stateLine, setStateLine] = createSignal("<название инстанса>");
  const [busy, setBusy] = createSignal(false);
  const [note, setNote] = createSignal("");

  const refresh = async () => {
    try {
      const st = await launcherAPI.getDiscordRpcStatus();
      setStatus(st);
      setEnabled(st.enabled);
    } catch (_err: unknown) {
      // status is best-effort; the card keeps the last known value
    }
  };

  onMount(async () => {
    await refresh();
    try {
      const res = await launcherAPI.getSettings();
      if (res?.settings?.["discord_app_id"]) setAppId(res.settings["discord_app_id"]);
    } catch (_err: unknown) {
      // fall back to empty input
    }
    try {
      const list = await launcherAPI.listInstances();
      const first = (list || [])[0];
      if (first) {
        const p = await launcherAPI.getDiscordRpcPreview(first.id);
        if (p?.details) setDetails(p.details);
        if (p?.state) setStateLine(p.state);
      }
    } catch (_err: unknown) {
      // preview fallback to placeholders
    }
  });

  const toggle = async (v: boolean) => {
    setBusy(true);
    setNote("");
    try {
      await launcherAPI.setDiscordRpcEnabled(v);
      setEnabled(v);
      await refresh();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setNote(`Не удалось переключить: ${msg}`);
    } finally {
      setBusy(false);
    }
  };

  const saveAppId = async () => {
    setBusy(true);
    setNote("");
    try {
      await launcherAPI.setDiscordAppID(appId().trim());
      await refresh();
      setNote(appId().trim() ? "Application ID сохранён." : "Application ID очищен — presence остаётся выключенным.");
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setNote(`Сохранение не удалось: ${msg}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="bg-zinc-950 border border-zinc-800 rounded-xl p-5 space-y-4" data-testid="discord-rpc-card">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-2">
          <MessageCircle class="w-4 h-4 text-[#00D4B2]" />
          <h3 class="text-sm font-semibold uppercase tracking-wider text-zinc-100">Discord Rich Presence</h3>
        </div>
        <label class="flex items-center gap-2 cursor-pointer" data-testid="discord-rpc-toggle-label">
          <span class="text-[11px] font-mono text-zinc-400">Показывать статус</span>
          <input
            type="checkbox"
            checked={enabled()}
            disabled={busy()}
            onChange={(e) => toggle(e.currentTarget.checked)}
            class="rounded border-zinc-800 text-[#00D4B2] focus:ring-0 disabled:opacity-50"
            data-testid="discord-rpc-toggle"
          />
        </label>
      </div>

      {/* What others will see */}
      <div class="p-3 bg-zinc-900/60 border border-zinc-800 rounded-lg space-y-1" data-testid="discord-rpc-preview">
        <div class="text-[10px] font-mono text-zinc-500 uppercase tracking-wider">Что увидят другие при запуске</div>
        <div class="text-xs font-mono text-zinc-200">{details()}</div>
        <div class="text-xs font-mono text-zinc-400">{stateLine()} · с начала сессии</div>
        <div class="text-[10px] font-mono text-zinc-600">
          Только эти две строки. Никаких аккаунтных данных, идентификаторов и ссылок.
        </div>
      </div>

      <Show when={enabled()}>
        <div class="flex items-center gap-3">
          <Show
            when={status()?.connected}
            fallback={
              <span class="flex items-center gap-1.5 text-[11px] font-mono text-amber-300" data-testid="discord-rpc-offline">
                <XCircle class="w-3.5 h-3.5" />
                {status()?.app_id_set
                  ? `Discord не подключён${status()?.last_error ? ` (${status()!.last_error})` : ""} — при запуске клиента статус подхватится автоматически`
                  : "Нужен Application ID — без него presence не активируется"}
              </span>
            }
          >
            <span class="flex items-center gap-1.5 text-[11px] font-mono text-[#00D4B2]" data-testid="discord-rpc-online">
              <CheckCircle2 class="w-3.5 h-3.5" />
              Подключено к локальному Discord
            </span>
          </Show>
        </div>
      </Show>

      <div class="flex items-center gap-2">
        <input
          type="text"
          value={appId()}
          onInput={(e) => setAppId(e.currentTarget.value)}
          placeholder="Application ID из discord.com/developers"
          class="flex-1 bg-zinc-900 border border-zinc-800 focus:border-[#00D4B2] rounded px-3 py-2 text-xs text-zinc-100 placeholder-zinc-600 outline-none font-mono"
          data-testid="discord-appid-input"
        />
        <button
          type="button"
          disabled={busy()}
          onClick={saveAppId}
          class="px-3 py-2 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 rounded text-xs font-mono transition-colors flex items-center gap-1.5 cursor-pointer disabled:opacity-50"
          data-testid="discord-appid-save"
        >
          <Show when={busy()} fallback={<Save class="w-3.5 h-3.5 text-[#00D4B2]" />}>
            <Loader2 class="w-3.5 h-3.5 animate-spin text-[#00D4B2]" />
          </Show>
          <span>Сохранить</span>
        </button>
      </div>
      <Show when={note()}>
        <div class="text-[11px] font-mono text-zinc-400" data-testid="discord-rpc-note">
          {note()}
        </div>
      </Show>
    </div>
  );
};
