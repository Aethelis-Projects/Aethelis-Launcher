import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  getWailsCall,
  invokeWails,
  resetWailsCallPromiseForTesting,
} from "./api";

describe("Wails IPC Bridge & invokeWails", () => {
  const originalWails = window.wails;
  const originalGo = window.go;
  const originalEnvMode = import.meta.env.MODE;
  const originalEnvDev = import.meta.env.DEV;

  beforeEach(() => {
    resetWailsCallPromiseForTesting();
    window.wails = undefined;
    window.go = undefined;
  });

  afterEach(() => {
    resetWailsCallPromiseForTesting();
    window.wails = originalWails;
    window.go = originalGo;
    (import.meta.env as unknown as Record<string, unknown>).MODE = originalEnvMode;
    (import.meta.env as unknown as Record<string, unknown>).DEV = originalEnvDev;
  });

  it("invokes Wails v3 runtime call by name when window.wails is present", async () => {
    const mockByName = vi.fn().mockResolvedValue({ id: "inst-1", name: "Test" });
    window.wails = {
      Call: {
        ByName: mockByName,
      },
    };

    const result = await invokeWails("ListInstances", () => []);
    expect(mockByName).toHaveBeenCalledTimes(1);
    expect(mockByName).toHaveBeenCalledWith(
      "github.com/nord-launcher/launcher/internal/adapters/wails.WailsAdapter.ListInstances",
    );
    expect(result).toEqual({ id: "inst-1", name: "Test" });
  });

  it("invokes legacy window.go bindings if present", async () => {
    const mockLegacyFn = vi.fn().mockResolvedValue("1.0.0");
    window.go = {
      wails: {
        WailsAdapter: {
          GetCurrentVersion: mockLegacyFn,
        },
      },
    };

    const result = await invokeWails("GetCurrentVersion", () => "dev-ver");
    expect(mockLegacyFn).toHaveBeenCalledTimes(1);
    expect(result).toBe("1.0.0");
  });

  it("falls back to mock in dev/test environment when bridge is absent", async () => {
    const fallbackFn = vi.fn().mockReturnValue(["mock-instance"]);
    const result = await invokeWails("ListInstances", fallbackFn);
    expect(fallbackFn).toHaveBeenCalledTimes(1);
    expect(result).toEqual(["mock-instance"]);
  });

  it("throws clear connection error in production environment when bridge is absent", async () => {
    (import.meta.env as unknown as Record<string, unknown>).MODE = "production";
    (import.meta.env as unknown as Record<string, unknown>).DEV = false;

    const fallbackFn = vi.fn().mockReturnValue(["mock-instance"]);
    await expect(invokeWails("ListInstances", fallbackFn)).rejects.toThrow(
      "Wails IPC bridge unavailable for ListInstances. Application is not connected to desktop runtime.",
    );
    expect(fallbackFn).not.toHaveBeenCalled();
  });

  it("getWailsCall returns window.wails.Call when already present", async () => {
    const mockCall = { ByName: vi.fn() };
    window.wails = { Call: mockCall };

    const call = await getWailsCall();
    expect(call).toBe(mockCall);
  });
});
