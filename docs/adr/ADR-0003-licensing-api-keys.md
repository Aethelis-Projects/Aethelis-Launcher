# ADR-0003: Licensing Model (GPLv3) and API Secret Strategy
## Status: Accepted
## Context
CurseForge prohibits public leaks of API keys in open-source repos. Microsoft OAuth uses public client specifications (RFC 8252).
## Decision
License under GPLv3. Official builds embed CF key via CI -ldflags. Code in repo contains a BYOK (Bring Your Own Key) and proxy fallback. MS OAuth Client ID is public with PKCE.
