# ADR-0001: Selection of Desktop Shell Framework - Wails v3 (beta.20)
## Status: Accepted
## Context
A desktop application shell was needed for a high-performance Go-based launcher. Wails v2 is stable but has limitations around native multi-window, menu bars, and clean IPC.
## Decision
Pin Wails v3 to github.com/wailsapp/wails/v3@v3.0.0-beta.20 in go.mod. Isolate all Wails interactions strictly inside internal/adapters/wails.
## Rollback
If fatal packaging blockers occur on Windows CI, fallback to Wails v2.3+ by replacing the adapter.
