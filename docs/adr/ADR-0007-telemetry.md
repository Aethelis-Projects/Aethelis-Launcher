# ADR-0007: Strict Opt-In Sentry Crash Reporting with PII Redaction
## Status: Deferred to M7
## Context
Gamers are sensitive to telemetry. Need actionable crash reporting while preserving privacy.
## Decision
Crash reporting is strictly opt-in (default OFF). All local filesystem paths, user accounts, and session tokens are redacted before sending. Structured logs saved locally in <dataDir>/logs.
## Notes
PII log sanitization (`SanitizeLogs`) is implemented and enforced in core. Remote GlitchTip/Sentry client transport integration is deferred to milestone M7 to focus the initial v0.1.0 release strictly on local offline stability and zero network telemetry.
