# ADR-0016: i18n Scope Reduction for v0.1.0 Baseline

## Status: Accepted

## Context
The architectural plan initially proposed multi-language support (RU/EN). Implementing full dynamic runtime language switching across all Workbench views, settings, and asynchronous error handling introduces non-trivial bundle overhead and potential UI layout jitter prior to public release stability.

## Decision
For the initial `v0.1.0` release:
1. **Workbench UI**: English is set as the default, unambiguous interface language across all primary navigation rails, catalog cards, and instance managers.
2. **Crash Diagnostics**: The critical diagnostic modal (`CrashModal.tsx`) provides localized explanations and remedy guidance in Russian to assist the primary testing audience during JVM crash triage.
3. **Windows Installer**: The NSIS installer (`build/windows/installer.nsi`) provides native bilingual support (`English` and `Russian` language selection tables).
4. **Dynamic Switcher**: Full dynamic runtime internationalization (i18n) is formalized as post-release backlog item `ISSUE-001` (scheduled for `v0.1.1`).

## Consequences
- Reduces bundle size by ~8 KB gzip.
- Eliminates missing translation key regressions.
- Keeps diagnostic information clear and actionable for testers.
