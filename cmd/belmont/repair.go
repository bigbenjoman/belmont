package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// `belmont repair` — the healer.
//
// #27 and #31 made a corrupted PROGRESS.md legible. Nothing fixed one. Repos
// already carrying the damage were left diagnosed and unfixable, and the agents
// working in them still had no idea what to do with a `[?]`. That gap is what
// this command closes.
//
// Two tiers, the same shape as `belmont reverify`:
//
//  1. MECHANICAL — pure Go, zero tokens. Every finding that carries a task ID
//     is checked against the commit log with the same primitive
//     `runEvidenceCheck` uses to demote an unearned `[v]`. A commit naming the
//     ID proves the work happened, so the marker becomes `[x]`.
//  2. REVIEW — an agent reads what survived against the current code. That is
//     usually enough to separate "still outstanding" from "superseded": if the
//     route, component or spec the task names is gone, the task is moot.
//
// EVIDENCE-FIRST, NEVER MEMORY-FIRST. The obvious implementation of this
// command interrogates the user — "did this ship? was it withdrawn? was it
// blocked?" — and it is worse than useless. These arrive dozens at a time and
// nobody remembers months later; you get "I don't know" followed by a guess,
// which is issue #27 again with extra steps. Only what survives both tiers is
// ever put to a human, and by then the question is grounded in what the
// repository says today.
//
// Bounds, all enforced in validateRepairPlans:
//
//   - The action set is CLOSED (the causes are not — they travel as a reason
//     string). set_marker / move_milestone / withdraw / leave / escalate.
//   - Capped at `[x]`. Repair never mints `[v]`: that flip has its own evidence
//     contract and `belmont reverify` is the only thing that may write it.
//   - Withdrawal is `[-]` plus the reason in `## Decisions Log`, NEVER a
//     deletion — a deleted line does not survive `mergeProgressState`, which
//     takes the worktree as base and carries master's missing lines back in.
//   - Milestone structure is immutable: repair never creates, renames or
//     removes a `### M<n>:` heading. Moving a task BETWEEN existing milestones
//     is allowed here and only here, because repair runs interactively outside
//     the auto loop — `runScopeGuard` is called from `executeLoopAction`
//     (auto_loop.go) and from nowhere else, so it is not running.
//   - Repair may only touch lines it flagged, and only if they are still
//     byte-for-byte what it scanned.

// The closed action set. Adding a member here means teaching
// validateRepairPlans what it may do and applyRepairPlans how to write it —
// which is the point of keeping the set closed and the causes open.
const (
	// repairSetMarker writes a different marker on the same line, in place.
	repairSetMarker = "set_marker"
	// repairMoveMilestone relocates the line under the milestone its ID names.
	repairMoveMilestone = "move_milestone"
	// repairWithdraw writes `[-]` and records why in `## Decisions Log`.
	repairWithdraw = "withdraw"
	// repairLeave means "this is not a task" — a retro bullet, a quoted log
	// line. No edit; recorded so the report says why nothing happened.
	repairLeave = "leave"
	// repairEscalate means the evidence does not settle it. No edit.
	repairEscalate = "escalate"
)

// repairFinding is one line repair is allowed to act on.
type repairFinding struct {
	Rule      string `json:"rule"`
	TaskID    string `json:"task_id,omitempty"`
	Milestone string `json:"milestone,omitempty"`
	// NamedMilestone is the milestone the task ID itself names — "M2" for
	// P3-M2-1. Empty when the ID names none.
	NamedMilestone string `json:"named_milestone,omitempty"`
	// AlsoUnverified marks a parse finding whose line is ALSO a
	// verified-without-evidence audit finding. One line, one finding, one
	// action — so the second fact travels on the first finding rather than as a
	// second entry the gate would refuse.
	AlsoUnverified bool   `json:"also_verified_without_evidence,omitempty"`
	Marker         string `json:"marker"`
	Text           string `json:"text"`
	Line           int    `json:"line"`
	// Raw is the line exactly as scanned. Every write re-checks it: if the file
	// changed under us (an editor, a concurrent agent), the action is refused
	// rather than applied to a line that is no longer the one we judged.
	Raw      string         `json:"-"`
	Evidence commitEvidence `json:"evidence"`
}

// repairAction is one decision about one finding.
type repairAction struct {
	TaskID string `json:"task_id,omitempty"`
	Line   int    `json:"line"`
	Action string `json:"action"`
	// Marker is the raw marker for set_marker — one of " ", ">", "x", "!".
	Marker string `json:"marker,omitempty"`
	// Milestone is the destination for move_milestone.
	Milestone string `json:"milestone,omitempty"`
	Reason    string `json:"reason"`
	// Source records who decided: "commit_evidence" or "agent".
	Source string `json:"source,omitempty"`
}

// repairPlan pairs a finding with the action chosen for it.
type repairPlan struct {
	Finding repairFinding `json:"finding"`
	Action  repairAction  `json:"action"`
}

// repairRejection is a proposed action repair refused to apply, and why. These
// are always printed: an action silently dropped is the same failure mode as a
// task silently dropped.
type repairRejection struct {
	Action repairAction `json:"action"`
	Reason string       `json:"reason"`
}

// repairProposal is the JSON an agent writes for the review tier.
type repairProposal struct {
	Repairs []repairAction `json:"repairs"`
}

// ---------------------------------------------------------------------------
// Tier 1 — mechanical
// ---------------------------------------------------------------------------

