# ADR-0007: Strict Opt-In Sentry Crash Reporting with PII Redaction
## Status: Accepted
## Context
Gamers are sensitive to telemetry. Need actionable crash reporting while preserving privacy.
## Decision
Crash reporting is strictly opt-in (default OFF). All local filesystem paths, user accounts, and session tokens are redacted before sending. Structured logs saved locally in <dataDir>/logs.
