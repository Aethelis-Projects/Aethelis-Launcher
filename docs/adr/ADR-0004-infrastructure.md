# ADR-0004: Zero-Cost Distribution & Cloudflare Edge Gateway
## Status: Accepted
## Context
Need resilient distribution without dedicated server maintenance costs.
## Decision
Binary releases on GitHub Releases. Game/mod assets downloaded directly from Mojang/Modrinth/Adoptium CDNs. Cloudflare Worker used for update manifest caching and optional CF key proxying.
