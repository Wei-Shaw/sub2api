# Changelog

## Unreleased

- Added a config script download action to each API key row, matching the existing API key action area.
- Added a config script dropdown with Codex CLI, Claude Code, and OpenCode options, styled after the provided reference image.
- Added automatic OS detection so macOS downloads `.sh` scripts and Windows downloads `.bat` scripts.
- Added config script generation for Codex CLI, Claude Code, and OpenCode with the current API endpoint and API key injected into the generated files.
- Set the generated script site name to `look2eye`.
- Added Chinese and English i18n text for the config script button, menu, hints, and download states.
- Added focused unit coverage for config script generation and client availability rules.
