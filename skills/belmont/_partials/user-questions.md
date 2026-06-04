## Asking Questions (MANDATORY)

When you need to ask the user a question:

1. **Use your structured question tool** (e.g. `AskUserQuestion`, or equivalent). This is NON-NEGOTIABLE when such a tool is available.
2. **Ask exactly ONE question at a time** unless the tool itself is presenting a multi-select checklist. Wait for the user's answer before asking the next question.
3. **NEVER print the question as inline text AND use the tool.** The tool call IS the question — do not duplicate it in your response body.
4. **NEVER ask questions as plain inline text** when a structured question tool exists. No "Question 1: ..." followed by more text. Use the tool.
5. **Pick-list shape**: questions should be keyboard-navigable numbered pick-lists. Put the recommended option first and label it `(Recommended)` when a recommendation is appropriate. Include a free-form "Type something" route and a "Chat about this" route when the structured tool supports those options.
6. **Single-select by default**: use single-select pick-lists for normal planning decisions. Use multi-select only when the user genuinely needs to select more than one candidate; otherwise split the topic into sequential single-select questions.
7. **Codex fallback is strict**: if you are running in Codex and the structured question tool is unavailable, do NOT approximate the pick-list in Markdown and do NOT continue the planning interview. Stop immediately and tell the user to restart the skill in Codex plan mode:
   - Product planning: `/plan $belmont:product-plan <brief>`
   - Technical planning: `/plan $belmont:tech-plan <brief or feature>`
8. **Non-Codex fallback**: outside Codex, if no structured question tool exists at all, ask one clearly formatted plain-text question at a time and explicitly note that the structured picker is unavailable.
