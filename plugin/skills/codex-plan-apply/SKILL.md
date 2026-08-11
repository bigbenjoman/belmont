---
name: codex-plan-apply
description: Apply a Codex-only Belmont planning handoff packet after a plan-mode product-plan, ux-design or tech-plan interview; write only the explicit .belmont files named in the packet without asking new planning questions.
alwaysApply: false
---

# Belmont: Codex Plan Apply

Use this skill only when the user provides a `BELMONT_PLAN_PACKET` produced by a Codex plan-mode Belmont planning session.

This is a Codex compatibility adapter. Other agents that can ask structured questions and write files in the same session should keep using `product-plan`, `ux-design` and `tech-plan` directly.

## Purpose

Codex plan mode provides keyboard-navigable structured questions, but may not be the right phase for writing planning files. This skill runs after the user leaves plan mode. It persists the already-approved planning output without reopening the interview.

## Rules

1. Do NOT ask product or technical planning questions.
2. Do NOT infer missing PRD, PROGRESS, TECH_PLAN, UX_DESIGN, or models.yaml content.
3. Do NOT edit source code. **This prohibition is about project source, not file extension.** A planning artifact under `.belmont/` is in scope even when its extension looks like code — specifically `{base}/design-preview.html` and `{base}/ux-flows.html`, the self-contained review pages for the design authority in `{base}/UX_DESIGN.md`. Apply them like any other packet file. What remains forbidden is writing to the project's own `.tsx`/`.ts`/`.css`/etc. files, at any path outside `.belmont/`.
4. Only write files under `.belmont/`.
5. Only write, create, or update files explicitly listed in the packet.
6. Preserve existing content unless the packet explicitly says a file should be replaced.
7. For update operations, edit the named sections only. Never replace a real existing file unless the packet explicitly marks it as a full-file replacement.
8. If the packet is incomplete, ambiguous, malformed, or lacks target paths, stop and ask the user to rerun the relevant Codex plan-mode skill.

## Expected Packet Shape

The packet is plain Markdown in a fenced block:

````markdown
```BELMONT_PLAN_PACKET
kind: product-plan | ux-design | tech-plan
mode: create | update
feature_slug: <slug or none>
base_path: .belmont/features/<slug> | .belmont

files:
  - path: .belmont/features/<slug>/PRD.md
    operation: create | replace | update-section
    content: |
      ...
  - path: .belmont/features/<slug>/PROGRESS.md
    operation: create | replace | update-section
    content: |
      ...

post_apply:
  commit_message: belmont: update planning files after product planning
  next_prompt: /plan $belmont:ux-design <feature>
```
````

YAML-like structure is preferred, but exact YAML parsing is not required. The key requirement is that each file path, operation, and content block is explicit enough to apply safely.

## Workflow

1. Read the packet.
2. Validate that every target path is under `.belmont/`.
3. Validate that no **project source-code** paths are listed. A `.html`, `.css` or `.json` file under `.belmont/` is a planning artifact and is allowed; the same extension outside `.belmont/` is not.
4. Apply the file changes exactly as specified.
5. Run:
   ```bash
   git status --porcelain .belmont/
   ```
6. If `.belmont/` is not git-ignored and the packet includes `post_apply.commit_message`, commit only `.belmont/` changes:
   ```bash
   git check-ignore -q .belmont/ 2>/dev/null
   git add .belmont/
   git commit -m "<packet commit_message>"
   ```
   Skip the commit if `.belmont/` is ignored or there are no `.belmont/` changes.
7. Report the files written and the next prompt from `post_apply.next_prompt`.

## Refusal Cases

Stop without editing if:

- No `BELMONT_PLAN_PACKET` is present.
- Any target path escapes `.belmont/`.
- The packet asks for project source-code changes (any path outside `.belmont/`).
- The packet asks you to make new planning decisions.
- The packet conflicts with existing files and does not state whether to update or replace.