// collectRepairFindings returns every line in PROGRESS.md that repair may act
// on, in document order.
//
// It composes the existing readers rather than walking the file again:
// `parseMilestones` for in-region tasks and `orphanedTaskLines` for everything
// past a column-zero `## `. A third walk here would be a fourth answer to
// "where does the milestones region end", which is exactly the duplication
// that produced #27 and #31.
func collectRepairFindings(progress string) []repairFinding {
	lines := strings.Split(progress, "\n")
	rawAt := func(line int) string {
		if line-1 >= 0 && line-1 < len(lines) {
			return lines[line-1]
		}
		return ""
	}

	var out []repairFinding
	for _, m := range parseMilestones(progress) {
		for _, t := range m.Tasks {
			named, _ := taskIDNamedMilestone(t.ID)
			// An unreadable marker is reported once, as itself. A task whose
			// marker Belmont cannot parse has a state problem before it has a
			// location problem, and reporting both would ask the reviewer to
			// decide the same line twice.
			if t.Status == taskUnknown {
				out = append(out, repairFinding{
					Rule:           ruleUnrecognisedMarker,
					TaskID:         t.ID,
					Milestone:      m.ID,
					NamedMilestone: named,
					Marker:         t.Marker,
					Text:           t.Name,
					Line:           t.Line,
					Raw:            rawAt(t.Line),
				})
				continue
			}
			if named != "" && named != m.ID {
				out = append(out, repairFinding{
					Rule:           ruleCrossMilestoneTaskID,
					TaskID:         t.ID,
					Milestone:      m.ID,
					NamedMilestone: named,
					Marker:         t.Marker,
					Text:           t.Name,
					Line:           t.Line,
					Raw:            rawAt(t.Line),
				})
			}
		}
	}
	for _, t := range orphanedTaskLines(progress) {
		named, _ := taskIDNamedMilestone(t.ID)
		out = append(out, repairFinding{
			Rule:           ruleTaskOutsideMilestone,
			TaskID:         t.ID,
			NamedMilestone: named,
			Marker:         t.Marker,
			Text:           t.Name,
			Line:           t.Line,
			Raw:            rawAt(t.Line),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

// ruleVerifiedWithoutEvidence is the audit, and it is deliberately NOT one of
// the three parse findings above.
//
// Those three are lines Belmont cannot act on: it says so, `belmont validate`
// exits 1, and repair exists to clear them. A `[v]` with no commit behind it is
// a different animal — the file parses perfectly, every count is right, and
// nothing downstream is broken. What is wrong is the CLAIM.
//
// Keeping it separate matters for a practical reason as well as a conceptual
// one. Repair reports "nothing to repair" and counts what is left unresolved by
// re-reading the file through collectRepairFindings; folding a permanent
// audit finding into that would mean a clean file never reads clean, which is
// how a warning becomes wallpaper.
const ruleVerifiedWithoutEvidence = "verified_without_evidence"

// auditVerifiedWithoutEvidence returns every `[v]` task that no commit names.
//
// This is the mirror of runEvidenceCheck, for the half it cannot see. That
// guard compares a phase's before and after, so it only ever audits a flip
// written WHILE it was watching; a `[v]` already on disk when the run starts is
// never a flip and is never checked, by anything, ever. `[V]` made that gap
// visible — it used to be an unrecognised marker that blocked the loop and now
// reads as the terminal state — but the gap is older than that and is exactly
// the same for a hand-written lowercase `[v]`.
//
// NEVER MECHANICAL, in either direction. No commit means no commit; it does not
// mean the work is absent. `knowledge/auto-mode/verify-evidence.md` records
// commit-less tasks as a known rough edge — a documentation-only or
// configuration-only task can be genuinely verified with nothing in the log
// naming it. So this reports, and the review tier reads the code. The action
// that follows is `set_marker "x"`, which the existing gate already permits and
// which hands the `[v]` back to `belmont reverify` and its own evidence
// contract. Repair still never writes a `[v]`.
//
// Returns nil when the log cannot be read: "could not look" must not print as
// "no evidence", which is the same rule the mechanical tier follows.
//
// And it applies the SAME cross-feature rule the mechanical tier does. Task IDs
// are feature-local, the commit log is not, so a sibling feature's commit for
// its own P1-M1-1 is not evidence about this feature's P1-M1-1 — clearing the
// audit on it silences the finding for every feature that shares the ID, which
// under the shipped template is every feature. That is the same acceptance
// `taskIDsClaimedElsewhere` was written to refuse one commit earlier, pointed
// the other way: there it wrote a state nobody could justify, here it withholds
// a question nobody else will ask. Reported as ambiguous, never as proof.
func auditVerifiedWithoutEvidence(root, slug, progress string) []repairFinding {
	named, ok := commitNamedTaskIDs(root)
	if !ok {
		return nil
	}
	shared, _ := taskIDsClaimedElsewhere(root, slug)
	lines := strings.Split(progress, "\n")
	var out []repairFinding
	for _, m := range parseMilestones(progress) {
		for _, t := range m.Tasks {
			if t.Status != taskVerified || t.ID == "" {
				continue
			}
			ev, hasCommit := named[t.ID]
			// A commit names it AND no other feature claims the ID: settled.
			if hasCommit && !shared[t.ID] {
				continue
			}
			raw := ""
			if t.Line-1 >= 0 && t.Line-1 < len(lines) {
				raw = lines[t.Line-1]
			}
			ev.Checked = true
			ev.Ambiguous = hasCommit
			namedMS, _ := taskIDNamedMilestone(t.ID)
			out = append(out, repairFinding{
				Rule:      ruleVerifiedWithoutEvidence,
				TaskID:    t.ID,
				Milestone: m.ID,
				// Carries NamedMilestone like every other finding. One line can
				// be BOTH a parse finding and an audit finding — a `[v]` filed
				// under the wrong milestone that no commit names is both — and
				// whichever of the two the gate looks up has to hold enough to
				// validate a move.
				NamedMilestone: namedMS,
				Marker:         t.Marker,
				Text:           t.Name,
				Line:           t.Line,
				Raw:            raw,
				Evidence:       ev,
			})
		}
	}
	return out
}

// attachCommitEvidence fills in the Evidence field of every finding that
// carries a task ID. The second return value reports whether the commit log
// could be read at all — false means the mechanical tier has no answers to
// give, and the caller must say so rather than let an empty result read as
// "nothing was provable".
func attachCommitEvidence(root, slug string, findings []repairFinding) ([]repairFinding, bool) {
	if !isGitWorkTree(root) {
		return findings, false
	}
	// Deliberately UNSCOPED — the whole history reachable from HEAD, not
	// `mergeBase..HEAD` the way runEvidenceCheck scopes it.
	//
	// The guard's question is "did THIS phase earn the flip it just wrote", so
	// it must exclude older work or a prior feature's commits would credit it.
	// Repair's question is the opposite: this damage is historical — months
	// old, usually on the default branch, written by a run that finished long
	// ago. Scoping to the merge base there is not conservative, it is empty:
	// on `main`, merge-base(HEAD, main) IS HEAD, so `HEAD..HEAD` matches
	// nothing and every single task reports "no commit names it". The
	// mechanical tier would look like it had run and settled nothing.
	//
	// The cost is that task IDs are feature-local, so two features can both
	// hold a P1-M1-1 and a commit for one could credit the other. That is why
	// every applied change names the commit it relied on, and why `--dry-run`
	// exists.
	const since = ""
	shared, _ := taskIDsClaimedElsewhere(root, slug)
	cache := map[string]commitEvidence{}
	// Availability is a property of the REPOSITORY, not of the findings. Setting
	// it inside the loop made "no finding carried a task ID" indistinguishable
	// from "the log could not be read", and the second is what every caller
	// prints — including the brief, which then tells the review tier that
	// nothing has been checked against a log it read perfectly well.
	available := commitLogReadable(root)
	for i := range findings {
		id := findings[i].TaskID
		if id == "" {
			continue
		}
		ev, seen := cache[id]
		if !seen {
			ev = lookupCommitEvidence(root, id, since)
			cache[id] = ev
		}
		ev.Ambiguous = shared[id]
		findings[i].Evidence = ev
	}
	return findings, available
}

// taskIDsClaimedElsewhere returns the task IDs that appear in some OTHER
// feature's PROGRESS.md.
//
// Task IDs are feature-local — `P1-M1-1` exists in essentially every feature —
// and the commit log is not, so an unscoped search for a bare ID can match a
// commit that belongs to a different feature entirely. Repairing `billing`
// would then mark its never-built `P1-M1-1` done on the strength of `auth`'s
// commit, and a milestone of those reads Complete.
//
// The fix is not to scope the git query. `.belmont/` is `--assume-unchanged`
// inside worktrees, so an agent's work commits do not touch the feature's state
// directory at all — a pathspec would find Belmont's own state commits, whose
// messages carry no task ID, and the mechanical tier would go quietly inert.
// That is the same mistake as scoping to the merge base, one layer down.
//
// So: detect the ambiguity directly and refuse it. An ID no other feature
// claims cannot be confused; one that two features claim is exactly the
// "ambiguous structure is refused, not guessed" rule this codebase already
// applies to duplicate milestone IDs and duplicate task IDs. The finding still
// goes to the review tier, which reads the code and can tell them apart.
//
// It FAILS OPEN, which is worth saying out loud rather than leaving to be
// discovered. It sees only the IDs in sibling PROGRESS.md files that still
// exist, while the commits stay in the log for ever — so a feature archived by
// `/belmont:cleanup`, whose files are replaced by a slim ARCHIVE.md, stops
// claiming its IDs and the ambiguity goes invisible again. `unreadable` names
// those directories so the caller can say the check was partial rather than
// imply it was complete.
func taskIDsClaimedElsewhere(root, slug string) (claimed map[string]bool, unreadable []string) {
	out := map[string]bool{}
	featuresDir := filepath.Join(root, ".belmont", "features")
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return out, nil
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == slug || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(featuresDir, e.Name(), "PROGRESS.md"))
		if err != nil {
			unreadable = append(unreadable, e.Name())
			continue
		}
		// Every task line, in region or not: a sibling's stray copy of the ID
		// makes the commit just as ambiguous as an in-region one.
		for _, line := range strings.Split(string(data), "\n") {
			m := repairAnyTaskLineRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if id, _, ok := parseTaskID(m[1]); ok {
				out[id] = true
			}
		}
	}
	sort.Strings(unreadable)
	return out, unreadable
}

// repairAnyTaskLineRe matches any checkbox line and captures its text. The ID
// then comes from `parseTaskID`, so this side of the ambiguity check accepts
// exactly the IDs the finding side does — a sibling claiming `FWLUP-SWEEP-1`
// has to be visible here, or the ambiguity it creates is invisible.
var repairAnyTaskLineRe = regexp.MustCompile(`^\s*-\s+\[.\]\s+(.+)$`)

// mechanicalRepairs returns the actions the commit log settles on its own.
//
// Deliberately narrow. Only an unreadable marker on a line that is already
// inside a milestone is auto-applied, and only to `[x]`:
//
//   - Capped at `[x]` because `[v]` has its own evidence contract
//     (knowledge/auto-mode/verify-evidence.md) and `belmont reverify` is its
//     only legitimate writer.
//   - An ORPHANED line is never auto-moved even with commit evidence. The
//     commonest orphan is a sibling's completion quoted into `## Session
//     History` — "- [x] P1-M1-1: done, commit abc123" — and it names a task ID
//     that certainly has a commit. Moving it in would duplicate a live ID.
//     Whether an orphan is a task or a log line is a reading question, so it
//     goes to the review tier.
//   - A cross-milestone ID is never auto-moved either: the ID naming M2 is
//     evidence about where the task was FILED, not about whether it is real.
func mechanicalRepairs(findings []repairFinding) []repairPlan {
	var out []repairPlan
	for _, f := range findings {
		if f.Rule != ruleUnrecognisedMarker || f.Milestone == "" {
			continue
		}
		if !f.Evidence.Checked || !f.Evidence.Found {
			continue
		}
		// The newest thing said about this task is that it was reverted. A
		// commit naming the ID is evidence that something happened, not that the
		// work stands, and this is the one case the log states outright.
		if f.Evidence.Reverting {
			continue
		}
		// Another feature holds this task ID too, so the commit that names it
		// may not be about this task at all. Not settled — send it to the tier
		// that can read the code and tell.
		if f.Evidence.Ambiguous {
			continue
		}
		reason := fmt.Sprintf("commit %s names this task", shortSHA(f.Evidence.SHA))
		if f.Evidence.Subject != "" {
			reason = fmt.Sprintf("commit %s (%q) names this task", shortSHA(f.Evidence.SHA), f.Evidence.Subject)
		}
		out = append(out, repairPlan{
			Finding: f,
			Action: repairAction{
				TaskID: f.TaskID,
				Line:   f.Line,
				Action: repairSetMarker,
				Marker: "x",
				Reason: reason,
				Source: "commit_evidence",
			},
		})
	}
	return out
}

// needsReview returns the findings the mechanical tier could not settle.
func needsReview(findings []repairFinding, settled []repairPlan) []repairFinding {
	done := map[int]bool{}
	for _, p := range settled {
		done[p.Finding.Line] = true
	}
	var out []repairFinding
	for _, f := range findings {
		if !done[f.Line] {
			out = append(out, f)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Validation — the gate every action passes through, whoever proposed it
// ---------------------------------------------------------------------------

// validateRepairPlans pairs proposed actions with the findings they name and
// refuses anything outside the bounds at the top of this file. Both tiers go
// through it: the mechanical tier's own output is validated too, so there is
// exactly one place that decides what repair is allowed to write.
//
// `content` is the current PROGRESS.md, needed to answer questions about the
// destination of a move.
func validateRepairPlans(content string, findings []repairFinding, actions []repairAction) ([]repairPlan, []repairRejection) {
	// FIRST wins. A line can produce two findings — a `[v]` filed under the
	// wrong milestone that no commit names is both a cross-milestone parse
	// finding and a verified-without-evidence audit finding — and the callers
	// pass the parse findings first because those are the ones carrying the
	// structural problem. Last-wins silently replaced them with the audit entry
	// and then refused a legitimate move for the wrong reason.
	byLine := map[int]repairFinding{}
	for _, f := range findings {
		if _, seen := byLine[f.Line]; seen {
			continue
		}
		byLine[f.Line] = f
	}
	milestoneIDs := map[string]bool{}
	idsIn := map[string]map[string]bool{} // milestone -> task IDs it already holds
	for _, m := range parseMilestones(content) {
		milestoneIDs[m.ID] = true
		if idsIn[m.ID] == nil {
			idsIn[m.ID] = map[string]bool{}
		}
		for _, t := range m.Tasks {
			if t.ID != "" {
				idsIn[m.ID][t.ID] = true
			}
		}
	}

	var plans []repairPlan
	var rejected []repairRejection
	seen := map[int]bool{}
	reject := func(a repairAction, reason string) {
		rejected = append(rejected, repairRejection{Action: a, Reason: reason})
	}

	for _, a := range actions {
		f, ok := byLine[a.Line]
		if !ok {
			// Repair may only touch lines it flagged. Without this an agent
			// could propose a marker change on any task in the file, which is
			// the out-of-scope flip runScopeGuard exists to revert — and the
			// guard is not running here.
			reject(a, fmt.Sprintf("line %d is not one of the findings repair reported", a.Line))
			continue
		}
		// The cross-check has to fire when the finding has NO id too. An agent
		// off by one line otherwise has its action accepted against whichever
		// finding sits there, provided that one is ID-less — and `withdraw`
		// would write `[-]` plus a Decisions Log entry onto the wrong task.
		if a.TaskID != "" && a.TaskID != f.TaskID {
			held := f.TaskID
			if held == "" {
				held = "a line with no task ID"
			}
			reject(a, fmt.Sprintf("names task %s but line %d holds %s", a.TaskID, a.Line, held))
			continue
		}
		if seen[a.Line] {
			reject(a, fmt.Sprintf("line %d already has an action; the second one was not applied", a.Line))
			continue
		}
		switch a.Action {
		case repairSetMarker:
			st, known := canonicalMarker(a.Marker)
			if !known {
				reject(a, fmt.Sprintf("marker %q is not one Belmont recognises", a.Marker))
				continue
			}
			if st == taskVerified {
				reject(a, "repair is capped at [x] — only `belmont reverify` may write [v], and only against its own commit evidence")
				continue
			}
			if st == taskWithdrawn {
				reject(a, "use the withdraw action for [-] so the reason is recorded in ## Decisions Log")
				continue
			}
			if f.Milestone == "" {
				reject(a, "this line sits outside every milestone, so a marker change would not make it visible to anything — move it or leave it")
				continue
			}
		case repairMoveMilestone:
			dest := a.Milestone
			if dest == "" {
				dest = f.NamedMilestone
			}
			if dest == "" {
				reject(a, "no destination milestone, and the task ID does not name one")
				continue
			}
			if !milestoneIDs[dest] {
				reject(a, fmt.Sprintf("milestone %s does not exist, and repair never creates one — run /belmont:tech-plan", dest))
				continue
			}
			if dest == f.Milestone {
				reject(a, fmt.Sprintf("the line is already under %s", dest))
				continue
			}
			if f.TaskID != "" && idsIn[dest][f.TaskID] {
				reject(a, fmt.Sprintf("%s already holds a task with ID %s — moving this line in would make the ID ambiguous", dest, f.TaskID))
				continue
			}
			// Claim the ID for the destination. `idsIn` was built from the
			// pre-apply document, so without this two moves of the same ID into
			// the same milestone both pass — neither is there *yet* — and the
			// result is the duplicate this check exists to prevent.
			if f.TaskID != "" {
				if idsIn[dest] == nil {
					idsIn[dest] = map[string]bool{}
				}
				idsIn[dest][f.TaskID] = true
			}
			a.Milestone = dest
		case repairWithdraw:
			if strings.TrimSpace(a.Reason) == "" {
				reject(a, "withdrawal needs a reason — a marker cannot carry why, which is what ## Decisions Log is for")
				continue
			}
			// The mirror of the `[v]` cap. Repair may not WRITE a verified
			// marker, and it may not take one away in a single step either:
			// nothing in this codebase can put a `[v]` back — `belmont reverify`
			// only ever promotes `[x]` — so `[v]` → `[-]` is irreversible by
			// tooling, and the evidence that reaches this gate for a verified
			// task is explicitly a question rather than a verdict. Demote to
			// `[x]` first; withdrawing from there is unrestricted.
			if st, known := canonicalMarker(f.Marker); known && st == taskVerified {
				reject(a, "this task is marked [v] and nothing can write that marker back — demote it with set_marker \"x\" first, then withdraw if the work really was dropped")
				continue
			}
		case repairLeave, repairEscalate:
			// No edit. Recorded so the report can say what happened.
		default:
			reject(a, fmt.Sprintf("%q is not one of the actions repair can take (%s)", a.Action, strings.Join(repairActionNames(), ", ")))
			continue
		}
		seen[a.Line] = true
		plans = append(plans, repairPlan{Finding: f, Action: a})
	}
	return plans, rejected
}

// markAlsoUnverified flags every parse finding whose line is also an unproven
// `[v]`. `needs_review` is what the interactive skill builds its proposal from,
// so the flag has to be on that list too, not only on the copy handed to the
// CLI-dispatched tier.
func markAlsoUnverified(parse, unearned []repairFinding) {
	audited := map[int]bool{}
	for _, f := range unearned {
		audited[f.Line] = true
	}
	for i := range parse {
		parse[i].AlsoUnverified = audited[parse[i].Line]
	}
}

// reviewableFindings is what the review tier is shown: ONE entry per line.
//
// A line can be both a parse finding and an audit finding — a `[v]` filed under
// a milestone its ID does not name that no commit names is both. Two entries
// asked the tier for two actions on a line `validateRepairPlans` accepts exactly
// one action for, so an agent obeying "one entry per finding" earned "line N
// already has an action", and which of the two the gate happened to keep decided
// user-visible wording. The parse finding survives because it carries the
// structural problem; `markAlsoUnverified` has already flagged it, so nothing
// downstream has to guess the line is two problems.
func reviewableFindings(parse, unearned []repairFinding) []repairFinding {
	out := make([]repairFinding, 0, len(parse)+len(unearned))
	onLine := map[int]bool{}
	for _, f := range parse {
		onLine[f.Line] = true
		out = append(out, f)
	}
	for _, f := range unearned {
		if onLine[f.Line] {
			continue
		}
		out = append(out, f)
	}
	return out
}

func repairActionNames() []string {
	return []string{repairSetMarker, repairMoveMilestone, repairWithdraw, repairLeave, repairEscalate}
}

// ---------------------------------------------------------------------------
// Writers
// ---------------------------------------------------------------------------

// repairTaskLineRe splits a task line into prefix, marker and remainder.
var repairTaskLineRe = regexp.MustCompile(`^(\s*-\s+)\[(.)\](\s+.*)$`)

// applyRepairPlans rewrites PROGRESS.md content according to the plans.
//
// Returns the new content, one summary line per applied change, and warnings
// for plans that could not be applied. A plan is skipped — never forced — when
// the line is no longer byte-for-byte what was scanned.
//
// `today` is passed in rather than read from the clock so the Decisions Log
// entries are deterministic under test.
func applyRepairPlans(content string, plans []repairPlan, today string) (string, []repairPlan, []string) {
	lines := strings.Split(content, "\n")
	var wrote []repairPlan
	var warnings []string

	moves := map[int]string{}
	var decisions []string

	for _, p := range plans {
		idx := p.Finding.Line - 1
		if idx < 0 || idx >= len(lines) {
			warnings = append(warnings, fmt.Sprintf("line %d is past the end of PROGRESS.md — skipped", p.Finding.Line))
			continue
		}
		if lines[idx] != p.Finding.Raw {
			warnings = append(warnings, fmt.Sprintf(
				"line %d changed since it was scanned — skipped rather than applied to a line repair has not read", p.Finding.Line))
			continue
		}
		switch p.Action.Action {
		case repairSetMarker:
			m := repairTaskLineRe.FindStringSubmatch(lines[idx])
			if len(m) < 4 {
				warnings = append(warnings, fmt.Sprintf("line %d is not shaped like a task line — skipped", p.Finding.Line))
				continue
			}
			lines[idx] = m[1] + "[" + p.Action.Marker + "]" + m[3]
			wrote = append(wrote, p)
		case repairWithdraw:
			m := repairTaskLineRe.FindStringSubmatch(lines[idx])
			if len(m) < 4 {
				warnings = append(warnings, fmt.Sprintf("line %d is not shaped like a task line — skipped", p.Finding.Line))
				continue
			}
			lines[idx] = m[1] + "[-]" + m[3]
			wrote = append(wrote, p)
			decisions = append(decisions, fmt.Sprintf("%s — %s withdrawn: %s",
				today, repairLabel(p.Finding), strings.TrimSpace(p.Action.Reason)))
		case repairMoveMilestone:
			moves[idx] = p.Action.Milestone
			wrote = append(wrote, p)
		case repairLeave, repairEscalate:
			// Nothing to write.
		}
	}

	if len(moves) > 0 {
		var moveWarnings []string
		var dropped []int
		lines, moveWarnings, dropped = moveTaskLines(lines, moves)
		warnings = append(warnings, moveWarnings...)
		// A move that could not be placed did not happen, so it must come back
		// out of the written set — otherwise the report, and the JSON, claim a
		// change that is not in the file.
		if len(dropped) > 0 {
			droppedIdx := map[int]bool{}
			for _, i := range dropped {
				droppedIdx[i] = true
			}
			kept := wrote[:0]
			for _, p := range wrote {
				if p.Action.Action == repairMoveMilestone && droppedIdx[p.Finding.Line-1] {
					continue
				}
				kept = append(kept, p)
			}
			wrote = kept
		}
	}

	out := strings.Join(lines, "\n")
	for _, d := range decisions {
		out = appendDecisionLogEntry(out, d)
	}
	return out, wrote, warnings
}

// summariseRepairPlan is the ONE description of a change repair made. The
// report prints it, and the JSON `applied` list is the plans behind it — two
// renderings of one fact rather than two lists that can disagree.
func summariseRepairPlan(p repairPlan) string {
	switch p.Action.Action {
	case repairSetMarker:
		return fmt.Sprintf("%s [%s] → [%s] — %s",
			repairLabel(p.Finding), p.Finding.Marker, p.Action.Marker, p.Action.Reason)
	case repairWithdraw:
		return fmt.Sprintf("%s [%s] → [-] withdrawn — %s",
			repairLabel(p.Finding), p.Finding.Marker, p.Action.Reason)
	case repairMoveMilestone:
		return fmt.Sprintf("%s moved to %s — %s",
			repairLabel(p.Finding), p.Action.Milestone, p.Action.Reason)
	}
	return fmt.Sprintf("%s %s", repairLabel(p.Finding), p.Action.Action)
}

// repairLabel names a finding the way the rest of Belmont names tasks.
func repairLabel(f repairFinding) string {
	if f.TaskID != "" {
		return f.TaskID
	}
	return fmt.Sprintf("PROGRESS.md:%d", f.Line)
}

// moveTaskLines relocates the tasks in `moves` (line index -> destination
// milestone ID) under the destination milestone, preserving every line verbatim.
//
// A task is a bullet PLUS its body — the indented continuation lines beneath
// it, which is where `**Verification**` and `**Evidence**` prose lives. Moving
// the bullet alone turns one task into two false statements: the body stays
// behind and silently re-attaches to whatever task now precedes it, crediting
// that task with evidence it never earned, while the task that moved arrives
// asserting done with nothing behind it. Nothing catches it afterwards — the
// file still parses, the task count is unchanged, and `belmont validate`
// reports no violations. See issue #33.
//
// The extent of a task's body is `taskBodyEnd`, the same reading that anchors
// an insertion past the destination task's own body and the same one
// `mergeProgressState` uses. One definition of where a task ends.
//
// Anchoring follows `mergeProgressState`: after the destination's last task
// line (past its indented body), or after its header when it holds no tasks
// yet — otherwise the first task moved into an empty milestone has nowhere to
// land. Region boundaries are `isSectionBreak`, like every other reader.
//
// Returns the new lines, warnings, and the starting indices of moves that could
// not be placed. A move that cannot be placed leaves its whole block exactly
// where it was: repair never deletes a task line, and never creates a milestone
// to receive one.
func moveTaskLines(lines []string, moves map[int]string) ([]string, []string, []int) {
	msHeaderRe := regexp.MustCompile(`^###\s+(?:[✅⬜🔄🚫]\s*)?M(\d+):`)
	taskRe := regexp.MustCompile(`^\s*-\s+\[(.)\]\s+`)

	lastTaskIdx := map[string]int{}
	lastHeaderIdx := map[string]int{}
	currentMS := ""
	for i, line := range lines {
		if m := msHeaderRe.FindStringSubmatch(line); len(m) >= 2 {
			currentMS = "M" + m[1]
			lastHeaderIdx[currentMS] = i
			continue
		}
		if isSectionBreak(line) {
			currentMS = ""
			continue
		}
		if currentMS != "" && taskRe.MatchString(line) {
			lastTaskIdx[currentMS] = i
		}
	}

	var warnings []string
	var dropped []int

	var order []int
	for idx := range moves {
		order = append(order, idx)
	}
	sort.Ints(order)

	// Extents first. `moving` holds every line of every block, not just the
	// bullets, so an anchor landing anywhere inside a block is recognised as a
	// line that will not be emitted where it currently sits.
	//
	// A bullet inside another moving task's body is refused AS A SEPARATE ACTION
	// rather than moved twice: its lines are already carried by the enclosing
	// block, so emitting them again would duplicate a task ID and switch off
	// every milestone-keyed reader.
	//
	// Note what the warning must NOT say. The line does not stay where it is — it
	// travels with the block enclosing it, to that block's destination. Only the
	// separate move failed. Reporting a real relocation as "left exactly where it
	// is" would be a false statement about a moved task, which is the class of
	// bug this whole change exists to remove.
	blockEnd := map[int]int{}
	moving := map[int]bool{}
	var placeable []int
	coveredTo := -1
	coveringDest := ""
	for _, idx := range order {
		if idx <= coveredTo {
			warnings = append(warnings, fmt.Sprintf(
				"the task at line %d sits inside the body of another task being moved, so it travels to %s with that task instead of being moved on its own — separate the two first if it belongs somewhere else",
				idx+1, coveringDest))
			dropped = append(dropped, idx)
			continue
		}
		end := taskBodyEnd(lines, idx)
		blockEnd[idx] = end
		for i := idx; i <= end; i++ {
			moving[i] = true
		}
		coveredTo = end
		coveringDest = moves[idx]
		placeable = append(placeable, idx)
	}

	// Resolve anchors second: a line being moved OUT cannot serve as the anchor
	// for a block being moved IN, because it will not be emitted.
	insertAfter := map[int][]int{} // anchor index -> moved block start indices
	for _, idx := range placeable {
		dest := moves[idx]
		anchor, ok := lastTaskIdx[dest]
		if ok && moving[anchor] {
			anchor, ok = lastHeaderIdx[dest]
		} else if ok {
			// Past the anchor task's indented body, so the moved block cannot
			// land between that task and its own continuation lines.
			anchor = taskBodyEnd(lines, anchor)
			if moving[anchor] {
				// The anchor's body runs into a block that is leaving.
				anchor, ok = lastHeaderIdx[dest]
			}
		}
		if !ok {
			anchor, ok = lastHeaderIdx[dest]
		}
		if !ok {
			warnings = append(warnings, fmt.Sprintf(
				"milestone %s is not in this PROGRESS.md, so the line at %d was left exactly where it is — repair never creates a milestone",
				dest, idx+1))
			dropped = append(dropped, idx)
			for i := idx; i <= blockEnd[idx]; i++ {
				delete(moving, i)
			}
			continue
		}
		insertAfter[anchor] = append(insertAfter[anchor], idx)
	}

	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if moving[i] {
			continue
		}
		out = append(out, line)
		for _, moved := range insertAfter[i] {
			out = append(out, lines[moved:blockEnd[moved]+1]...)
		}
	}
	sort.Ints(dropped)
	return out, warnings, dropped
}

// decisionsHeadingRe matches the `## Decisions Log` heading at column zero.
var decisionsHeadingRe = regexp.MustCompile(`^##\s+Decisions Log\s*$`)

// appendDecisionLogEntry adds one line to `## Decisions Log`, creating the
// section at the end of the document when it is absent.
//
// The reason a task was withdrawn cannot live in the marker, so it lives here.
// A withdrawal recorded without its reason is the state that made someone
// invent `[-]` in the first place: the file says work stopped and nothing says
// why, so the next reader either re-opens it or deletes it.
func appendDecisionLogEntry(content, entry string) string {
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if decisionsHeadingRe.MatchString(line) {
			start = i
			break
		}
	}
	if start == -1 {
		trimmed := strings.TrimRight(content, "\n")
		return trimmed + "\n\n## Decisions Log\n\n- " + entry + "\n"
	}
	// Find the end of the section: the next column-zero `## `, the next
	// milestone header, or EOF.
	//
	// A milestone header is deliberately NOT a section break, so scanning for
	// one alone ran past `### M1:` when `## Decisions Log` sits above the
	// milestones — the whole rest of the document became the log body and the
	// entry was appended at EOF, inside the last milestone.
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if isSectionBreak(lines[i]) || msHeaderRe.MatchString(lines[i]) {
			end = i
			break
		}
	}
	// Drop the "(none yet)" placeholder and any trailing blank lines, then
	// append. Keeping the placeholder above a real entry reads as a bug.
	section := lines[start+1 : end]
	var kept []string
	for _, line := range section {
		if strings.EqualFold(strings.TrimSpace(line), "(none yet)") {
			continue
		}
		kept = append(kept, line)
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) == 0 {
		kept = append(kept, "")
	}
	kept = append(kept, "- "+entry, "")

	out := append([]string{}, lines[:start+1]...)
	out = append(out, kept...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// ---------------------------------------------------------------------------
// Review tier — agent dispatch
// ---------------------------------------------------------------------------

// buildRepairBrief renders the findings the agent must judge.
func buildRepairBrief(feature, proposalPath string, findings []repairFinding, evidenceAvailable bool) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "/belmont:repair --feature %s\n\n", feature)
	sb.WriteString("REPAIR BRIEF — proposal mode.\n\n")
	sb.WriteString("Do NOT edit PROGRESS.md, or any other file. Investigate, then write a JSON proposal to:\n  ")
	sb.WriteString(proposalPath)
	sb.WriteString("\n\n")
	if !evidenceAvailable {
		sb.WriteString("The commit log could not be read here, so NONE of these has been checked against it. Weigh the code accordingly.\n\n")
	}
	sb.WriteString("Each finding below is a line in .belmont/features/" + feature + "/PROGRESS.md that Belmont cannot act on as written.\n")
	sb.WriteString("`evidence.found: true` means a commit names the task ID — that has already been applied where it settles the answer, so anything still listed here is NOT settled by the commit log.\n\n")

	data, err := json.MarshalIndent(struct {
		Findings []repairFinding `json:"findings"`
	}{findings}, "", "  ")
	if err == nil {
		sb.Write(data)
		sb.WriteString("\n\n")
	}
	sb.WriteString(repairAgentRules)
	return sb.String()
}

// repairAgentRules is the contract the review tier works to. It is duplicated
// in the skill body on purpose: the CLI path injects this text directly (the
// tool's own skill discovery is bypassed), and the interactive path reads the
// skill. Both must state the same bounds. See
// knowledge/cross-cutting/dual-invocation-paths.md.
const repairAgentRules = `TWO KINDS OF FINDING. Check the "rule" field.

Everything except "verified_without_evidence" is a line Belmont cannot act on as
written — an unreadable marker, a task outside every milestone, a task filed
under the wrong one. Decide those by the table below.

READING THE EVIDENCE on a finding. These fields decide how much the commit log
has already told you, and one of them is a trap:

  "checked": false     the commit log could not be read at all. Nothing below
                       applies; weigh the code alone.
  "found": true        a commit names this task ID.
  "ambiguous": true    ...but ANOTHER feature's PROGRESS.md claims the same task
                       ID. Task IDs are feature-local and the commit log is not,
                       so that commit may be about entirely different work.
                       TREAT IT AS NO EVIDENCE and read the code. This is why
                       the finding reached you with a SHA attached instead of
                       being settled without you.
  "also_verified_without_evidence": true
                       this line is BOTH the parse problem named by "rule" AND a
                       verified marker no commit proves. You get one action for
                       it. Decide the parse problem, and say in your reason
                       whether the verified claim still stands.

"verified_without_evidence" is different: the line parses, the counts are right,
and the only thing in question is the CLAIM. The task is marked verified and no
commit in this repository names it. Nothing audits that — the commit-evidence
guard only ever compares one phase's before and after, so a verified marker
already on disk when a run started was never checked by anything. For each of
these, look for the work in the code and decide:

  - the work is there, and a test or the code plainly demonstrates it — the task
    is verified, the commit convention was just not followed (docs-only and
    config-only tasks routinely leave no commit naming them)  -> leave
  - the work is there but nothing shows it was verified                -> set_marker "x"
  - the work is not there at all                                        -> set_marker " "
  - you cannot tell                                                     -> escalate

Never set_marker "v" — the CLI refuses it. Demoting to "x" is the useful move:
` + "`belmont reverify`" + ` then re-earns the flip under its own evidence
contract. "leave" is a real and common answer here; do not treat a missing
commit as proof of missing work.

HOW TO DECIDE the other three — evidence, never memory:

For each finding, read the task text and then look for what it names in the
CURRENT code: the route, component, table, endpoint, flag, test. You are
deciding between "still outstanding", "already there", "superseded" and "not a
task at all", and the repository answers all four.

  - The thing exists and does what the task describes  -> set_marker "x"
  - The thing does not exist and nothing supersedes it -> set_marker " "
  - Something replaced it, or it was folded into other work, or the feature it
    belonged to is gone                                -> withdraw (+ reason)
  - It is waiting on something identifiable            -> set_marker "!"
  - It is not a task: a retro bullet, a quoted log line, a table row that
    happens to look like a checkbox                    -> leave
  - It is a real task sitting in the wrong place       -> move_milestone
  - The code does not settle it                        -> escalate

Never guess. "escalate" is a real answer and is always better than a marker
nobody can justify — a wrong state here is the bug this command exists to fix.

WHERE A MISPLACED TASK GOES. This is the step people loop on, so it has a rule:

  - "task_outside_milestone" or "cross_milestone_task_id" whose ID names a
    milestone the file ALREADY has -> move_milestone to that milestone. Done.
  - a real task whose ID names no milestone, OR names one this file does not
    have (an ID saying M9 in a plan that stops at M5 — repair cannot create M9,
    so that ID settles nothing) — a follow-up from a cross-cutting sweep is the
    usual case — still needs a destination. Do NOT escalate it to
    /belmont:tech-plan expecting a new milestone: that skill forbids creating
    one for follow-ups, so the task comes straight back here unscheduled. File
    it under the HIGHEST-NUMBERED existing milestone whose work it touches — the
    last one whose outputs the fix depends on — or the final milestone in the
    plan when it is genuinely global. Name the milestones it touches in your
    reason. Filing it under the earliest one instead re-opens work that later
    milestones already built on, which is the failure the milestone-immutability
    rule exists to prevent.
  - only when you cannot tell what it touches -> escalate.

ACTIONS (this set is closed; the reasons are not):
  {"line": <int>, "task_id": "<id>", "action": "set_marker",     "marker": "x|>| |!", "reason": "<what in the code says so>"}
  {"line": <int>, "task_id": "<id>", "action": "move_milestone", "milestone": "M<n>", "reason": "..."}
  {"line": <int>, "task_id": "<id>", "action": "withdraw",       "reason": "<why it was dropped>"}
  {"line": <int>, "task_id": "<id>", "action": "leave",          "reason": "<why this is not a task>"}
  {"line": <int>, "task_id": "<id>", "action": "escalate",       "reason": "<what you could not determine>"}

"line" identifies the finding and must match one from the brief exactly.

HARD LIMITS — the CLI enforces every one of these and will refuse a proposal
that breaks them, so proposing one only wastes the run:
  - "v" is not an available marker. Repair is capped at [x]; only
    ` + "`belmont reverify`" + ` may write [v], against its own commit evidence.
  - Withdrawal is the withdraw action, not set_marker "-", because the reason
    has to reach ## Decisions Log. Never express it by deleting the line: a
    deletion does not survive a sibling worktree merge and the task comes back.
  - A task currently marked [v] cannot be withdrawn in one step. Nothing in
    Belmont can write that marker back, so demote it with set_marker "x" first;
    withdrawing from there is unrestricted.
  - move_milestone only ever targets a milestone that ALREADY exists, and never
    one that already holds that task ID. Repair does not create, rename or
    remove milestones — that is /belmont:tech-plan's job alone.
  - You may only act on the lines in this brief.

OUTPUT: write ONLY this JSON to the proposal path, nothing else:
{"repairs": [ {...}, {...} ]}
One entry per finding. A finding you cannot settle gets "escalate", not silence.`

// parseRepairProposal reads and validates the agent's proposal file shape.
func parseRepairProposal(path string) (repairProposal, error) {
	var p repairProposal
	data, err := os.ReadFile(path)
	if err != nil {
		return p, fmt.Errorf("read proposal: %w", err)
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("parse proposal: %w", err)
	}
	if len(p.Repairs) == 0 {
		return p, fmt.Errorf("proposal contains no repairs")
	}
	for i := range p.Repairs {
		p.Repairs[i].Source = "agent"
	}
	return p, nil
}

// runRepairAgent dispatches the review tier and returns the proposal path.
func runRepairAgent(root, feature, tool string, tiers modelTierConfig, findings []repairFinding, evidenceAvailable bool) (string, error) {
	proposalPath := filepath.Join(root, ".belmont", "features", feature, "repair-proposal.json")
	// A stale proposal from an aborted run must never be mistaken for this
	// run's answer.
	os.Remove(proposalPath)

	prompt := adaptPromptForTool(buildRepairBrief(feature, proposalPath, findings, evidenceAvailable), tool)
	// Repair decides what a state marker means, which is the same class of
	// judgement as merge reconciliation — and wrong is expensive. Same tier,
	// which defaults to high.
	flags := resolveModelFlags(tool, reconciliationTier(tiers), root)
	args := toolHeadlessArgs(tool, prompt, root, flags, true)
	if args == nil {
		return "", fmt.Errorf("unsupported tool: %s", tool)
	}
	cmd := exec.Command(toolBinary(tool), args...)
	cmd.Dir = root

	prefix := fmt.Sprintf("\033[36m[%s][repair]\033[0m: ", feature)
	if tool == "claude" {
		tw := newTailWriter(os.Stderr, 1500, "")
		cmd.Stdout = &claudeStreamWriter{tw: tw, prefix: prefix}
		cmd.Stderr = tw
	} else {
		tw := newTailWriter(os.Stderr, 1500, prefix)
		cmd.Stdout = tw
		cmd.Stderr = tw
	}
	if err := cmd.Run(); err != nil {
		return proposalPath, fmt.Errorf("%s: %w", tool, err)
	}
	return proposalPath, nil
}

// ---------------------------------------------------------------------------
// Command
// ---------------------------------------------------------------------------

type repairJSON struct {
	Feature           string          `json:"feature"`
	EvidenceAvailable bool            `json:"evidence_available"`
	Findings          []repairFinding `json:"findings"`
	Applied           []repairPlan    `json:"applied"`
	// WouldApply is what the mechanical tier settles but has not written,
	// because this is a --dry-run. Separate from Applied so a consumer — the
	// interactive skill runs `--dry-run --format json` first — can never mistake
	// a preview for a change on disk. Changed is false throughout a dry run.
	WouldApply  []repairPlan    `json:"would_apply,omitempty"`
	NeedsReview []repairFinding `json:"needs_review"`
	// UnearnedVerified is the audit, kept out of Findings and NeedsReview on
	// purpose: these lines parse, so `belmont validate` is happy with them and
	// "nothing to repair" stays meaningful. They still reach the review tier.
	UnearnedVerified []repairFinding   `json:"verified_without_evidence,omitempty"`
	Rejected         []repairRejection `json:"rejected,omitempty"`
	Warnings         []string          `json:"warnings,omitempty"`
	Changed          bool              `json:"changed"`
}

// runRepairCmd handles `belmont repair`.
func runRepairCmd(args []string) error {
	fs := flag.NewFlagSet("repair", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, feature, format, tool, applyProposal string
	var dryRun, mechanicalOnly, yes bool
	fs.StringVar(&root, "root", ".", "project root")
	fs.StringVar(&feature, "feature", "", "feature slug")
	fs.StringVar(&format, "format", "text", "output format (text|json)")
	fs.StringVar(&tool, "tool", "", "CLI tool (claude|codex|gemini|copilot|cursor|pi|opencode)")
	fs.StringVar(&applyProposal, "apply-proposal", "", "apply a proposal JSON file instead of dispatching an agent")
	fs.BoolVar(&dryRun, "dry-run", false, "report findings and commit evidence; change nothing")
	fs.BoolVar(&mechanicalOnly, "mechanical-only", false, "apply only what commit evidence settles; do not dispatch an agent")
	fs.BoolVar(&yes, "yes", false, "apply reviewed proposals without prompting")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("repair: %w", err)
	}
	root, _ = filepath.Abs(root)

	slug, err := resolveSingleFeature(root, feature, "repair")
	if err != nil {
		return err
	}
	progressPath := filepath.Join(root, ".belmont", "features", slug, "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("repair: cannot read %s: %w", progressPath, err)
	}

	// Ambiguous milestone structure is refused, not guessed — the same policy
	// runScopeGuard and runEvidenceCheck apply. Every lookup a move makes is
	// keyed by milestone ID, so with a repeated heading repair would relocate a
	// task into whichever block it saw last.
	if snap := parseProgressSnapshot(progressPath, string(content)); snap != nil && len(snap.DupIDs) > 0 {
		return fmt.Errorf(
			"repair: PROGRESS.md has more than one `### %s:` heading, so milestone blocks are ambiguous. "+
				"Repair will not guess which one a task belongs to. De-duplicate the heading (a session note must not be written in milestone-header shape), then re-run",
			snap.DupIDs[0])
	}

	findings := collectRepairFindings(string(content))
	findings, evidenceAvailable := attachCommitEvidence(root, slug, findings)
	unearned := auditVerifiedWithoutEvidence(root, slug, string(content))

	out := repairJSON{
		Feature:           slug,
		EvidenceAvailable: evidenceAvailable,
		Findings:          findings,
		UnearnedVerified:  unearned,
	}
	jsonOut := strings.EqualFold(format, "json")

	if len(findings) == 0 && len(unearned) == 0 {
		if jsonOut {
			return emitJSON(out)
		}
		fmt.Fprintf(os.Stderr, "\033[32m✓ %s: nothing to repair — every task line parses and sits under a milestone.\033[0m\n", slug)
		return nil
	}

	// ---- Tier 1: mechanical ------------------------------------------------
	settled := mechanicalRepairs(findings)
	settledActions := make([]repairAction, 0, len(settled))
	for _, p := range settled {
		settledActions = append(settledActions, p.Action)
	}
	// The mechanical tier's own output is validated too — one gate, whoever
	// proposed the action.
	plans, rejected := validateRepairPlans(string(content), findings, settledActions)
	out.Rejected = rejected

	today := time.Now().Format("2006-01-02")
	if !dryRun && len(plans) > 0 {
		newContent, applied, warnings := applyRepairPlans(string(content), plans, today)
		if newContent != string(content) {
			if err := writeStateFile(progressPath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("repair: write %s: %w", progressPath, err)
			}
			content = []byte(newContent)
			out.Changed = true
		}
		// Only plans applyRepairPlans actually wrote. `leave` and `escalate`
		// write nothing by definition, and a plan it skipped emitted a warning
		// instead — reporting either as applied is the same class of untruth
		// this command exists to remove.
		out.Applied = append(out.Applied, applied...)
		out.Warnings = append(out.Warnings, warnings...)
		// The ambiguity check is only as complete as the sibling PROGRESS.md
		// files it can read, and an archived feature keeps its commits while
		// losing its task IDs. Said once, and only when the tier actually wrote
		// something on commit evidence — the only case where the gap can have
		// cost anything.
		if _, blind := taskIDsClaimedElsewhere(root, slug); len(blind) > 0 && len(applied) > 0 {
			out.Warnings = append(out.Warnings, fmt.Sprintf(
				"%s has no PROGRESS.md, so its task IDs were not checked against this feature's — an archived feature keeps its commits. Confirm the commit(s) above belong to %s.",
				strings.Join(blind, ", "), slug))
		}
		if !jsonOut {
			fmt.Fprintf(os.Stderr, "\033[1mBelmont Repair\033[0m — %s\n\n", slug)
			fmt.Fprintf(os.Stderr, "\033[1mCommit evidence\033[0m — applied %d change(s), no tokens spent:\n", len(applied))
			for _, a := range applied {
				fmt.Fprintf(os.Stderr, "  \033[32m✓\033[0m %s\n", summariseRepairPlan(a))
			}
			fmt.Fprintln(os.Stderr)
		}
	} else {
		if dryRun {
			out.WouldApply = plans
		}
		if !jsonOut {
			fmt.Fprintf(os.Stderr, "\033[1mBelmont Repair\033[0m — %s\n\n", slug)
			// Nothing to repair, but something to look at. Reaching the audit
			// block with no preamble reads as though repair had failed to run.
			if len(findings) == 0 && len(unearned) > 0 {
				fmt.Fprintf(os.Stderr, "\033[32m✓ Every task line parses and sits under a milestone.\033[0m\n\n")
			}
			// A dry run must still say what the mechanical tier WOULD settle.
			// Without this the preview lists every finding as needing a code
			// read, including the ones a commit already answers — which is both
			// wrong and the opposite of the point.
			if dryRun && len(plans) > 0 {
				_, applied, _ := applyRepairPlans(string(content), plans, today)
				fmt.Fprintf(os.Stderr, "\033[1mCommit evidence\033[0m — would apply %d change(s), no tokens spent:\n", len(applied))
				for _, a := range applied {
					fmt.Fprintf(os.Stderr, "  \033[2m→\033[0m %s\n", summariseRepairPlan(a))
				}
				fmt.Fprintln(os.Stderr)
			}
		}
	}

	// Re-scan: the applied changes moved line numbers if anything was moved,
	// and a finding that has been fixed is no longer a finding.
	//
	// The line-keyed exclusion applies to the DRY RUN ONLY. There, nothing was
	// written, so what the mechanical tier settled has to be subtracted by hand
	// or the preview and the real run report different work. On the real path
	// the file has already been rewritten, so anything `collectRepairFindings`
	// still returns is still true of the file — and subtracting by line there
	// hid a finding repair had just CREATED. `collectRepairFindings` reports one
	// finding per line (an unreadable marker `continue`s before the
	// cross-milestone check), so a `[?] P1-M2-7` filed under M1 is only ever the
	// marker problem; flipping it to `[x]` makes it a cross-milestone problem on
	// the same line, and that line was in `plans`. The report said "nothing left
	// needs a code read" and `belmont validate` then exited 1 on it.
	remaining := collectRepairFindings(string(content))
	if dryRun {
		remaining = needsReview(remaining, plans)
	}
	remaining, _ = attachCommitEvidence(root, slug, remaining)
	// Mark the lines that are ALSO an unproven `[v]` before anything reads the
	// list. `needs_review` is what the interactive skill builds its proposal
	// from, so the flag has to be there too, not only on the copy handed to the
	// CLI-dispatched tier.
	markAlsoUnverified(remaining, unearned)
	out.NeedsReview = remaining

	if !jsonOut {
		renderRepairFindings(os.Stderr, slug, remaining, evidenceAvailable)
		renderUnearnedVerified(os.Stderr, unearned, isShallowClone(root))
	}

	// The audit rides along to the review tier. It is not in `remaining` —
	// `remaining` is re-derived from the file and drives "nothing left needs a
	// code read" — but an agent can settle these, and the gate has to know
	// about a line before it will act on it.
	//
	// ONE ENTRY PER LINE. A line can be both a parse finding and an audit
	// finding — a `[v]` filed under a milestone its ID does not name that no
	// commit names is both. Handing the review tier two entries for one line
	// asked it for two actions on a line the gate accepts exactly one action
	// for: an agent obeying "one entry per finding" earned a rejection reading
	// "line N already has an action", and which of the two the gate happened to
	// keep decided user-visible wording. The parse finding survives because it
	// carries the structural problem, and it carries a flag so nothing
	// downstream has to guess that the line is also an unproven claim.
	reviewable := reviewableFindings(remaining, unearned)

	if len(reviewable) == 0 {
		if jsonOut {
			return emitJSON(out)
		}
		fmt.Fprintf(os.Stderr, "\033[32m✓ Nothing left needs a code read.\033[0m\n")
		// Even on the happy path: a refusal or a skipped write is never dropped.
		renderRepairRejections(os.Stderr, out.Rejected)
		for _, w := range out.Warnings {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠\033[0m %s\n", w)
		}
		renderRepairNextSteps(os.Stderr, slug, out.Changed, 0)
		return nil
	}

	if dryRun || mechanicalOnly {
		if jsonOut {
			return emitJSON(out)
		}
		if dryRun {
			fmt.Fprintln(os.Stderr, "\033[2m--dry-run: nothing was written.\033[0m")
		}
		renderRepairRejections(os.Stderr, out.Rejected)
		for _, w := range out.Warnings {
			fmt.Fprintf(os.Stderr, "  \033[33m⚠\033[0m %s\n", w)
		}
		fmt.Fprintf(os.Stderr, "Run `belmont repair --feature %s` to have an agent read these against the code.\n", slug)
		if out.Changed {
			renderRepairNextSteps(os.Stderr, slug, true, len(remaining))
		}
		return nil
	}

	// ---- Tier 2: review ----------------------------------------------------
	proposalPath := applyProposal
	if proposalPath == "" {
		if tool == "" {
			tool = detectTool()
			if tool == "" {
				return fmt.Errorf("repair: no supported AI tool CLI found on PATH\n\nSupported tools: %s\nInstall one, pass --tool, or use --mechanical-only to stop after the commit-evidence tier",
					strings.Join(allToolNames(), ", "))
			}
		}
		tiers, _ := parseModelTiers(filepath.Join(root, ".belmont", "features", slug, "models.yaml"))
		if !jsonOut {
			fmt.Fprintf(os.Stderr, "\033[1mReview tier\033[0m — asking %s to read %d finding(s) against the code…\n\n", tool, len(reviewable))
		}
		p, err := runRepairAgent(root, slug, tool, tiers, reviewable, evidenceAvailable)
		// Arm the cleanup on the path RETURNED, before checking the error and
		// before parsing. Every failure below — the agent exiting non-zero,
		// writing prose instead of JSON, writing an empty repairs array —
		// otherwise returns with the scratch file still sitting inside
		// `.belmont/features/<slug>/`, where it is an untracked file that
		// `requireCleanWorkingTree` refuses to start `belmont auto` over. A
		// command that heals PROGRESS.md must not leave debris that blocks the
		// loop.
		defer os.Remove(p)
		if err != nil {
			return fmt.Errorf("repair: review tier failed: %w\n\nThe commit-evidence changes above are already applied and are unaffected", err)
		}
		proposalPath = p
	}

	proposal, err := parseRepairProposal(proposalPath)
	if err != nil {
		return fmt.Errorf("repair: %w\n\nThe commit-evidence changes above are already applied and are unaffected. Nothing from the review tier was written", err)
	}

	// RE-READ from disk before validating or writing anything.
	//
	// The review tier is a subprocess that runs for minutes, and everything
	// below rebuilds the WHOLE file and writes it back. Using the copy read
	// before the agent started would silently revert any edit made in the
	// meantime — by the user, by an editor, or by the agent itself if it
	// ignored the instruction not to write.
	//
	// It is also what makes the staleness guard real rather than decorative.
	// `applyRepairPlans` refuses a plan whose line no longer matches
	// `Finding.Raw`; against the same snapshot the findings came from, that
	// comparison can never fail, so it protected nothing on this path.
	fresh, err := os.ReadFile(progressPath)
	if err != nil {
		return fmt.Errorf("repair: re-read %s after the review tier: %w", progressPath, err)
	}
	content = fresh

	reviewPlans, reviewRejected := validateRepairPlans(string(content), reviewable, proposal.Repairs)
	out.Rejected = append(out.Rejected, reviewRejected...)
	if !jsonOut {
		// "leave" and "escalate" are answers, not silences — they are the two
		// ways the review tier says a line should stay exactly as written, and
		// a report that omitted them would look like the agent had skipped
		// those findings.
		renderRepairNonEdits(os.Stderr, reviewPlans)
	}

	accepted := reviewPlans
	if !yes {
		accepted, err = confirmRepairPlans(reviewPlans, jsonOut)
		if err != nil {
			return fmt.Errorf("repair: %w", err)
		}
	}

	if len(accepted) > 0 {
		newContent, applied, warnings := applyRepairPlans(string(content), accepted, today)
		if newContent != string(content) {
			if err := writeStateFile(progressPath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("repair: write %s: %w", progressPath, err)
			}
			out.Changed = true
		}
		out.Applied = append(out.Applied, applied...)
		out.Warnings = append(out.Warnings, warnings...)
		if !jsonOut && len(applied) > 0 {
			fmt.Fprintf(os.Stderr, "\n\033[1mReviewed\033[0m — applied %d change(s):\n", len(applied))
			for _, a := range applied {
				fmt.Fprintf(os.Stderr, "  \033[32m✓\033[0m %s\n", summariseRepairPlan(a))
			}
		}
	}

	if jsonOut {
		return emitJSON(out)
	}
	renderRepairRejections(os.Stderr, out.Rejected)
	for _, w := range out.Warnings {
		fmt.Fprintf(os.Stderr, "  \033[33m⚠\033[0m %s\n", w)
	}
	// Re-scan the file as it now stands rather than reasoning from the plans:
	// an escalation resolves nothing, and a plan that did not apply left the
	// finding exactly where it was, so what the file still reports is the only
	// measure that cannot drift.
	//
	// Then subtract the orphans the review tier ruled are not tasks. A `leave`
	// verdict DOES resolve an orphan — `task_outside_milestone` is a warning,
	// `belmont validate` exits 0 on it, and nothing needs editing — while the
	// same verdict on an unreadable marker or a cross-milestone ID resolves
	// nothing, because those still block the loop whatever anyone calls them.
	// Counting the re-scan alone ended the run with "1 finding(s) unresolved —
	// edit those lines by hand" directly under "left as written (not a task)"
	// for the same line, which is the contradiction this measure exists to
	// avoid. A `leave` writes nothing, so every left orphan is still in the
	// re-scan and the subtraction is exact.
	unresolved := 0
	if final, err := os.ReadFile(progressPath); err == nil {
		unresolved = len(collectRepairFindings(string(final)))
	}
	for _, p := range reviewPlans {
		if p.Action.Action == repairLeave && p.Finding.Rule == ruleTaskOutsideMilestone && unresolved > 0 {
			unresolved--
		}
	}
	renderRepairNextSteps(os.Stderr, slug, out.Changed, unresolved)
	return nil
}

func emitJSON(v any) error {
	// A nil slice marshals to `null`, and the interactive skill iterates these
	// straight out of the JSON. Empty list, not null — for ALL THREE lists
	// declared without `omitempty`. `applied` was missed the first time and
	// emitted `null` on every dry run, which is the first command the skill
	// runs.
	if r, ok := v.(repairJSON); ok {
		if r.Findings == nil {
			r.Findings = []repairFinding{}
		}
		if r.NeedsReview == nil {
			r.NeedsReview = []repairFinding{}
		}
		if r.Applied == nil {
			r.Applied = []repairPlan{}
		}
		v = r
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// renderRepairFindings prints what still needs a code read.
func renderRepairFindings(w io.Writer, slug string, findings []repairFinding, evidenceAvailable bool) {
	if len(findings) == 0 {
		return
	}
	if !evidenceAvailable {
		fmt.Fprintf(w, "\033[33m⚠ The commit log could not be read here, so nothing was settled mechanically.\033[0m\n")
		fmt.Fprintf(w, "  Run repair from inside the git working tree that holds this project's history.\n\n")
	}
	fmt.Fprintf(w, "\033[1m%d finding(s) need a code read:\033[0m\n", len(findings))
	for _, f := range findings {
		fmt.Fprintf(w, "  [%s] PROGRESS.md:%-4d %s\n", f.Marker, f.Line, repairFindingHeadline(f))
		fmt.Fprintf(w, "        %s\n", repairEvidenceLine(f))
	}
	fmt.Fprintln(w)
}

func repairFindingHeadline(f repairFinding) string {
	label := f.TaskID
	if label == "" {
		label = "(no task ID)"
	}
	text := f.Text
	if len([]rune(text)) > 64 {
		text = string([]rune(text)[:63]) + "…"
	}
	switch f.Rule {
	case ruleUnrecognisedMarker:
		return fmt.Sprintf("%s — %s  \033[2m(marker Belmont cannot read)\033[0m", label, text)
	case ruleTaskOutsideMilestone:
		return fmt.Sprintf("%s — %s  \033[2m(outside every milestone)\033[0m", label, text)
	case ruleCrossMilestoneTaskID:
		return fmt.Sprintf("%s — %s  \033[2m(filed under %s, ID names %s)\033[0m", label, text, f.Milestone, f.NamedMilestone)
	}
	return fmt.Sprintf("%s — %s", label, text)
}

func repairEvidenceLine(f repairFinding) string {
	switch {
	case f.TaskID == "":
		return "\033[2mno task ID, so the commit log cannot speak to it\033[0m"
	case !f.Evidence.Checked:
		return "\033[2mcommit log not readable\033[0m"
	case f.Evidence.Found && f.Evidence.Ambiguous:
		return fmt.Sprintf("\033[2mcommit %s %q names it, but another feature uses this task ID too\033[0m", shortSHA(f.Evidence.SHA), f.Evidence.Subject)
	case f.Evidence.Found && f.Evidence.Reverting:
		return fmt.Sprintf("\033[2mthe newest commit naming it is a revert: %s %q\033[0m", shortSHA(f.Evidence.SHA), f.Evidence.Subject)
	case f.Evidence.Found:
		return fmt.Sprintf("\033[2mcommit %s %q names it\033[0m", shortSHA(f.Evidence.SHA), f.Evidence.Subject)
	default:
		return "\033[2mno commit on this branch names it\033[0m"
	}
}

// renderRepairNonEdits prints the review tier's "no change" verdicts.
// renderUnearnedVerified prints the audit block.
//
// Worded as a question, not a verdict, because it is one: a commit-less task
// can be genuinely verified. Overstating it here is how the block gets ignored,
// and an ignored audit is worse than none — it looks like coverage.
func renderUnearnedVerified(w io.Writer, findings []repairFinding, shallow bool) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(w, "\033[33m%d task(s) marked [v] that the commit log does not settle:\033[0m\n", len(findings))
	for i, f := range findings {
		if i == diagnosticListCap {
			fmt.Fprintf(w, "  … and %d more\n", len(findings)-diagnosticListCap)
			break
		}
		text := f.Text
		if len([]rune(text)) > 56 {
			text = string([]rune(text)[:55]) + "…"
		}
		// The marker AS WRITTEN, like renderRepairFindings. `[V]` is what made
		// this gap visible in the first place; printing it as `[v]` hides the
		// spelling the reader has to go and find.
		fmt.Fprintf(w, "  [%s] PROGRESS.md:%-4d %s — %s\n", f.Marker, f.Line, f.TaskID, text)
		// The evidence line, because "no commit names it" is not true of all of
		// them: an ID another feature also claims is listed here precisely
		// BECAUSE a commit names it and the match may be about that feature.
		fmt.Fprintf(w, "        %s\n", repairEvidenceLine(f))
	}
	fmt.Fprintln(w, "  Nothing audits a [v] that was already on disk when a run started — the commit-evidence")
	fmt.Fprintln(w, "  guard only ever compares one phase's before and after. A commit-less task can still be")
	fmt.Fprintln(w, "  genuinely verified (docs- or config-only work), so this is a question, not a verdict.")
	if shallow {
		// `git log` succeeds in a shallow clone and returns what it has, so
		// without this the report turns a history it could only partly read
		// into an accusation against work committed before the cut.
		fmt.Fprintln(w, "  \033[33mThis clone is shallow, so most of the history was not searched — expect false alarms.\033[0m")
		fmt.Fprintln(w, "  `git fetch --unshallow` first if you intend to act on this.")
	}
	fmt.Fprintln(w)
}

func renderRepairNonEdits(w io.Writer, plans []repairPlan) {
	var noted []repairPlan
	for _, p := range plans {
		if p.Action.Action == repairLeave || p.Action.Action == repairEscalate {
			noted = append(noted, p)
		}
	}
	if len(noted) == 0 {
		return
	}
	fmt.Fprintf(w, "\n\033[1m%d finding(s) left exactly as written:\033[0m\n", len(noted))
	for _, p := range noted {
		fmt.Fprintf(w, "  • %s\n", describeRepairPlan(p))
	}
}

func renderRepairRejections(w io.Writer, rejected []repairRejection) {
	if len(rejected) == 0 {
		return
	}
	fmt.Fprintf(w, "\n\033[33m%d proposed action(s) refused:\033[0m\n", len(rejected))
	for _, r := range rejected {
		label := r.Action.TaskID
		if label == "" {
			label = fmt.Sprintf("PROGRESS.md:%d", r.Action.Line)
		}
		fmt.Fprintf(w, "  • %s %s — %s\n", label, r.Action.Action, r.Reason)
	}
}

func renderRepairNextSteps(w io.Writer, slug string, changed bool, unresolved int) {
	fmt.Fprintln(w)
	if changed {
		fmt.Fprintf(w, "  Review the diff before committing:  git diff -- .belmont/features/%s/PROGRESS.md\n", slug)
	}
	// An escalation is a finding repair looked at and could not settle. Saying
	// "confirm the file now parses" and nothing else points the user at a
	// command that will exit 1 on exactly those lines, with no route forward.
	if unresolved > 0 {
		fmt.Fprintf(w, "  \033[33m%d finding(s) unresolved\033[0m — edit those lines by hand; `belmont validate --feature %s` names each one\n", unresolved, slug)
	}
	fmt.Fprintf(w, "  Confirm the file now parses:        belmont validate --feature %s\n", slug)
	fmt.Fprintf(w, "  Repair stops at [x] — to earn [v]:  belmont reverify --feature %s\n", slug)
}

// confirmRepairPlans walks the reviewed proposals with the user.
//
// Non-interactive runs apply NOTHING from the review tier without --yes. The
// commit-evidence tier has already run by this point and needs no permission —
// it is a fact about the repository — but a judgement call about what a task
// meant is not something to apply to someone's file behind their back.
func confirmRepairPlans(plans []repairPlan, quiet bool) ([]repairPlan, error) {
	var editing []repairPlan
	for _, p := range plans {
		if p.Action.Action == repairLeave || p.Action.Action == repairEscalate {
			continue
		}
		editing = append(editing, p)
	}
	if len(editing) == 0 {
		if !quiet {
			fmt.Fprintln(os.Stderr, "\n  The review tier proposed no edits.")
		}
		return nil, nil
	}
	if !isTerminal(os.Stdin) {
		fmt.Fprintf(os.Stderr, "\n\033[33m%d proposed edit(s), not applied — no terminal to confirm at.\033[0m\n", len(editing))
		for _, p := range editing {
			fmt.Fprintf(os.Stderr, "  • %s\n", describeRepairPlan(p))
		}
		fmt.Fprintln(os.Stderr, "  Re-run with --yes to apply them.")
		return nil, nil
	}

	reader := bufio.NewReader(os.Stdin)
	var accepted []repairPlan
	all := false
	fmt.Fprintf(os.Stderr, "\n\033[1m%d proposed edit(s):\033[0m\n", len(editing))
	for i, p := range editing {
		fmt.Fprintf(os.Stderr, "\n  [%d/%d] %s\n", i+1, len(editing), describeRepairPlan(p))
		fmt.Fprintf(os.Stderr, "        %s\n", repairEvidenceLine(p.Finding))
		if all {
			accepted = append(accepted, p)
			continue
		}
		fmt.Fprint(os.Stderr, "  Apply? [y/N/a=all/q=quit] ")
		answer, err := reader.ReadString('\n')
		if err != nil {
			return accepted, nil
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			accepted = append(accepted, p)
		case "a", "all":
			all = true
			accepted = append(accepted, p)
		case "q", "quit":
			return accepted, nil
		}
	}
	return accepted, nil
}

func describeRepairPlan(p repairPlan) string {
	label := repairLabel(p.Finding)
	switch p.Action.Action {
	case repairSetMarker:
		return fmt.Sprintf("%s  [%s] → [%s]  — %s", label, p.Finding.Marker, p.Action.Marker, p.Action.Reason)
	case repairWithdraw:
		return fmt.Sprintf("%s  [%s] → [-] withdrawn (reason recorded in ## Decisions Log) — %s", label, p.Finding.Marker, p.Action.Reason)
	case repairMoveMilestone:
		return fmt.Sprintf("%s  move to %s — %s", label, p.Action.Milestone, p.Action.Reason)
	case repairLeave:
		// "not a task" is right for a stray bullet and wrong for a verified
		// task whose work is genuinely there — the reviewer would read it as
		// the tool having misunderstood its own finding.
		if p.Finding.Rule == ruleVerifiedWithoutEvidence || p.Finding.AlsoUnverified {
			return fmt.Sprintf("%s  left verified — %s", label, p.Action.Reason)
		}
		return fmt.Sprintf("%s  left as written (not a task) — %s", label, p.Action.Reason)
	case repairEscalate:
		return fmt.Sprintf("%s  escalated — %s", label, p.Action.Reason)
	}
	return fmt.Sprintf("%s  %s", label, p.Action.Action)
}
