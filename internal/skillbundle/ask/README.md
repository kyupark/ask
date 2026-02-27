# ask

Use this skill whenever the user wants to run provider chat from CLI, either single-provider or all-provider, especially:

- ChatGPT
- Claude
- Gemini
- Grok
- Perplexity

This skill is for local `ask` usage with browser cookies (no API key setup).

## Auto Trigger Guidance

Treat requests like these as a direct match for this skill:

- "chatgpt로 물어봐"
- "claude로 답해줘"
- "gemini 써서 테스트"
- "grok command 실행"
- "perplexity로 검색해"
- "all 돌려"
- "all providers 비교해줘"
- "ask로 답변 받아"

If the user mentions one or more of `chatgpt`, `claude`, `gemini`, `grok`, `perplexity`, or `all`, use this skill first.

Single-provider is fully supported. All-provider is optional.

## Core Commands

```bash
ask chatgpt "question"
ask claude "question"
ask gemini "question"
ask grok "question"
ask perplexity "question"
```

```bash
ask all "compare providers"
```

`all` prints:

- per-provider `Conversation: <id>`
- bundle `All conversation: <id>`

Default usage pattern (recommended): after running `all`, summarize:

- key point from each provider
- interesting differences/conflicts
- best combined conclusion

Continue exact multi-provider thread:

```bash
ask all -c <id> "follow up"
```

Install this OpenClaw skill bundle directly:

```bash
ask install-openclaw-skill
```
