import { Component, JSX } from "solid-js";
import { Play, Loader2, Check, AlertTriangle } from "lucide-solid";

export type LaunchButtonState = "default" | "loading" | "error" | "success";

export interface LaunchButtonProps {
  state?: LaunchButtonState;
  disabled?: boolean;
  errorMessage?: string;
  onClick?: () => void;
  onRetry?: () => void;
  class?: string;
}

export const LaunchButton: Component<LaunchButtonProps> = (props) => {
  const currentState = () => props.state || "default";

  const getButtonStyles = (): string => {
    const base =
      "relative inline-flex items-center justify-center gap-2.5 px-6 py-3 rounded-lg font-semibold text-sm tracking-wide transition-all duration-150 ease-out whitespace-nowrap select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-nord-dark";

    if (props.disabled) {
      return `${base} bg-zinc-800 text-zinc-500 cursor-not-allowed opacity-50`;
    }

    switch (currentState()) {
      case "loading":
        return `${base} bg-nord-cyan/80 text-nord-dark cursor-wait focus-visible:ring-nord-cyan`;
      case "success":
        return `${base} bg-nord-emerald text-zinc-950 shadow-md focus-visible:ring-nord-emerald`;
      case "error":
        return `${base} bg-nord-rose text-white shadow-md active:scale-[0.98] focus-visible:ring-nord-rose`;
      case "default":
      default:
        return `${base} bg-nord-cyan hover:bg-nord-cyan-hover text-nord-dark shadow-[0_0_15px_rgba(0,212,178,0.15)] active:scale-[0.98] focus-visible:ring-nord-cyan`;
    }
  };

  const handleClick: JSX.EventHandler<HTMLButtonElement, MouseEvent> = (e) => {
    e.preventDefault();
    if (props.disabled || currentState() === "loading") {
      return;
    }
    if (currentState() === "error" && props.onRetry) {
      props.onRetry();
      return;
    }
    if (props.onClick) {
      props.onClick();
    }
  };

  return (
    <div class="flex flex-col items-start gap-1.5">
      <button
        type="button"
        class={`${getButtonStyles()} ${props.class || ""}`}
        disabled={props.disabled}
        onClick={handleClick}
        aria-busy={currentState() === "loading"}
        aria-live="polite"
        data-testid="launch-button"
      >
        {currentState() === "loading" && (
          <Loader2 class="w-4 h-4 animate-spin stroke-[2]" aria-hidden="true" />
        )}
        {currentState() === "success" && (
          <Check class="w-4 h-4 stroke-[2.5]" aria-hidden="true" />
        )}
        {currentState() === "error" && (
          <AlertTriangle class="w-4 h-4 stroke-[2]" aria-hidden="true" />
        )}
        {currentState() === "default" && (
          <Play class="w-4 h-4 fill-current stroke-[2]" aria-hidden="true" />
        )}

        <span>
          {currentState() === "loading" && "Запуск игры..."}
          {currentState() === "success" && "Игра запущена"}
          {currentState() === "error" && "Повторить запуск"}
          {currentState() === "default" && "Запустить игру"}
        </span>
      </button>

      {currentState() === "error" && props.errorMessage && (
        <span class="text-xs text-nord-rose font-medium mt-0.5" role="alert" data-testid="launch-error-msg">
          {props.errorMessage}
        </span>
      )}
    </div>
  );
};