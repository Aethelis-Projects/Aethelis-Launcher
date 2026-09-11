# ADR-0014: NSIS Packaging for Windows 10/11 64-bit Installer

## Status: Accepted (Updates packaging technology in ADR-0002 and ADR-0006)

## Context
Initial planning referenced Inno Setup for Windows installer creation. However, Inno Setup has limited cross-platform CI tooling on non-Windows hosts and heavier automation requirements. NSIS (Nullsoft Scriptable Install System) has universal CI support (`makensis`), native 64-bit support (`x64.nsh`), and built-in Windows version gates (`WinVer.nsh`).

## Decision
Adopt NSIS (`build/windows/installer.nsi`) for packaging the Windows release:
1. **Operating System Gate**: Checked via `${AtLeastWin10}` and `${RunningX64}` during `.onInit`. Execution aborts with an informative dialog if running on 32-bit Windows or legacy Windows (7, 8, 8.1).
2. **Per-User Installation**: Installs to `$LOCALAPPDATA\Programs\NordLauncher` (`RequestExecutionLevel user`), avoiding unnecessary UAC elevation prompts during installation and updates.
3. **Registry & Add/Remove Programs**: Full integration with `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\NordLauncher`, providing clean uninstallation and display metadata.
4. **CI Integration**: Headless compilation via GitHub Actions `joncloud/makensis-action@v4`.
