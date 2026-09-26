import { Component, createSignal, createEffect, onCleanup, For, Show } from "solid-js";
import { X, Image, Copy, Trash2, FolderOpen, RefreshCw, Check, AlertCircle } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import { ScreenshotDTO } from "../../bindings/ipc_types";

export interface ScreenshotGalleryModalProps {
  isOpen: boolean;
  instanceId: string;
  instanceName: string;
  onClose: () => void;
}

export const ScreenshotGalleryModal: Component<ScreenshotGalleryModalProps> = (props) => {
  const [screenshots, setScreenshots] = createSignal<ScreenshotDTO[]>([]);
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal("");
  const [toast, setToast] = createSignal<{ message: string; type: "success" | "info" | "error" } | null>(null);
  const [thumbnails, setThumbnails] = createSignal<Record<string, string>>({});
  const [deletingFile, setDeletingFile] = createSignal<string | null>(null);

  const showToast = (message: string, type: "success" | "info" | "error" = "success") => {
    setToast({ message, type });
    setTimeout(() => {
      setToast((prev) => (prev?.message === message ? null : prev));
    }, 3000);
  };

  const loadScreenshots = async () => {
    if (!props.instanceId) return;
    setLoading(true);
    setError("");
    try {
      const list = await launcherAPI.listScreenshots(props.instanceId);
      setScreenshots(list);
      // Pre-load thumbnails for first 20 screenshots
      for (const shot of list.slice(0, 20)) {
        if (!thumbnails()[shot.file_name]) {
          launcherAPI
            .getScreenshotData({ instance_id: props.instanceId, file_name: shot.file_name })
            .then((res) => {
              setThumbnails((prev) => ({ ...prev, [shot.file_name]: res.data_url }));
            })
            .catch(() => {
              // ignore thumbnail errors
            });
        }
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  createEffect(() => {
    if (props.isOpen && props.instanceId) {
      loadScreenshots();
      const onFocus = () => {
        loadScreenshots();
      };
      window.addEventListener("focus", onFocus);
      onCleanup(() => {
        window.removeEventListener("focus", onFocus);
      });
    }
  });

  const handleCopy = async (shot: ScreenshotDTO) => {
    try {
      if (typeof navigator !== "undefined" && navigator.clipboard && typeof window !== "undefined" && "ClipboardItem" in window) {
        let dataUrl = thumbnails()[shot.file_name];
        if (!dataUrl) {
          const res = await launcherAPI.getScreenshotData({
            instance_id: props.instanceId,
            file_name: shot.file_name,
          });
          dataUrl = res.data_url;
          setThumbnails((prev) => ({ ...prev, [shot.file_name]: dataUrl }));
        }

        const base64Data = dataUrl.split(",")[1];
        if (base64Data) {
          const byteCharacters = atob(base64Data);
          const byteNumbers = new Array(byteCharacters.length);
          for (let i = 0; i < byteCharacters.length; i++) {
            byteNumbers[i] = byteCharacters.charCodeAt(i);
          }
          const byteArray = new Uint8Array(byteNumbers);
          const blob = new Blob([byteArray], { type: "image/png" });
          const clipboardItem = new window.ClipboardItem({ "image/png": blob });
          await navigator.clipboard.write([clipboardItem]);
          showToast("Скриншот скопирован в буфер", "success");
          return;
        }
      }
      // Fallback: reveal in file manager
      await launcherAPI.openPath(shot.path);
      showToast("Файл открыт в папке (буфер недоступен)", "info");
    } catch (_err: unknown) {
      // Fallback on clipboard write failure
      try {
        await launcherAPI.openPath(shot.path);
        showToast("Файл открыт в папке (буфер недоступен)", "info");
      } catch (fallbackErr: unknown) {
        showToast("Не удалось скопировать или открыть файл", "error");
      }
    }
  };

  const handleDelete = async (shot: ScreenshotDTO) => {
    setDeletingFile(shot.file_name);
    try {
      await launcherAPI.deleteScreenshot({
        instance_id: props.instanceId,
        file_name: shot.file_name,
      });
      setScreenshots((prev) => prev.filter((s) => s.file_name !== shot.file_name));
      showToast("Скриншот удален", "info");
    } catch (err: unknown) {
      showToast("Не удалось удалить скриншот", "error");
    } finally {
      setDeletingFile(null);
    }
  };

  const handleOpenFolder = () => {
    launcherAPI.openPath(props.instanceId);
  };

  const formatSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} Б`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
  };

  const formatDate = (dateStr: string): string => {
    try {
      const d = new Date(dateStr);
      return d.toLocaleDateString("ru-RU", {
        day: "2-digit",
        month: "short",
        hour: "2-digit",
        minute: "2-digit",
      });
    } catch {
      return dateStr;
    }
  };

  return (
    <Show when={props.isOpen}>
      <div
        class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm"
        data-testid="screenshot-gallery-modal"
      >
        <div class="w-full max-w-4xl bg-nord-surface border border-white/10 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[85vh] transition-all">
          {/* Header */}
          <div class="flex items-center justify-between px-6 py-4 border-b border-white/5 bg-nord-dark/40">
            <div class="flex items-center gap-3">
              <div class="p-2 rounded-lg bg-nord-cyan/10 text-nord-cyan border border-nord-cyan/20">
                <Image class="w-5 h-5" />
              </div>
              <div>
                <h3 class="font-bold text-white text-base">Скриншоты</h3>
                <p class="text-xs text-zinc-400">{props.instanceName} ({screenshots().length})</p>
              </div>
            </div>

            <div class="flex items-center gap-2">
              <button
                type="button"
                onClick={handleOpenFolder}
                class="px-3 py-1.5 rounded-lg bg-zinc-800/80 hover:bg-zinc-700 border border-white/10 text-zinc-300 hover:text-white text-xs font-medium flex items-center gap-1.5 transition-colors cursor-pointer"
                data-testid="gallery-open-folder-btn"
                title="Открыть папку со скриншотами"
              >
                <FolderOpen class="w-4 h-4" />
                <span>Папка</span>
              </button>

              <button
                type="button"
                onClick={loadScreenshots}
                class="p-2 rounded-lg text-zinc-400 hover:text-white hover:bg-white/5 transition-colors cursor-pointer"
                title="Обновить список"
                data-testid="gallery-refresh-btn"
              >
                <RefreshCw class={`w-4 h-4 ${loading() ? "animate-spin" : ""}`} />
              </button>

              <button
                type="button"
                onClick={props.onClose}
                class="p-2 rounded-lg text-zinc-400 hover:text-white hover:bg-white/5 transition-colors cursor-pointer"
                title="Закрыть"
                data-testid="gallery-close-btn"
              >
                <X class="w-5 h-5" />
              </button>
            </div>
          </div>

          {/* Toast Notification */}
          <Show when={toast()}>
            <div
              class={`mx-6 mt-3 p-2.5 rounded-lg text-xs flex items-center gap-2 font-medium transition-all ${
                toast()!.type === "success"
                  ? "bg-nord-emerald/15 text-nord-emerald border border-nord-emerald/30"
                  : toast()!.type === "error"
                  ? "bg-nord-rose/15 text-nord-rose border border-nord-rose/30"
                  : "bg-nord-cyan/15 text-nord-cyan border border-nord-cyan/30"
              }`}
              data-testid="gallery-toast"
            >
              {toast()!.type === "success" ? (
                <Check class="w-4 h-4 shrink-0" />
              ) : (
                <AlertCircle class="w-4 h-4 shrink-0" />
              )}
              <span>{toast()!.message}</span>
            </div>
          </Show>

          {/* Error Banner */}
          <Show when={error()}>
            <div class="mx-6 mt-3 p-3 rounded-lg bg-nord-rose/10 border border-nord-rose/20 text-xs text-nord-rose flex items-center gap-2">
              <AlertCircle class="w-4 h-4 shrink-0" />
              <span>{error()}</span>
            </div>
          </Show>

          {/* Gallery Content */}
          <div class="flex-1 p-6 overflow-y-auto">
            <Show
              when={screenshots().length > 0}
              fallback={
                <div class="py-16 flex flex-col items-center justify-center text-center">
                  <div class="w-16 h-16 rounded-2xl bg-zinc-900 border border-white/5 flex items-center justify-center text-zinc-600 mb-3">
                    <Image class="w-8 h-8" />
                  </div>
                  <p class="text-sm font-semibold text-zinc-300">Скриншотов пока нет</p>
                  <p class="text-xs text-zinc-500 mt-1 max-w-xs">
                    Нажмите F2 во время игры в Minecraft, чтобы сохранить снимок экрана
                  </p>
                </div>
              }
            >
              <div class="grid grid-cols-2 sm:grid-cols-3 gap-4" data-testid="screenshots-grid">
                <For each={screenshots()}>
                  {(shot) => (
                    <div
                      class="group relative bg-zinc-900/80 border border-white/5 hover:border-white/20 rounded-xl overflow-hidden flex flex-col transition-all"
                      data-testid={`screenshot-card-${shot.file_name}`}
                    >
                      {/* Image Thumbnail Container */}
                      <div class="w-full aspect-video bg-black/40 flex items-center justify-center overflow-hidden relative">
                        {thumbnails()[shot.file_name] ? (
                          <img
                            src={thumbnails()[shot.file_name]}
                            alt={shot.file_name}
                            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-200"
                            loading="lazy"
                          />
                        ) : (
                          <div class="flex flex-col items-center gap-1 text-zinc-600">
                            <Image class="w-6 h-6" />
                            <span class="text-[10px] font-mono">PNG</span>
                          </div>
                        )}

                        {/* Hover Overlay Actions */}
                        <div class="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 flex items-center justify-center gap-2 transition-opacity">
                          <button
                            type="button"
                            onClick={() => handleCopy(shot)}
                            class="p-2 rounded-lg bg-nord-cyan text-nord-dark hover:bg-nord-cyan-hover transition-colors cursor-pointer"
                            title="Копировать в буфер обмена"
                            data-testid={`copy-btn-${shot.file_name}`}
                          >
                            <Copy class="w-4 h-4" />
                          </button>
                          <button
                            type="button"
                            onClick={() => launcherAPI.openPath(shot.path)}
                            class="p-2 rounded-lg bg-zinc-800 text-white hover:bg-zinc-700 transition-colors cursor-pointer"
                            title="Открыть файл"
                            data-testid={`open-btn-${shot.file_name}`}
                          >
                            <FolderOpen class="w-4 h-4" />
                          </button>
                          <button
                            type="button"
                            disabled={deletingFile() === shot.file_name}
                            onClick={() => handleDelete(shot)}
                            class="p-2 rounded-lg bg-nord-rose/80 text-white hover:bg-nord-rose transition-colors cursor-pointer"
                            title="Удалить скриншот"
                            data-testid={`delete-btn-${shot.file_name}`}
                          >
                            <Trash2 class="w-4 h-4" />
                          </button>
                        </div>
                      </div>

                      {/* File Metadata */}
                      <div class="p-2.5 flex flex-col gap-0.5">
                        <span
                          class="text-xs text-zinc-200 font-mono font-medium truncate"
                          title={shot.file_name}
                        >
                          {shot.file_name}
                        </span>
                        <div class="flex items-center justify-between text-[11px] text-zinc-500 font-mono">
                          <span>{formatDate(shot.created_at)}</span>
                          <span>{formatSize(shot.size)}</span>
                        </div>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </div>
      </div>
    </Show>
  );
};
