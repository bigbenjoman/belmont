**How to tell you are headless.** Belmont emits no auto-mode marker and the
headless prompt is byte-identical to what a user types, so decide from what you
can observe: **if your invocation prompt is bare programmatic syntax (just
`{{skill_ref}} --feature <slug>` with no human prose), or no structured question
tool is available in this turn, treat this as a non-interactive call.**
