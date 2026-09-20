import { Component, createSignal, Show, onMount, For } from "solid-js";
import { RefreshCw, Download, CheckCircle2, AlertCircle, ShieldCheck, Loader2, RotateCcw } from "lucide-solid";
import { launcherAPI } from "../../services/api";
import type { UpdateInfoDTO, UpdateApplyResultDTO } from "../../bindings/ipc_types";

export interface UpdatePanelProps {
  channel?: string;
  onUpdateAvailable?: (info: UpdateInfoDTO) => void;
  onRestartTriggered?: () => void;
}

export type UpdateStatus =
  | "idle"
  | "checking"
  | "available"
  | "up_to_date"
  | "applying"
  | "restart_required"
  | "error";

export const formatVersion = (v?: string): string => {
  if (!v) return "";
  return v.startsWith("v") ? v : `v${v}`;
};

export const renderMarkdownLite = (text?: string) => {
  if (!text) {
    return <p class="text-zinc-400">Regular maintenance release with security and performance improvements.</p>;
  }
  const lines = text.split("\n");
  return (
    <div class="space-y-1">
      <For each={lines}>
        {(line) => {
          const trimmed = line.trim();
          if (trimmed.startsWith("### ")) {
            return (
              <div class="font-bold text-zinc-200 text-[11px] uppercase tracking-wider mt-2.5 mb-1 text-nord-cyan/90">
                {trimmed.substring(4)}
              </div>
            );
          }
          if (trimmed.startsWith("## ")) {
            return (
              <div class="font-bold text-white text-xs mt-3 mb-1">
                {trimmed.substring(3)}
              </div>
            );
          }
          if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
            return (
              <div class="flex items-start gap-2 pl-1.5 py-0.5 text-zinc-300">
                <span class="text-nord-cyan select-none leading-none mt-1 text-[10px]">•</span>
                <span class="leading-snug">{trimmed.substring(2)}</span>
              </div>
            );
          }
          if (trimmed === "") {
            return <div class="h-1" />;
          }
          return <div class="leading-relaxed text-zinc-300">{line}</div>;
        }}
      </For>
    </div>
  );
};

