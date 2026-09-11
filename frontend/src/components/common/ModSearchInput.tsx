import { Component, createSignal, JSX } from "solid-js";
import { Search, X, Loader2 } from "lucide-solid";

export interface ModSearchInputProps {
  value?: string;
  placeholder?: string;
  disabled?: boolean;
  loading?: boolean;
  error?: string;
  matchCount?: number;
  onSearch?: (query: string) => void;
  onClear?: () => void;
  class?: string;
}

export const ModSearchInput: Component<ModSearchInputProps> = (props) => {
  const [internalValue, setInternalValue] = createSignal(props.value || "");

  const handleInput: JSX.EventHandler<HTMLInputElement, InputEvent> = (e) => {
    const val = (e.currentTarget as HTMLInputElement).value;
    setInternalValue(val);
    if (props.onSearch) {
      props.onSearch(val);
    }
  };

  const handleClear = () => {
    setInternalValue("");
    if (props.onClear) {
      props.onClear();
    }
    if (props.onSearch) {
      props.onSearch("");
    }
  };

  return (
    <div class={`flex flex-col gap-1 w-full ${props.class || ""}`}>
      <div
        class={`relative flex items-center w-full px-3.5 py-2 bg-nord-surface rounded-lg border transition-all duration-150 ${
          props.disabled
            ? "opacity-50 border-white/5 cursor-not-allowed"
            : props.error
            ? "border-nord-rose ring-1 ring-nord-rose/40"
            : "border-white/10 hover:border-white/20 focus-within:border-nord-cyan focus-within:ring-2 focus-within:ring-nord-cyan/30"
        }`}
      >
        <div class="text-zinc-400 mr-2.5 flex items-center justify-center pointer-events-none">
          {props.loading ? (
            <Loader2 class="w-4 h-4 animate-spin text-nord-cyan" aria-hidden="true" />
          ) : (
            <Search class="w-4 h-4" aria-hidden="true" />
          )}
        </div>

        <input
          type="text"
          value={props.value !== undefined ? props.value : internalValue()}
          placeholder={props.placeholder || "Поиск модов и сборок..."}
          disabled={props.disabled}
          onInput={handleInput}
          class="w-full bg-transparent text-sm text-zinc-100 placeholder-zinc-500 outline-none select-text"
          data-testid="mod-search-input"
          aria-invalid={!!props.error}
        />

        {internalValue().length > 0 && !props.disabled && (
          <button
            type="button"
            onClick={handleClear}
            class="p-0.5 text-zinc-400 hover:text-zinc-200 rounded transition-colors focus-visible:ring-2 focus-visible:ring-nord-cyan focus-visible:outline-none"
            aria-label="Очистить поиск"
            data-testid="mod-search-clear"
          >
            <X class="w-3.5 h-3.5" />
          </button>
        )}

        {props.matchCount !== undefined && !props.loading && internalValue().length > 0 && (
          <span class="ml-2 px-1.5 py-0.5 text-[11px] font-mono font-medium rounded bg-zinc-800 text-zinc-300 border border-white/5">
            {props.matchCount}
          </span>
        )}
      </div>

      {props.error && (
        <span class="text-xs text-nord-rose font-medium" role="alert">
          {props.error}
        </span>
      )}
    </div>
  );
};