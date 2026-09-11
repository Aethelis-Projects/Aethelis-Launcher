# ADR-0009: Windows-First Development + Linux CI Matrix
## Status: Accepted
## Context
Primary player base and dev environment is Windows. macOS is paused.
## Decision
Core library headless unit tests run on [windows-latest, ubuntu-latest] from M0. Linux GUI verified in M5. Linux AppImage/tar.gz packaging in M6.
