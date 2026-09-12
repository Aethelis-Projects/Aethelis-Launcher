import { Component, createSignal, createResource, For, Show } from "solid-js";
import { User, Plus, Key, ShieldCheck, Loader2 } from "lucide-solid";
import { launcherAPI } from "../../services/api";

export const AccountManager: Component = () => {
  const [accounts, { refetch }] = createResource(() => launcherAPI.listAccounts());
  const [offlineName, setOfflineName] = createSignal("");
  const [isLoggingInMS, setIsLoggingInMS] = createSignal(false);
  const [showOfflineModal, setShowOfflineModal] = createSignal(false);

  const handleSetActive = async (uuid: string) => {
    await launcherAPI.setActiveAccount(uuid);
    refetch();
  };

  const handleOfflineLogin = async (e: Event) => {
    e.preventDefault();
    const name = offlineName().trim();
    if (!name) return;
    await launcherAPI.loginOffline(name);
    setOfflineName("");
    setShowOfflineModal(false);
    refetch();
  };

  const handleMicrosoftLogin = async () => {
    setIsLoggingInMS(true);
    try {
      await launcherAPI.loginMicrosoft();
      refetch();
    } catch (err) {
      console.error("Microsoft login failed:", err);
    } finally {
      setIsLoggingInMS(false);
    }
  };

  return (
    <div class="space-y-4">
      <div class="flex items-center justify-between border-b border-zinc-800 pb-4">
        <div>
          <h2 class="text-sm font-semibold uppercase tracking-wider text-zinc-100 flex items-center gap-2">
            <User class="w-4 h-4 text-[#00D4B2]" />
            Управление аккаунтами
          </h2>
          <p class="text-xs text-zinc-400 mt-0.5">
            Аутентифицированные профили Microsoft и локальные офлайн-аккаунты
          </p>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setShowOfflineModal(true)}
            class="px-3 py-1.5 rounded text-xs font-mono border border-zinc-800 hover:border-zinc-700 bg-zinc-900 text-zinc-300 hover:text-zinc-100 transition-colors flex items-center gap-1.5"
          >
            <Plus class="w-3.5 h-3.5" />
            Офлайн аккаунт
          </button>

          <button
            type="button"
            disabled={isLoggingInMS()}
            onClick={handleMicrosoftLogin}
            class="px-3 py-1.5 rounded text-xs font-medium bg-[#00D4B2] hover:bg-[#00b89a] text-zinc-950 transition-colors flex items-center gap-1.5 disabled:opacity-50"
          >
            <Show
              when={isLoggingInMS()}
              fallback={
                <>
                  <Key class="w-3.5 h-3.5" />
                  Войти через Microsoft
                </>
              }
            >
              <Loader2 class="w-3.5 h-3.5 animate-spin" />
              Ожидание браузера...
            </Show>
          </button>
        </div>
      </div>

      {/* Offline Login Modal */}
      <Show when={showOfflineModal()}>
        <div class="p-4 bg-zinc-900 border border-zinc-800 rounded space-y-3">
          <h3 class="text-xs font-medium text-zinc-100">Добавить офлайн-аккаунт</h3>
          <form onSubmit={handleOfflineLogin} class="flex items-center gap-2">
            <input
              type="text"
              value={offlineName()}
              onInput={(e) => setOfflineName(e.currentTarget.value)}
              placeholder="Введите никнейм игрока..."
              class="flex-1 bg-zinc-950 border border-zinc-800 focus:border-[#00D4B2] rounded px-3 py-1.5 text-xs text-zinc-100 outline-none font-mono"
            />
            <button
              type="submit"
              class="px-3 py-1.5 bg-[#00D4B2] hover:bg-[#00b89a] text-zinc-950 rounded text-xs font-medium"
            >
              Сохранить
            </button>
            <button
              type="button"
              onClick={() => setShowOfflineModal(false)}
              class="px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 text-zinc-300 rounded text-xs"
            >
              Отмена
            </button>
          </form>
        </div>
      </Show>

      {/* Accounts List */}
      <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
        <For each={accounts()}>
          {(acc) => (
            <div
              class={`p-4 rounded border transition-colors flex items-center justify-between ${
                acc.is_active
                  ? "bg-zinc-900/90 border-[#00D4B2]/40"
                  : "bg-zinc-900/40 border-zinc-800 hover:border-zinc-700"
              }`}
            >
              <div class="flex items-center gap-3">
                <div class="w-10 h-10 rounded bg-zinc-800 border border-zinc-700 flex items-center justify-center font-mono font-bold text-zinc-300">
                  {acc.username.slice(0, 2).toUpperCase()}
                </div>
                <div>
                  <div class="flex items-center gap-2">
                    <span class="text-xs font-medium text-zinc-100">
                      {acc.username}
                    </span>
                    <span class="text-[10px] font-mono px-1.5 py-0.2 rounded uppercase bg-zinc-800 text-zinc-400 border border-zinc-700/60">
                      {acc.type}
                    </span>
                  </div>
                  <span class="text-[10px] font-mono text-zinc-500 truncate block mt-0.5 max-w-[180px]">
                    UUID: {acc.uuid}
                  </span>
                </div>
              </div>

              <div>
                <Show
                  when={acc.is_active}
                  fallback={
                    <button
                      type="button"
                      onClick={() => handleSetActive(acc.uuid)}
                      class="px-2.5 py-1 rounded text-[11px] font-mono border border-zinc-700 hover:border-zinc-600 bg-zinc-800 text-zinc-300 hover:text-zinc-100"
                    >
                      Выбрать
                    </button>
                  }
                >
                  <span class="text-xs font-mono text-[#00D4B2] flex items-center gap-1">
                    <ShieldCheck class="w-3.5 h-3.5" />
                    Активен
                  </span>
                </Show>
              </div>
            </div>
          )}
        </For>
      </div>
    </div>
  );
};