# ADR-0006: Windows Authenticode Code Signing via SignPath Foundation
## Status: Accepted
## Context
SmartScreen false positives block new binaries. Commercial EV certs cost +/year.
## Decision
Apply for free open-source EV code signing via SignPath Foundation in M0. CI uses self-signed certificates during development (M0-M4). macOS notarization is paused.
