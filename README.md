# ask

Unified local CLI for ChatGPT, Claude, Gemini, Grok, and Perplexity using browser cookies (no API keys).

## Install

```bash
go install github.com/kyupark/ask/cmd/ask@latest
```

### Install via Homebrew (macOS)

```bash
brew tap kyupark/tap
brew install kyupark/tap/ask
```

If `ask` is not found, add Go bin to your shell profile:

```bash
export PATH="$HOME/go/bin:$PATH"
```

## Quick Start

```bash
ask chatgpt "hello"
ask claude "hello"
ask gemini "hello"
ask grok "hello"
ask perplexity "hello"
```

```bash
ask all "say hello in one sentence"
ask all -c <id> "follow up"
```

## OpenClaw Skill (included)

This repo includes an OpenClaw skill at `skills/ask`.

Install skill from the CLI:

```bash
ask install-openclaw-skill
```

Install CLI + skill on macOS:

```bash
./scripts/onboard-macos.sh
```
