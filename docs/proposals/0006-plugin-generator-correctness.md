# 0006 — Plugin generator correctness

**Type:** bug fix — one bash script, plus the regenerated `plugin/` tree it produces.
**Size:** ~15 lines of awk, one new copy loop, one `--check` argument. Large regenerated diff.
**Sequencing:** **lands first.** 0003, 0004 and 0005 all consume a working generator.

---

## 1. Problem — this is shipping broken today

`plugin/agents/` on `blake-simpson/belmont@main` (5f1c2c2), verified via the GitHub API and `raw.githubusercontent.com` independently:

| File | Published size | Source |
|---|---|---|
| `code-review-agent.md` | **0 bytes** | 268 lines |
| `codebase-agent.md` | **0 bytes** | 215 lines |
| `reconciliation-agent.md` | **0 bytes** | 128 lines |
| `verification-agent.md` | **0 bytes** | 363 lines |
| `design-agent.md` | 5,101 bytes (118 lines) | 276 lines |
| `implementation-agent.md` | 5,067 bytes (102 lines) | 427 lines |

Four of six plugin agents are **empty files in the published repository**. Anyone installing Belmont via the plugin distribution gets no verification agent, no code-review agent, no codebase agent and no reconciliation agent — and a design agent whose `FORBIDDEN ACTIONS` block is absent (`grep -c` returns 0 in the plugin copy, 1 in the source).

`scripts/release.sh:39-41` regenerates and `git add`s `plugin/`, and `.github/workflows/release.yml:66-70` tars it into `belmont-plugin-<tag>.tar.gz`. Every release since the bug landed has shipped this.

## 2. Root cause

`scripts/generate-plugin.sh:84-108`. The agents loop transforms frontmatter with awk:

```awk
BEGIN { in_fm=0; fm_done=0; first_line=1; got_desc=0 }
{
    if (first_line && $0 == "---") { in_fm=1; first_line=0; next }
    ...
    if (fm_done) { print $0 }
}
```

`first_line` is cleared **only inside** the branch that requires line 1 to be `---`. **No source agent has frontmatter** — every one begins `# Belmont: …` (verified across all six). So `first_line` stays `1` forever, the guard remains live for every subsequent line, and the first `---` *anywhere in the body* latches frontmatter mode.

Nothing prints unless `fm_done` is true. Therefore:

- **No `^---$` anywhere in the body** → `fm_done` never set → **zero output**. Explains all four empty files.
- **Body `---` markers present** → latches at the first, closes at the second, prints only what follows. `design-agent.md` has them at 160/225 → 276−225 = 51 body lines + injected frontmatter → **118 lines**. `implementation-agent.md` has them at 327/362 → **102 lines**. Both match the published sizes exactly.

`--check` cannot detect any of this: it regenerates into a temp dir with the *same* broken awk and diffs that against the committed tree, so both sides are equally wrong. On a clean `main` the only reported difference is `STALE: plugin.json`.

