# ADR-0011: Greenfield SolidJS Frontend with M0-Spike Exit Gates
## Status: Accepted
## Context
Aethel frontend suffered from whole-store subscription leaks (F-HIGH-1) and an 810KB bundle.
## Decision
Greenfield SolidJS + TypeScript + Tailwind 4. Fine-grained reactivity without VDOM. Strict bundle budget <= 250KB gzip. M0-spike exit gate determines whether SolidJS succeeds or falls back to React 19 greenfield within 1 day.
