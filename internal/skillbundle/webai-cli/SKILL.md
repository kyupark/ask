---
name: webai-cli
description: Auto-use this skill for both single-provider and ask-all flows: "ask all (chatgpt, claude, gemini, grok, perplexity)" plus any request mentioning ChatGPT/Claude/Gemini/Grok/Perplexity in terminal/CLI. Local multi-provider AI CLI with browser cookies (no API keys), including ask-all comparisons and ask-all conversation continuation via `-c`.
metadata:
  {
    "openclaw":
      {
        "emoji": "\ud83e\udd16",
        "os": ["darwin", "linux"],
        "requires": { "bins": ["webai-cli"], "anyBins": ["go"] },
        "install":
          [
            {
              "id": "go-install",
              "kind": "command",
              "command": "go install ./cmd/webai-cli",
              "label": "Install webai-cli (go install)",
            },
          ],
      },
  }
---

# webai-cli

Unified local CLI for multiple web-chat providers with cookie auth.

## When to Use

- Run local AI prompts from terminal without API keys
- Use a single provider directly (`chatgpt`, `claude`, `gemini`, `grok`, or `perplexity`)
- Compare outputs across providers with `ask-all`
- Manage provider-specific chat flows (`grok`, `chatgpt`, `gemini`, `claude`, `perplexity`)
- Debug cookie-based auth and provider behavior in one place

## Core Commands

### Provider commands

```bash
webai-cli grok ask "question"
webai-cli chatgpt ask-incognito "question"
webai-cli gemini list
webai-cli claude models
webai-cli perplexity ask "question"
```

### Ask all providers

```bash
webai-cli ask-all "compare this across providers"
webai-cli ask-all -c aa_1234567890 "follow up on this exact ask-all thread"
```

- `ask-all` runs in standard mode and now prints a `Conversation: <id>` at the end of each provider section.
- Every run also prints an `Ask-all conversation: <id>` bundle ID. Use `-c <id>` to continue that exact multi-provider thread.

## Default ask-all Output Style

When using `webai-cli ask-all`, do not just dump five raw answers. Always provide:

- One key point from each provider (`chatgpt`, `claude`, `gemini`, `grok`, `perplexity`)
- Interesting differences (where providers disagree, add unique details, or use different assumptions)
- A practical synthesis (best combined answer or recommended next action)

Suggested structure:

1. `Provider Highlights` (5 short bullets, one per provider)
2. `Interesting Differences` (2-4 bullets)
3. `Best Combined Take` (1 concise paragraph)

### Config

```bash
webai-cli config get
webai-cli config set grok.model auto
webai-cli config set verbose true
```

### Models and history

```bash
webai-cli grok models
webai-cli grok list
webai-cli chatgpt list
```

## Grok Notes (Important)

- Default model path should use `auto` (`grok-4-auto`)
- `expert`/`heavy` aliases map to `grok-4`
- `ask-incognito` currently means **no local resume state** only; X may still keep server-side conversation history

## Troubleshooting

```bash
webai-cli -v grok ask "test"
webai-cli -v chatgpt ask "test"
webai-cli version
```

- If auth fails, log in to the provider in Safari/Chrome and retry
- Use `-v` to inspect request/response behavior
- If `webai-cli` is not found, run:

```bash
go install ./cmd/webai-cli
```