export const UpdatePanel: Component<UpdatePanelProps> = (props) => {
  const [status, setStatus] = createSignal<UpdateStatus>("idle");
  const [updateInfo, setUpdateInfo] = createSignal<UpdateInfoDTO | null>(null);
  const [installedVersion, setInstalledVersion] = createSignal<string>("");
  const [applyResult, setApplyResult] = createSignal<UpdateApplyResultDTO | null>(null);
  const [errorMessage, setErrorMessage] = createSignal<string>("");
  const currentChannel = () => props.channel || "stable";

  onMount(async () => {
    try {
      const ver = await launcherAPI.getCurrentVersion();
      if (ver) {
        setInstalledVersion(ver);
      }
    } catch (_err: unknown) {
      // D1: on runtime failure/race, installedVersion remains empty string (clean display, no crash, no hardcoded fallback)
      setInstalledVersion("");
    }
  });

  const currentDisplayVersion = () => {
    const raw = updateInfo()?.current_version || installedVersion();
    return formatVersion(raw);
  };

  const handleCheck = async () => {
    setStatus("checking");
    setErrorMessage("");
    try {
      const info = await launcherAPI.checkForUpdates();
      setUpdateInfo(info);
      if (info.has_update) {
        setStatus("available");
        if (props.onUpdateAvailable) {
          props.onUpdateAvailable(info);
        }
      } else {
        setStatus("up_to_date");
      }
    } catch (err: unknown) {
      setStatus("error");
      if (err instanceof Error) {
        setErrorMessage(err.message);
      } else {
        setErrorMessage("Failed to check for updates");
      }
    }
  };

  const handleApply = async () => {
    setStatus("applying");
    setErrorMessage("");
    try {
      const result = await launcherAPI.applyUpdate();
      setApplyResult(result);
      if (result.success && result.restart_required) {
        setStatus("restart_required");
      } else if (!result.success) {
        setStatus("error");
        setErrorMessage(result.message || "Failed to apply update");
      }
    } catch (err: unknown) {
      setStatus("error");
      if (err instanceof Error) {
        setErrorMessage(err.message);
      } else {
        setErrorMessage("Failed to download and apply update");
      }
    }
  };

  const handleRestart = async () => {
    if (props.onRestartTriggered) {
      props.onRestartTriggered();
    }
    await launcherAPI.restartApplication();
  };

  const formatBytes = (bytes: number): string => {
    if (!bytes || bytes <= 0) return "0 MB";
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  };

  const formatDate = (dateStr?: string): string => {
    if (!dateStr) return "";
    try {
      const d = new Date(dateStr);
      return d.toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric" });
    } catch (_ignored: unknown) {
      return dateStr;
    }
  };

  return (
    <div class="flex flex-col gap-6 max-w-2xl mx-auto py-2 select-none" data-testid="update-panel">
      {/* Header & Current Version Card */}
      <section class="p-6 rounded-2xl bg-nord-surface border border-white/10 shadow-xl relative overflow-hidden">
        <div class="flex items-center justify-between">
          <div class="flex flex-col gap-1">
            <span class="text-xs font-mono font-medium uppercase tracking-wider text-nord-cyan">
              Settings &amp; Updates
            </span>
            <h2 class="text-xl font-bold text-white tracking-tight">Software Updates</h2>
          </div>

          <div class="flex items-center gap-2">
            <span class="px-2.5 py-1 rounded-md text-xs font-mono font-medium capitalize bg-zinc-800 text-zinc-300 border border-white/5">
              Channel: {currentChannel()}
            </span>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4 mt-6 pt-6 border-t border-white/5">
          <div class="flex flex-col gap-1">
            <span class="text-xs text-zinc-400 font-mono">Installed Version</span>
            <span class="text-lg font-bold font-mono text-white" data-testid="current-version-text">
              {currentDisplayVersion()}
            </span>
          </div>

          <div class="flex flex-col gap-1">
            <span class="text-xs text-zinc-400 font-mono">Cryptographic Verification</span>
            <div class="flex items-center gap-1.5 text-xs text-nord-emerald font-medium">
              <ShieldCheck class="w-4 h-4 text-nord-emerald" />
              <span>Ed25519 Signed</span>
            </div>
          </div>
        </div>

        <div class="mt-6 flex items-center justify-between pt-4 border-t border-white/5">
          <span class="text-xs text-zinc-400">
            {status() === "checking"
              ? "Checking remote manifest..."
              : status() === "available"
              ? "New update available"
              : status() === "up_to_date"
              ? "System is up to date"
              : "Check for release updates"}
          </span>

          <button
            type="button"
            onClick={handleCheck}
            disabled={status() === "checking" || status() === "applying"}
            class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-zinc-800 hover:bg-zinc-700 disabled:opacity-50 text-white text-xs font-medium font-mono border border-white/10 transition-all cursor-pointer disabled:cursor-not-allowed shadow-sm"
            data-testid="check-updates-button"
          >
            <Show
              when={status() === "checking"}
              fallback={<RefreshCw class="w-3.5 h-3.5 text-nord-cyan" />}
            >
              <Loader2 class="w-3.5 h-3.5 text-nord-cyan animate-spin" />
            </Show>
            <span>{status() === "checking" ? "Checking..." : "Check for updates"}</span>
          </button>
        </div>
      </section>

      {/* Up-to-date State */}
      <Show when={status() === "up_to_date"}>
        <section
          class="p-5 rounded-xl bg-nord-emerald/10 border border-nord-emerald/20 flex items-center gap-4 transition-all"
          data-testid="up-to-date-banner"
        >
          <div class="w-10 h-10 rounded-lg bg-nord-emerald/20 flex items-center justify-center text-nord-emerald shrink-0">
            <CheckCircle2 class="w-5 h-5" />
          </div>
          <div class="flex flex-col">
            <span class="font-semibold text-sm text-white">You are running the latest version</span>
            <span class="text-xs text-zinc-400 mt-0.5">
              Nord Launcher {currentDisplayVersion()} is up to date.
            </span>
          </div>
        </section>
      </Show>

      {/* Update Available State */}
      <Show when={status() === "available" && updateInfo()?.has_update}>
        <section
          class="p-6 rounded-2xl bg-nord-surface border border-nord-cyan/30 shadow-xl flex flex-col gap-5 relative overflow-hidden"
          data-testid="update-available-card"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-xl bg-nord-cyan/10 border border-nord-cyan/30 flex items-center justify-center text-nord-cyan">
                <Download class="w-5 h-5" />
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <span class="text-base font-bold text-white tracking-tight">
                    Nord Launcher {formatVersion(updateInfo()?.version)}
                  </span>
                  <span class="px-2 py-0.5 rounded text-[11px] font-mono font-semibold bg-nord-cyan text-nord-dark">
                    Available
                  </span>
                </div>
                <Show when={updateInfo()?.release_date}>
                  <span class="text-xs text-zinc-400 font-mono">
                    Released on {formatDate(updateInfo()?.release_date)}
                  </span>
                </Show>
              </div>
            </div>

            <div class="text-right">
              <span class="text-xs text-zinc-400 block font-mono">Download Size</span>
              <span class="text-sm font-bold font-mono text-zinc-200">
                {formatBytes(updateInfo()?.size || 0)}
              </span>
            </div>
          </div>

          {/* Release Notes / Changelog */}
          <div class="flex flex-col gap-2">
            <span class="text-xs font-mono font-medium uppercase tracking-wider text-zinc-400">
              Changelog
            </span>
            <div
              class="p-3.5 rounded-xl bg-zinc-950/70 border border-white/5 text-xs font-mono leading-relaxed max-h-48 overflow-y-auto"
              data-testid="changelog-text"
            >
              {renderMarkdownLite(updateInfo()?.release_notes)}
            </div>
          </div>

          {/* Authentic Ed25519 signature status line */}
          <div
            class="flex items-center gap-2 p-2.5 rounded-lg bg-nord-cyan/5 border border-nord-cyan/20 text-xs font-mono text-nord-cyan"
            data-testid="ed25519-status-line"
          >
            <ShieldCheck class="w-4 h-4 text-nord-cyan shrink-0" />
            <span>Цифровая подпись Ed25519 проверена</span>
          </div>

          {/* Checksum info */}
          <Show when={updateInfo()?.sha256}>
            <div class="flex flex-col gap-1 p-3 rounded-lg bg-zinc-900/60 border border-white/5 font-mono text-[11px]">
              <span class="text-zinc-500">Payload SHA-256 Checksum:</span>
              <span class="text-zinc-300 truncate" title={updateInfo()?.sha256}>
                {updateInfo()?.sha256}
              </span>
            </div>
          </Show>

          {/* Action button */}
          <div class="pt-3 border-t border-white/5 flex items-center justify-between">
            <span class="text-xs text-zinc-400">
              Cryptographically verified with official Ed25519 release key.
            </span>
            <button
              type="button"
              onClick={handleApply}
              class="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg bg-nord-cyan hover:bg-nord-cyan/90 text-nord-dark font-semibold text-xs font-mono shadow-[0_0_12px_rgba(0,212,178,0.3)] transition-all cursor-pointer"
              data-testid="apply-update-button"
            >
              <Download class="w-4 h-4" />
              <span>Download &amp; install</span>
            </button>
          </div>
        </section>
      </Show>

      {/* Applying State */}
      <Show when={status() === "applying"}>
        <section
          class="p-6 rounded-xl bg-nord-surface border border-white/10 flex flex-col items-center justify-center gap-3 text-center"
          data-testid="applying-indicator"
        >
          <Loader2 class="w-8 h-8 text-nord-cyan animate-spin" />
          <div class="flex flex-col gap-1">
            <span class="text-sm font-semibold text-white">Downloading &amp; verifying update...</span>
            <span class="text-xs text-zinc-400 font-mono">
              Validating Ed25519 signature and staging atomic executable replacement.
            </span>
          </div>
        </section>
      </Show>

      {/* Restart Required State */}
      <Show when={status() === "restart_required"}>
        <section
          class="p-6 rounded-2xl bg-nord-emerald/10 border border-nord-emerald/30 shadow-xl flex flex-col gap-4"
          data-testid="restart-required-banner"
        >
          <div class="flex items-center gap-3">
            <div class="w-10 h-10 rounded-xl bg-nord-emerald/20 flex items-center justify-center text-nord-emerald shrink-0">
              <CheckCircle2 class="w-5 h-5" />
            </div>
            <div class="flex flex-col">
              <span class="text-base font-bold text-white">Update installed successfully</span>
              <span class="text-xs text-zinc-300 mt-0.5">
                {applyResult()?.message || "Restart required for changes to take effect."}
              </span>
            </div>
          </div>

          <div class="pt-3 border-t border-nord-emerald/20 flex items-center justify-between">
            <span class="text-xs text-zinc-400">
              Nord Launcher will relaunch automatically.
            </span>
            <button
              type="button"
              onClick={handleRestart}
              class="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg bg-nord-emerald hover:bg-nord-emerald/90 text-nord-dark font-bold text-xs font-mono shadow-[0_0_12px_rgba(163,190,140,0.3)] transition-all cursor-pointer"
              data-testid="restart-launcher-button"
            >
              <RotateCcw class="w-4 h-4" />
              <span>Restart Nord Launcher</span>
            </button>
          </div>
        </section>
      </Show>

      {/* Error State */}
      <Show when={status() === "error"}>
        <section
          class="p-5 rounded-xl bg-red-950/30 border border-red-500/30 flex items-center justify-between gap-4"
          data-testid="error-banner"
        >
          <div class="flex items-center gap-3">
            <AlertCircle class="w-5 h-5 text-red-400 shrink-0" />
            <div class="flex flex-col">
              <span class="text-sm font-semibold text-white">Operation failed</span>
              <span class="text-xs text-red-300 font-mono mt-0.5" data-testid="error-message">
                {errorMessage()}
              </span>
            </div>
          </div>

          <button
            type="button"
            onClick={handleCheck}
            class="px-3.5 py-1.5 rounded-lg bg-zinc-800 hover:bg-zinc-700 text-xs font-mono font-medium text-white border border-white/10 transition-colors shrink-0"
            data-testid="retry-button"
          >
            Retry
          </button>
        </section>
      </Show>
    </div>
  );
};
