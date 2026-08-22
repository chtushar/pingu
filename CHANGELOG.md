# Changelog

All notable changes to pingu are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[semantic versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] — 2026-08-22

First usable release: the core loop and CLI.

### Added

- Agent-directory contract: `instructions.md` is a complete agent; optional
  `agent.toml` with a frozen `model` field (unknown fields rejected).
- `pingu init PATH` — safe scaffolding of new agent directories (refuses to
  touch non-empty directories).
- `pingu run PATH` — interactive terminal session and `--message` one-shot
  mode; `/exit`, EOF, and Ctrl-C (double press to exit immediately)
  behavior defined.
- Configuration precedence: flags > `agent.toml` > `PINGU_*` environment >
  defaults. Model references are `provider/model-id`.
- OpenAI provider adapter using the Chat Completions streaming API via the
  standard library; `OPENAI_API_KEY` and `OPENAI_BASE_URL`.
- Bounded runner: maximum model turns, total tool calls, run timeout, tool
  timeout, and tool output caps, with ordered run events
  (`run_started`, `text_delta`, `tool_started`, `tool_finished`, `warning`,
  `error`, `run_finished`).
- Provider-neutral core types (`internal/llm`) so provider wire formats
  never leak beyond adapters.
- Stable CLI exit codes: `0` success, `1` runtime/provider failure, `2`
  usage/config error, `130` interrupted.
- Structured JSON logging to stderr (`LOG_LEVEL`); secrets never logged.
- CI (GitHub Actions) covering build, tests, race tests, vet, and
  formatting on Linux and macOS.

[Unreleased]: https://github.com/chtushar/pingu/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/chtushar/pingu/releases/tag/v0.1.0
