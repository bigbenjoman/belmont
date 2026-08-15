package main

import (
	"strings"
	"testing"
)

// `flag.ContinueOnError` makes `fs.Parse` return `flag.ErrHelp` for `-h`/`--help`,
// and `fs.SetOutput(io.Discard)` suppresses the usage the flag package would
// otherwise print. Every call site wrapped that sentinel as an ordinary error, so
// `belmont <cmd> --help` printed `<cmd>: flag: help requested` and exited 1.
//
// The non-zero exit is the part that bites beyond cosmetics: `--help` in a
// Makefile, a CI script or a doc-check fails, and the message gives the reader
// nothing to act on. See issue #50.

// everyCommandWithFlags is the full set of subcommands that build a FlagSet.
// Keeping them in one table is the point: eleven parse sites across nine files
// each answered "what does --help mean" separately, and all eleven answered it
// wrongly and identically.
var everyCommandWithFlags = []struct {
	name string
	run  func([]string) error
}{
	{"status", runStatus},
	{"auto", runAutoCmd},
	{"install", runInstall},
	{"update", runUpdate},
	{"recover", runRecover},
	{"steer", runSteerCmd},
	{"validate", runValidateCmd},
	{"reverify", runReverifyCmd},
	{"repair", runRepairCmd},
	{"blockers", runBlockersCmd},
	{"sync", runSyncCmd},
}

func TestEverySubcommandTreatsHelpAsSuccess(t *testing.T) {
	for _, c := range everyCommandWithFlags {
		for _, flagSpelling := range []string{"--help", "-h"} {
			t.Run(c.name+" "+flagSpelling, func(t *testing.T) {
				var err error
				out := captureStdout(t, func() { err = c.run([]string{flagSpelling}) })
				if err != nil {
					t.Fatalf("belmont %s %s returned an error (exit 1): %v", c.name, flagSpelling, err)
				}
				if !strings.Contains(out, "belmont "+c.name) {
					t.Errorf("help output does not name the command.\ngot:\n%s", out)
				}
				if strings.Contains(out, "flag: help requested") {
					t.Errorf("the ErrHelp sentinel leaked into the output.\ngot:\n%s", out)
				}
			})
		}
	}
}

// Help must print the command's OWN flags, not just a synopsis line. That is
// what stops the hand-maintained usage string being the only place a flag could
// have been documented — the defect part 2 of #50 is about.
//
// Asserted on each flag's DESCRIPTION rather than its name, deliberately. The
// synopsis line already contains `-feature`, `-all` and friends as substrings, so
// a name-based assertion passed with `PrintDefaults` deleted — it was matching
// the synopsis it was supposed to be checking past. Only the FlagSet emits these
// strings.
func TestSubcommandHelpListsItsOwnFlags(t *testing.T) {
	cases := map[string][]string{
		"auto": {
			"feature slug (required)",
			"comma-separated feature slugs for parallel execution",
			"run all pending features in parallel",
		},
		"reverify": {
			"start milestone (e.g. M3)",
			"output format (text|json)",
		},
		"recover": {
			"list preserved worktrees",
			"clean all preserved worktrees",
			"act on worktrees an active run owns",
		},
	}
	byName := map[string]func([]string) error{}
	for _, c := range everyCommandWithFlags {
		byName[c.name] = c.run
	}
	for name, wantFlags := range cases {
		t.Run(name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := byName[name]([]string{"--help"}); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			})
			for _, description := range wantFlags {
				if !strings.Contains(out, description) {
					t.Errorf("belmont %s --help does not print its own flag defaults — missing %q.\ngot:\n%s",
						name, description, out)
				}
			}
		})
	}
}

// Part 2 of the issue. `--features` and `--all` are real, carry their own help
// strings, and surfaced only in an error message — so a reader concluded that
// parallelism meant "within one feature" and never found the mode the
// `--max-parallel` flag exists for.
func TestAutoUsageNamesTheMultiFeatureFlags(t *testing.T) {
	var sb strings.Builder
	printUsage(&sb)
	usage := sb.String()

	autoLine := ""
	for _, l := range strings.Split(usage, "\n") {
		if strings.Contains(l, "belmont auto") {
			autoLine = l
			break
		}
	}
	if autoLine == "" {
		t.Fatalf("no `belmont auto` line in the usage output:\n%s", usage)
	}
	for _, want := range []string{"--features", "--all"} {
		if !strings.Contains(autoLine, want) {
			t.Errorf("the auto usage line omits %s, which is a real mode:\n  %s", want, autoLine)
		}
	}
}

// The synopsis a command prints for itself and the one `belmont --help` prints
// must be the same text. Two hand-maintained copies is how the first one drifted.
func TestCommandHelpAndTopLevelUsageAgree(t *testing.T) {
	var sb strings.Builder
	printUsage(&sb)
	topLevel := sb.String()

	for _, c := range everyCommandWithFlags {
		out := captureStdout(t, func() {
			if err := c.run([]string{"--help"}); err != nil {
				t.Fatalf("unexpected error for %s: %v", c.name, err)
			}
		})
		synopsis := ""
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "belmont "+c.name+" ") || strings.TrimSpace(l) == "belmont "+c.name {
				synopsis = strings.TrimSpace(l)
				break
			}
		}
		if synopsis == "" {
			t.Errorf("%s --help printed no synopsis line.\ngot:\n%s", c.name, out)
			continue
		}
		if !strings.Contains(topLevel, synopsis) {
			t.Errorf("%s --help prints a synopsis the top-level usage does not have:\n  %s", c.name, synopsis)
		}
	}
}

// The guard against over-correcting: a genuinely bad flag must still fail. A
// helper that swallowed every parse error would satisfy every test above.
func TestUnknownFlagIsStillAnError(t *testing.T) {
	for _, c := range everyCommandWithFlags {
		t.Run(c.name, func(t *testing.T) {
			var err error
			captureStdout(t, func() { err = c.run([]string{"--definitely-not-a-flag"}) })
			if err == nil {
				t.Errorf("belmont %s --definitely-not-a-flag exited 0", c.name)
			}
		})
	}
}
