import { Component } from "solid-js";
import { Box, Play, Clock, AlertCircle } from "lucide-solid";
import { InstanceDTO } from "../../bindings/ipc_types";

export interface InstanceCardProps {
  instance: InstanceDTO;
  selected?: boolean;
  disabled?: boolean;
  onSelect?: (instance: InstanceDTO) => void;
  onLaunch?: (instance: InstanceDTO) => void;
  class?: string;
}

export const InstanceCard: Component<InstanceCardProps> = (props) => {
  const isRunning = () => props.instance.state === "running";
  const isCrashed = () => props.instance.state === "crashed";

  const formatPlayTime = (seconds: number): string => {
    if (!seconds) return "0 ч";
    const hours = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    if (hours > 0) return `${hours} ч ${mins} м`;
    return `${mins} м`;
  };

  return (
    <div
      role="button"
      tabIndex={props.disabled ? -1 : 0}
      onClick={() => !props.disabled && props.onSelect?.(props.instance)}
      onKeyDown={(e) => {
        if (!props.disabled && (e.key === "Enter" || e.key === " ")) {
          e.preventDefault();
          props.onSelect?.(props.instance);
        }
      }}
      class={`group relative flex items-center justify-between p-4 rounded-xl border transition-all duration-150 select-none outline-none ${
        props.disabled
          ? "opacity-40 bg-zinc-900/50 border-white/5 cursor-not-allowed"
          : props.selected
          ? "bg-nord-card-hover border-nord-cyan shadow-[0_0_20px_rgba(0,212,178,0.1)] ring-1 ring-nord-cyan/50"
          : "bg-nord-card border-white/5 hover:border-white/15 hover:bg-nord-card-hover cursor-pointer"
      } focus-visible:ring-2 focus-visible:ring-nord-cyan ${props.class || ""}`}
      data-testid="instance-card"
      aria-selected={props.selected}
    >
      <div class="flex items-center gap-3.5">
        <div
          class={`w-11 h-11 rounded-lg flex items-center justify-center border transition-colors ${
            props.selected
              ? "bg-nord-cyan/15 border-nord-cyan/40 text-nord-cyan"
              : "bg-zinc-800/80 border-white/5 text-zinc-400 group-hover:text-zinc-200"
          }`}
        >
          <Box class="w-5 h-5 stroke-[1.5]" aria-hidden="true" />
        </div>

        <div class="flex flex-col gap-1">
          <div class="flex items-center gap-2">
            <span class="font-medium text-sm text-zinc-100 group-hover:text-white tracking-tight">
              {props.instance.name}
            </span>
            {isRunning() && (
              <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-mono font-medium bg-nord-emerald/15 text-nord-emerald border border-nord-emerald/30">
                <span class="w-1.5 h-1.5 rounded-full bg-nord-emerald animate-pulse" />
                В игре
              </span>
            )}
            {isCrashed() && (
              <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-mono font-medium bg-nord-rose/15 text-nord-rose border border-nord-rose/30">
                <AlertCircle class="w-3 h-3" />
                Вылет
              </span>
            )}
          </div>

          <div class="flex items-center gap-2 text-xs text-zinc-400">
            <span class="font-mono text-zinc-300 px-1.5 py-0.2 rounded bg-zinc-800 border border-white/5">
              {props.instance.game_version}
            </span>
            <span class="capitalize font-mono text-[11px] text-zinc-400">
              {props.instance.loader}
            </span>
            <span class="text-zinc-600">•</span>
            <span class="flex items-center gap-1 text-[11px] text-zinc-500 font-mono">
              <Clock class="w-3 h-3" />
              {formatPlayTime(props.instance.total_play_seconds)}
            </span>
          </div>
        </div>
      </div>

      <button
        type="button"
        tabIndex={-1}
        onClick={(e) => {
          e.stopPropagation();
          if (!props.disabled) props.onLaunch?.(props.instance);
        }}
        class={`p-2 rounded-lg transition-all ${
          props.selected
            ? "bg-nord-cyan text-nord-dark hover:bg-nord-cyan-hover"
            : "text-zinc-400 hover:text-white hover:bg-white/10"
        }`}
        aria-label={`Запустить ${props.instance.name}`}
      >
        <Play class="w-4 h-4 fill-current stroke-[2]" />
      </button>
    </div>
  );
};