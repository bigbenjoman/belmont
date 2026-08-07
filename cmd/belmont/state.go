package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func milestoneAllDone(m milestone) bool {
	if len(m.Tasks) == 0 {
		return false
	}
	for _, t := range m.Tasks {
		if t.Status != taskDone && t.Status != taskVerified {
			return false
		}
	}
	return true
}

func milestoneAllVerified(m milestone) bool {
	if len(m.Tasks) == 0 {
		return false
	}
	for _, t := range m.Tasks {
		if t.Status != taskVerified {
			return false
		}
	}
	return true
}

func milestoneHasBlockers(m milestone) bool {
	for _, t := range m.Tasks {
		if t.Status == taskBlocked {
			return true
		}
	}
	return false
}

func milestoneNotStarted(m milestone) bool {
	for _, t := range m.Tasks {
		if t.Status != taskTodo {
			return false
		}
	}
	return true
}

// parseMasterDeps reads the master PROGRESS.md and extracts feature slug → dependency slugs mapping
// from the ## Features table. Handles "None", empty, and comma-separated slugs.
// New table format: | Feature | Slug | Priority | Dependencies | Status | Milestones | Tasks |
func parseMasterDeps(root string) (deps map[string][]string, priorities map[string]string) {
	deps = make(map[string][]string)
	priorities = make(map[string]string)

	progressPath := filepath.Join(root, ".belmont", "PROGRESS.md")
	content, err := os.ReadFile(progressPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	colIdx := parseMasterTableColumns(lines)
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Features") {
			inTable = true
			continue
		}

		if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}

		if !inTable || !strings.HasPrefix(trimmed, "|") {
			continue
		}

		cells := splitTableCells(trimmed)
		slugCol := colIdx["Slug"]
		prioCol := colIdx["Priority"]
		depCol := colIdx["Dependencies"]

		if slugCol < 0 || len(cells) <= slugCol {
			continue
		}

		slug := strings.TrimSpace(cells[slugCol])
		if slug == "Slug" || strings.HasPrefix(slug, "-") || strings.HasPrefix(slug, ":") {
			continue
		}

		if prioCol >= 0 && prioCol < len(cells) {
			priorities[slug] = strings.TrimSpace(cells[prioCol])
		}

		if depCol < 0 || depCol >= len(cells) {
			continue
		}
		depStr := strings.TrimSpace(cells[depCol])
		if depStr == "" || strings.EqualFold(depStr, "None") || depStr == "-" {
			continue
		}

		var depSlugs []string
		for _, d := range strings.Split(depStr, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				depSlugs = append(depSlugs, d)
			}
		}
		if len(depSlugs) > 0 {
			deps[slug] = depSlugs
		}
	}
	return
}

// flattenTasks extracts all tasks from parsed milestones, sorted by task ID.
func flattenTasks(milestones []milestone, maxName int) []task {
	var tasks []task
	for _, m := range milestones {
		for _, t := range m.Tasks {
			name := t.Name
			if maxName > 0 && len([]rune(name)) > maxName {
				name = string([]rune(name)[:maxName-1]) + "…"
			}
			tasks = append(tasks, task{ID: t.ID, Name: name, Status: t.Status, MilestoneID: t.MilestoneID})
		}
	}

	sort.Slice(tasks, func(i, j int) bool {
		pi, ni := parseTaskOrder(tasks[i].ID)
		pj, nj := parseTaskOrder(tasks[j].ID)
		if pi != pj {
			return pi < pj
		}
		return ni < nj
	})

	return tasks
}

func parseMilestones(progress string) []milestone {
	// Match milestone headers: ### M1: Name
	msRe := regexp.MustCompile(`(?m)^###\s+M(\d+):\s*(.+)$`)
	depsRe := regexp.MustCompile(`\(depends:\s*(M[\d]+(?:\s*,\s*M[\d]+)*)\)\s*$`)
	// Match task checkboxes: - [ ] P0-1: Task Name, - [x] ..., - [>] ..., - [v] ..., - [!] ...
	taskRe := regexp.MustCompile(`(?m)^\s*-\s+\[(.)\]\s+(.+)$`)

	lines := strings.Split(progress, "\n")
	var milestones []milestone
	var currentMS *milestone

	for _, line := range lines {
		// Check for milestone header
		if msMatch := msRe.FindStringSubmatch(line); len(msMatch) >= 3 {
			// Save previous milestone
			if currentMS != nil {
				milestones = append(milestones, *currentMS)
			}

			id := "M" + strings.TrimSpace(msMatch[1])
			name := strings.TrimSpace(msMatch[2])

			// Extract dependency annotations from name
			var deps []string
			if depsMatch := depsRe.FindStringSubmatch(name); len(depsMatch) >= 2 {
				name = strings.TrimSpace(depsRe.ReplaceAllString(name, ""))
				for _, d := range strings.Split(depsMatch[1], ",") {
					deps = append(deps, strings.TrimSpace(d))
				}
			}

			currentMS = &milestone{ID: id, Name: name, Deps: deps}
			continue
		}

		// Check for next section (## header) — stops current milestone
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			if currentMS != nil {
				milestones = append(milestones, *currentMS)
				currentMS = nil
			}
			continue
		}

		// Parse task checkboxes under current milestone
		if currentMS != nil {
			if taskMatch := taskRe.FindStringSubmatch(line); len(taskMatch) >= 3 {
				marker := taskMatch[1]
				taskText := strings.TrimSpace(taskMatch[2])

				var status taskStatus
				switch marker {
				case " ":
					status = taskTodo
				case ">":
					status = taskInProgress
				case "x":
					status = taskDone
				case "v":
					status = taskVerified
				case "!":
					status = taskBlocked
				default:
					status = taskTodo
				}

				// Extract task ID if present (e.g., "P0-1: Task Name")
				taskID := ""
				taskName := taskText
				idRe := regexp.MustCompile(`^(P\d+-[\w][\w-]*):\s*(.+)$`)
				if idMatch := idRe.FindStringSubmatch(taskText); len(idMatch) >= 3 {
					taskID = idMatch[1]
					taskName = strings.TrimSpace(idMatch[2])
				}

				currentMS.Tasks = append(currentMS.Tasks, task{
					ID:          taskID,
					Name:        taskName,
					Status:      status,
					MilestoneID: currentMS.ID,
				})
			}
		}
	}

	// Don't forget the last milestone
	if currentMS != nil {
		milestones = append(milestones, *currentMS)
	}

	return milestones
}
