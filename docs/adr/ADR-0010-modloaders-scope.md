# ADR-0010: Phased Mod Loader Rollout
## Status: Accepted
## Context
Forge and NeoForge require complex jar patching processors (install-profile.json), while Vanilla, Fabric, and Quilt use clean REST/JSON manifests.
## Decision
M2 implements Vanilla, Fabric, and Quilt. M4 implements NeoForge and Forge with full jar patch processors.
