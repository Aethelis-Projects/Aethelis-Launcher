# ADR-0017: Tailwind CSS Version Strategy (Retaining v3.4 for v0.1.0)

## Status: Accepted

## Context
ADR-0011 references modern Tailwind CSS for the frontend UI. Tailwind v4 was released in early 2025 with an overhauled engine. The project's existing UI workbench was constructed and rigorously validated against Tailwind v3.4.17, incorporating custom `@layer` utilities, Zinc color tokens, custom scrollbars, and strict Hallmark design constraints.

## Decision
Retain Tailwind CSS `v3.4.17` for the initial `v0.1.0` release. Migration to Tailwind v4 is formally deferred to `v0.2.0`.

## Rationale
1. **Zero Regression Risk**: The current SolidJS frontend compiles to a verified bundle of 27.70 KB gzip, respecting the $\le 250$ KB budget by a 9x margin.
2. **Tooling Predictability**: Vite 6 and Tailwind v3.4 integration is rock-solid across Windows and Linux CI runners. Upgrading CSS engines immediately before release introduces unneeded visual and build risk without end-user value for `v0.1.0`.
3. **Migration Window**: A dedicated migration to Tailwind v4 will be conducted in the `v0.2.0` cycle alongside the i18n dynamic switcher.