**When it broke.** The awk logic is unchanged since at least April, so the flaw was dormant for months — every agent happened to begin with `---\nmodel: …\n---`, which satisfied the line-1 guard invisibly. `ad5f84c` (PR #18, "Pin no model in agent frontmatter", 2026-06-04) removed that frontmatter from all six agents. **PR #18 is not itself wrong** — agent frontmatter genuinely is not read at runtime in either invocation path, so removing a pin that pins nothing was correct. It touched neither `scripts/generate-plugin.sh` nor `plugin/`, so there was no review-time signal; the damage materialised at the next release (`f69e784`, v0.10.12, same day) when `release.sh` regenerated the tree and `plugin/agents/verification-agent.md` went 22,431 bytes → 0. The real defect is an undocumented coupling between two files with no test spanning them — which is why the DoD asserts plugin line count equals source + 3.

**Second, latent bug.** `for (i in fm_lines) print fm_lines[i]` iterates in awk's unspecified order. Harmless today because no source agent has frontmatter to reorder — but wrong the moment one does, which 0003 makes likely.

**Third bug.** `--check` compares against a `plugin.json` stamped with `VERSION`, which defaults to `dev` (`generate-plugin.sh:20`) while the committed file says `0.10.14`. So a bare `./scripts/generate-plugin.sh --check` **exits 1 on unmodified main** — meaning any DoD requiring it to pass is unsatisfiable without committing `"version": "dev"` into a tracked release artifact.

## 3. Rationale from the graph-engineering method

**The cheapest skeptic is a mechanical one (§3).** `--check` exists precisely to be the gate that catches generator drift, and it has been structurally incapable of catching the largest possible drift — total content loss — because it compares a broken generator against its own broken output. A checker that cannot fail is not a checker.

**Rare gates stay read (§8).** `--check` currently reports `STALE: plugin.json` on every clean run. A gate that always shows a benign failure trains everyone to ignore it, which is how four empty files reached production undetected.

**Bottleneck-at-a-time (§9).** This is the shared prerequisite: 0003 edits agents mirrored into `plugin/`, 0004 edits skills mirrored into `plugin/`, 0005 gates CI on `--check`. Fixing the generator once unblocks all three.

## 4. Design

### 4.1 The awk fix — verified before specifying

```awk
BEGIN { in_fm=0; fm_done=0; fm_count=0 }
NR==1 && $0 == "---"  { in_fm=1; next }
NR==1 && $0 != "---"  {
    print "---"; print "name: " AGENT_NAME; print "---"
    fm_done=1; print $0; next
}
in_fm && $0 == "---" {
    in_fm=0; fm_done=1
    print "---"; print "name: " AGENT_NAME
    for (i=1; i<=fm_count; i++) print fm_lines[i]
    print "---"
    next
}
in_fm    { fm_lines[++fm_count] = $0; next }
fm_done  { print $0 }
```

Three changes: the first-line decision is keyed on `NR==1` so it cannot re-trigger mid-body; a file without frontmatter gets one synthesised (`---` / `name: <n>` / `---`) and its body printed in full; and the frontmatter replay uses an ordered numeric loop instead of `for..in`.

**Measured output** — every agent now emits exactly `source + 3` lines, the three being the injected frontmatter:

| Agent | Fixed | Source |
|---|---|---|
| code-review-agent | 271 | 268 |
| codebase-agent | 218 | 215 |
| design-agent | 279 | 276 |
| implementation-agent | 430 | 427 |
| reconciliation-agent | 131 | 128 |
| verification-agent | 366 | 363 |

**Control — a file that *does* have frontmatter** still round-trips, with `name:` injected first and the existing key preserved in order:

```
---
name: probe
description: test desc
---
# Body
content
```

`FORBIDDEN ACTIONS` is present in the fixed `design-agent.md` output (`grep -c` = 1).

### 4.2 Agents `references/` branch

The skills loop copies `references/` (`generate-plugin.sh:70-73`); the agents loop does not. 0003 introduces `agents/belmont/references/design-no-figma.md`, which would therefore never reach `plugin/`. Mirror the skills-loop branch into the agents loop.

### 4.3 Version-aware `--check`

Make `--check` compare `plugin.json` on every field except `version`, or require the version argument and document it. Either way `./scripts/generate-plugin.sh --check 0.10.14` must exit 0 on unmodified `main` — verified that it does today, while the bare form exits 1.

Preferred: ignore `version` in the `--check` comparison, so the gate is honest without callers needing to know the current release number. State the choice in the PR description; a reviewer may prefer explicitness over convenience.

## 5. Scope

| File | Change |
|---|---|
| `scripts/generate-plugin.sh` | awk fix (§4.1), agents `references/` branch (§4.2), version-aware `--check` (§4.3) |
| `plugin/agents/*.md` | regenerated — 4 files go 0 → full, 2 go partial → full |
| `plugin/skills/**` | regenerated; expect no change beyond `plugin.json` |
| `knowledge/cross-cutting/skill-format.md` | amend — `--check` semantics and the agents-frontmatter contract |

**Out of scope:** adding frontmatter to source agents (the generator synthesises it); any agent or skill content change; any Go change.

## 6. Both invocation paths

Neither auto nor interactive mode reads `plugin/` — they read `.agents/` and `.claude/`, which are produced by `belmont install` and are unaffected. **The plugin tree is a third distribution surface**, and it is the only one this PR touches. Prove all three are unaffected-or-fixed in §8 rather than asserting it.

## 7. Docs and knowledge

- `docs/directory-structure.md` — if it describes `plugin/`
- `AGENTS.md` — the Skills Generation section, if it documents `--check` semantics
- `knowledge/cross-cutting/skill-format.md` — `Don't re-do` gains: relying on `--check` to catch generator bugs (rejected — it compares a generator against its own output, so it is blind to systematic faults); adding frontmatter to every source agent (rejected — synthesis keeps sources clean)

## 8. Author smoke test

**Step 1 — reproduce the bug on `main`.**
```bash
git checkout main
wc -l plugin/agents/*.md          # expect four 0-line files
grep -c 'FORBIDDEN ACTIONS' plugin/agents/design-agent.md   # expect 0
./scripts/generate-plugin.sh --check; echo "exit=$?"        # expect 1, STALE: plugin.json
```

**Step 2 — regenerate on the fix branch.**
```bash
git checkout fix/plugin-generator && ./scripts/generate-plugin.sh 0.10.14
for f in plugin/agents/*.md; do
  n=$(basename "$f"); printf "%-28s plugin=%s source=%s\n" "$n" \
    "$(wc -l < "$f")" "$(wc -l < "agents/belmont/$n")"
done
```
Expect every plugin file = source + 3. Any file not matching means the awk still drops content.

**Step 3 — content integrity, not just line count.**
```bash
for f in plugin/agents/*.md; do
  diff <(tail -n +4 "$f") "agents/belmont/$(basename "$f")" && echo "OK $(basename "$f")"
done
```
Expect no diff on any file — line counts alone would not catch reordering.

**Step 4 — `--check` is honest.**
```bash
./scripts/generate-plugin.sh --check 0.10.14; echo "exit=$?"   # expect 0
printf '\nINJECTED\n' >> plugin/agents/verification-agent.md
./scripts/generate-plugin.sh --check 0.10.14; echo "exit=$?"   # expect 1
git checkout plugin/
```
The negative case is the point — before this PR, `--check` could not fail for the right reason.

**Step 5 — the other two surfaces are untouched.**
```bash
belmont install --source ~/belmont --project /tmp/pr0-install --no-prompt
diff -r /tmp/pr0-install/.agents/belmont /tmp/pr0-baseline/.agents/belmont
```
Expect no difference against a baseline captured from `main`.

**Step 6 — plugin actually loads.** Install the regenerated plugin into a scratch project and confirm the verification agent is present and non-empty at its plugin path. This is the end-to-end proof that the four empty files are genuinely fixed for a real consumer.

**Step 7 — auto and interactive sanity.** One `belmont auto --from M1 --to M1` and one interactive `/belmont:implement` on a disposable branch, confirming no behaviour change.

## 9. Definition of Done

**Fix**
- [ ] `NR==1` keying — the first-line branch cannot re-trigger mid-body
- [ ] Frontmatter-less agents get `---` / `name: <n>` / `---` synthesised and their body printed in full
- [ ] Frontmatter replay uses an ordered numeric loop, not `for..in`
- [ ] Agents loop copies `references/` like the skills loop
- [ ] `--check` ignores `version` (or requires it) and exits 0 on unmodified `main`

**Evidence**
- [ ] All six `plugin/agents/*.md` = source + 3 lines
- [ ] `diff <(tail -n +4 plugin) source` empty for all six
- [ ] `grep -c 'FORBIDDEN ACTIONS' plugin/agents/design-agent.md` = 1
- [ ] Control file with real frontmatter round-trips, `name:` first, existing keys in order
- [ ] `--check` fails on an injected change (Step 4 negative case)
- [ ] `.agents/` install output byte-identical to a `main` baseline

**Mechanics**
- [ ] `./scripts/generate-skills.sh --check` passes
- [ ] `go build ./cmd/belmont` and `go test ./cmd/belmont` green
- [ ] Regenerated `plugin/` committed; `plugin.json` version handled deliberately, not accidentally
- [ ] All three surfaces exercised (auto, interactive, plugin)
- [ ] `skill-format.md` amended with a `Revisions` line
- [ ] PR description opens with the published-size table and the GitHub API citation

## 10. Risks

| Risk | Mitigation |
|---|---|
| Regenerated diff is huge and hides a real change | Step 3 diffs content against source, so the diff is mechanically verifiable rather than eyeballed |
| `--check` change masks a legitimate version drift | Prefer ignoring only `version`; `release.sh` still stamps it explicitly |
| Synthesised frontmatter conflicts with a future real one | Control case tested; `name:` is injected first and existing keys preserved |
| Plugin consumers cached the broken tree | Note in the PR that a plugin reinstall is needed; consider a release note |

## 11. Relationship to the other proposals

| Proposal | What this unblocks |
|---|---|
| 0003 | Its `agents/belmont/references/` file can reach `plugin/`; its edited agents ship intact rather than truncated |
| 0004 | Its `generate-plugin.sh --check` DoD becomes satisfiable |
| 0005 | Its CI gate on `--check` becomes meaningful rather than always-failing |

Land this first. It is also worth raising on its own merits regardless of whether the other three proceed — it is a shipping defect in the published distribution, not a design question.
