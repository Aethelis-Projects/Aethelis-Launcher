# ADR-0002: Target OS - Windows 10/11 & Linux, EOL Windows Cut
## Status: Accepted
## Context
Supporting legacy Windows (7, 8, 8.1) introduces obsolete API shims, Go <=1.20 lock-in, and bloats installers with offline WebView2 packages (>180MB).
## Decision
Strictly target Windows 10 (1809+) and Windows 11 (64-bit), plus modern Linux (Ubuntu 22.04+). Inno Setup enforces MinVersion=10.0.17763. Zero legacy shims in Go.
