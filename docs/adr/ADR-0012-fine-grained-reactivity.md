# ADR-0012: Elimination of Source-Guard Discipline via Fine-Grained Signals
## Status: Accepted
## Context
In Aethel, whole-store re-renders were fought using brittle runtime tests (source-guard).
## Decision
SolidJS signals bind directly to exact DOM nodes at compile time, eliminating whole-store subscription bugs by design. source-guard test discipline is deleted as a class.
