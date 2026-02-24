# chatmux

Unified local CLI for ChatGPT, Claude, Gemini, Grok, and Perplexity using browser cookies (no API keys).

## Install

```bash
go install ./cmd/chatmux
```

### Install via Homebrew (macOS)

```bash
brew tap kyupark/tap
brew install kyupark/tap/chatmux
```

If `chatmux` is not found, add Go bin to your shell profile:

```bash
export PATH="$HOME/go/bin:$PATH"
```

## Quick Start

```bash
chatmux chatgpt ask "hello"
chatmux claude ask "hello"
chatmux gemini ask "hello"
chatmux grok ask "hello"
chatmux perplexity ask "hello"
```

```bash
chatmux ask-all "say hello in one sentence"
chatmux ask-all -c <id> "follow up"
```

## OpenClaw Skill (included)

This repo includes an OpenClaw skill at `skills/chatmux`.

Install skill from the CLI:

```bash
chatmux install-openclaw-skill
```

Install CLI + skill on macOS:

```bash
./scripts/onboard-macos.sh
```
