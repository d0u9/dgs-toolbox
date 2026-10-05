package photo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/apps/photo/importer"
	"dgs-toolbox/internal/apps/photo/postprocess"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/pageactions"
	"dgs-toolbox/internal/tui/scrolllist"
	"dgs-toolbox/internal/verifiedcopy"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	targetChoiceID = "target-choice"
	addTargetID    = "add-target"
	removeTargetID = "remove-target"
)

type multipleTransferEventMsg struct {
	target int
	event  importer.Event
}

// multipleFileMsg is a source's overall outcome across every target.
type multipleFileMsg importer.FileResult

type multipleTransferDoneMsg struct {
	result importer.MultipleResult
	err    error
}

func (m importModel) destinationRoots() []string {
	return append([]string{m.paths[destinationField]}, m.extraDestinations...)
}
func (m importModel) selectedDestination() string {
	roots := m.destinationRoots()
	return roots[min(m.selectedTarget, len(roots)-1)]
}
func destinationControlID(index int) string {
	if index == 0 {
		return destinationID
	}
	return fmt.Sprintf("destination-%d", index+1)
}
func (m importModel) directoryPathIDs() []string {
	ids := []string{sourceID}
	for i := range m.destinationRoots() {
		ids = append(ids, destinationControlID(i))
	}
	return ids
}
func (m importModel) focusedDestination() (int, bool) {
	for i := range m.destinationRoots() {
		if m.controls.Focused(destinationControlID(i)) {
			return i, true
		}
	}
	return 0, false
}
func (m importModel) setupControlIDs() []string {
	return append(m.directoryPathIDs(), addTargetID, removeTargetID)
}
func (m *importModel) rebuildDirectoryControls() {
	ids := []string{sourceID, addTargetID, removeTargetID, targetChoiceID}
	for i := 0; i < max(1, m.directoryFieldCount); i++ {
		ids = append(ids, destinationControlID(i))
	}
	fields := []form.Field{{ID: sourceID, Kind: form.Path, Label: "Source", Value: m.paths[sourceField]}}
	for i, root := range m.destinationRoots() {
		fields = append(fields, form.Field{ID: destinationControlID(i), Kind: form.Path, Label: fmt.Sprintf("Destination %d", i+1), Value: root})
	}
	fields = append(fields, form.Field{ID: addTargetID, Kind: form.Button, Label: "Add destination"}, form.Field{ID: removeTargetID, Kind: form.Button, Label: fmt.Sprintf("Remove destination %d", m.selectedTarget+1)})
	m.controls.ReplaceFields(ids, fields)
	m.directoryFieldCount = len(m.destinationRoots())
}
func (m *importModel) updateTargetOptions() { m.rebuildDirectoryControls() }
func (m importModel) directoryButtons(width int) (string, int, int) {
	add := pageactions.Inline("Add destination", m.controls.Focused(addTargetID))
	remove := pageactions.Inline(fmt.Sprintf("Remove destination %d", m.selectedTarget+1), m.controls.Focused(removeTargetID))
	aw, rw := lipgloss.Width(add), lipgloss.Width(remove)
	return strings.Repeat(" ", max(0, width-aw-rw-2)) + add + "  " + remove, aw, rw
}
func (m *importModel) selectTarget(index int) {
	m.selectedTarget = index
	m.updateTargetOptions()
	if len(m.targetScans) > index {
		m.scan = m.targetScans[index]
		m.syncResultLists()
	}
}
func (m *importModel) syncTargetChoice() {
	if m.controls.IsActive() {
		return
	}
	for i := range m.destinationRoots() {
		if m.controls.Value(targetChoiceID) == fmt.Sprintf("Destination %d", i+1) {
			m.selectTarget(i)
			return
		}
	}
}
func (m *importModel) removeSelectedTarget() {
	roots := m.destinationRoots()
	if len(roots) == 1 {
		m.actionNotice = "At least one destination is required"
		return
	}
	roots = append(roots[:m.selectedTarget], roots[m.selectedTarget+1:]...)
	m.paths[destinationField] = roots[0]
	m.extraDestinations = append([]string(nil), roots[1:]...)
	m.selectedTarget = min(m.selectedTarget, len(roots)-1)
	m.targetScans = nil
	m.updateTargetOptions()
	m.actionNotice = "Destination removed"
}
func (m importModel) pickerPath(field int) string {
	if field == destinationField {
		return m.selectedDestination()
	}
	return m.paths[field]
}
func (m *importModel) acceptPickedPath(path string) bool {
	if m.pickerFor == destinationField {
		candidate, _ := filepath.Abs(expandHome(path))
		candidateInfo, _ := os.Stat(candidate)
		for i, root := range m.destinationRoots() {
			if !m.addingDestination && i == m.selectedTarget {
				continue
			}
			existing, _ := filepath.Abs(expandHome(root))
			existingInfo, _ := os.Stat(existing)
			same := filepath.Clean(candidate) == filepath.Clean(existing)
			if candidateInfo != nil && existingInfo != nil {
				same = same || os.SameFile(candidateInfo, existingInfo)
			}
			if same {
				m.actionNotice = fmt.Sprintf("Already selected as Destination %d", i+1)
				return false
			}
		}
	}
	path = displayPath(path)
	if m.pickerFor == sourceField {
		m.paths[sourceField] = path
		m.controls.SetValue(sourceID, path)
	} else {
		if m.addingDestination {
			m.extraDestinations = append(append([]string(nil), m.extraDestinations...), path)
			m.selectedTarget = len(m.extraDestinations)
		} else if m.selectedTarget == 0 {
			m.paths[destinationField] = path
		} else {
			m.extraDestinations = append([]string(nil), m.extraDestinations...)
			m.extraDestinations[m.selectedTarget-1] = path
		}
		m.updateTargetOptions()
	}
	m.addingDestination = false
	m.targetScans = nil
	m.actionNotice = ""
	return true
}
func (m importModel) scanAllTargetsCmd(generation int) tea.Cmd {
	roots := m.destinationRoots()
	source, state := m.paths[sourceField], m.stateFilename
	return func() tea.Msg {
		src := scanSource(expandHome(source))
		scans := make([]scanSummary, len(roots))
		for i, root := range roots {
			scans[i] = src
			scanTree("DST", root, state, &scans[i].destination, &scans[i].destinationErrors, &scans[i].destinationSkipped)
			if _, err := os.Stat(expandHome(root)); os.IsNotExist(err) {
				scans[i].destinationMissing = true
			}
		}
		return scanDoneMsg{generation: generation, summary: scans[0], targets: scans}
	}
}
func (m importModel) startMultipleProcessing() (tea.Model, tea.Cmd) {
	roots := m.destinationRoots()
	for i := range roots {
		roots[i] = expandHome(roots[i])
	}
	source := expandHome(m.paths[sourceField])
	if err := importer.ValidateTargets(source, roots); err != nil {
		m.actionNotice = err.Error()
		return m, nil
	}
	if m.controls.Checked(incrementalID) {
		for i, scan := range m.targetScans {
			if len(scan.destinationErrors) > 0 {
				m.actionNotice = fmt.Sprintf("Destination %d scan incomplete — press r to rescan", i+1)
				return m, nil
			}
		}
	}
	m.targetIdentities = make([]os.FileInfo, len(roots))
	for i, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			m.actionNotice = err.Error()
			return m, nil
		}
		m.targetIdentities[i] = info
	}
	files := append([]scannedFile(nil), m.filteredSource()...)
	operation := importer.Copy
	if m.controls.Value(operationID) == "Move" {
		operation = importer.Move
	}
	conflict := importer.Skip
	if m.controls.Value(duplicatesID) == "Keep both" {
		conflict = importer.KeepBoth
	}
	controller := &importer.Controller{}
	plans := make([]importer.Plan, len(roots))
	for i, root := range roots {
		plans[i] = importer.Plan{SourceRoot: source, DestinationRoot: root, Jobs: buildJobs(source, root, files), StatePath: filepath.Join(root, m.stateFilename), Workers: max(1, m.controls.IntValue(parallelID)), Conflict: conflict, Incremental: m.controls.Checked(incrementalID), Controller: controller}
	}
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan tea.Msg, 32)
	m.processing = processingState{files: files, finished: make(map[int]importer.Phase), moving: operation == importer.Move}
	m.resetMultipleWorkers()
	m.targetProcessing = make([]processingState, len(roots))
	for i := range roots {
		workers := append([]processingWorker(nil), m.processing.workers...)
		for j := range workers {
			workers[j].number = i*len(workers) + j + 1
		}
		m.targetProcessing[i] = processingState{files: files, finished: make(map[int]importer.Phase), workers: workers}
	}
	m.stage = processingStage
	m.actionNotice = ""
	m.processingTarget = -1
	m.targetResults = nil
	m.stateFailed = false
	if len(importer.SharedFilesystems(roots)) > 0 {
		m.actionNotice = "Warning: destinations share a filesystem; their copies are not independent of a disk failure"
	}
	m.transferCancel = cancel
	m.transferController = controller
	m.transferUpdates = updates
	go func() {
		result, err := importer.RunMultiple(ctx, plans, operation, importer.MultipleHooks{
			Event: func(target int, event importer.Event) {
				updates <- multipleTransferEventMsg{target: target, event: event}
			},
			File: func(file importer.FileResult) { updates <- multipleFileMsg(file) },
		})
		updates <- multipleTransferDoneMsg{result: result, err: err}
		close(updates)
	}()
	return m, waitTransferUpdate(updates)
}
func (m *importModel) resetMultipleWorkers() {
	m.processing.workers = make([]processingWorker, max(1, m.controls.IntValue(parallelID)))
	for i := range m.processing.workers {
		m.processing.workers[i] = processingWorker{number: i + 1, jobIndex: -1, phase: workerIdle}
	}
}
func (m *importModel) applyMultipleEvent(msg multipleTransferEventMsg) {
	if msg.target < 0 || msg.target >= len(m.targetProcessing) {
		return
	}
	aggregate := m.processing
	m.processing = m.targetProcessing[msg.target]
	m.processing.paused = aggregate.paused
	// Terminal notifications can be repeated by the coordinator.
	if _, done := m.processing.finished[msg.event.Index]; !done || !finalPhase(msg.event.Phase) {
		m.applyTransferEvent(msg.event)
	}
	m.targetProcessing[msg.target] = m.processing
	m.processing = aggregate
	m.processingTarget = msg.target
	m.processing.workers = nil
	for _, state := range m.targetProcessing {
		m.processing.workers = append(m.processing.workers, state.workers...)
	}
	if finalPhase(msg.event.Phase) {
		phase := importer.PhaseComplete
		existing := true
		for _, state := range m.targetProcessing {
			outcome, done := state.finished[msg.event.Index]
			if !done {
				return
			}
			existing = existing && outcome == importer.PhaseAlreadyPresent
			if outcome == importer.PhaseFailed || outcome == importer.PhaseUndetermined || (phase != importer.PhaseFailed && phase != importer.PhaseUndetermined && !imported(outcome)) {
				phase = outcome
			}
		}
		if _, done := m.processing.finished[msg.event.Index]; done {
			return
		}
		event := msg.event
		event.Phase = phase
		event.Existing = existing
		workers := m.processing.workers
		m.processing.workers = []processingWorker{{jobIndex: -1}}
		event.Worker = 1
		m.applyTransferEvent(event)
		m.processing.workers = workers
	}
}
func (m *importModel) applyMultipleResult(msg multipleTransferDoneMsg) {
	m.targetResults = msg.result.Targets
	results := append([]importer.FileResult(nil), msg.result.Files...)
	for i := range results {
		if len(msg.result.Targets) > 0 && i < len(msg.result.Targets[0].Files) {
			first := msg.result.Targets[0].Files[i]
			results[i].Destination = first.Destination
			results[i].Existing = true
			for _, target := range msg.result.Targets {
				if i >= len(target.Files) || (!target.Files[i].Existing && target.Files[i].Phase != importer.PhaseAlreadyPresent) {
					results[i].Existing = false
				}
			}
			results[i].DestinationHash = first.DestinationHash
		}
	}
	m.applyTransferResult(importer.Result{Files: results})
	m.countMultipleConflicts(results)
	m.processing.earlierKept = 0
	var notices []string
	if msg.err != nil {
		notices = append(notices, msg.err.Error())
		m.processing.failed++
		m.stateFailed = true
	}
	for i, target := range m.targetResults {
		if target.StateError != nil {
			m.stateFailed = true
			notices = append(notices, fmt.Sprintf("Destination %d state: %s", i+1, target.StateError))
		}
		if state, err := importer.LoadState(filepath.Join(expandHome(m.destinationRoots()[i]), m.stateFilename)); err == nil {
			m.processing.earlierKept += importer.EarlierSourceKept(state)
		}
	}
	if len(notices) > 0 {
		m.actionNotice = strings.Join(notices, "; ")
	}
	m.controls.SetChecked(deleteStateID, m.batchFinished())
}
func organizationFiles(source, root string, results []importer.FileResult) ([]postprocess.File, []postprocess.ExistingFile) {
	var files []postprocess.File
	var existing []postprocess.ExistingFile
	for _, result := range results {
		if !imported(result.Phase) {
			continue
		}
		file := postprocess.File{Source: result.Source, Path: result.Destination}
		if result.Existing || result.Phase == importer.PhaseAlreadyPresent {
			existing = append(existing, postprocess.ExistingFile{File: file, Digest: result.DestinationHash, Size: result.Size})
			if !unorganized(source, root, result) {
				continue
			}
		}
		files = append(files, file)
	}
	return files, existing
}
func (m importModel) planNextTargetOrganization() (tea.Model, tea.Cmd) {
	if m.postprocessTarget >= len(m.targetResults) {
		m.postprocess.running = false
		return m, nil
	}
	root := expandHome(m.destinationRoots()[m.postprocessTarget])
	files, existing := organizationFiles(expandHome(m.paths[sourceField]), root, m.targetResults[m.postprocessTarget].Files)
	m.postprocess.running = true
	return m.startOrganizationTask(func(ctx context.Context, report func(postprocess.Progress)) tea.Msg {
		plan := postprocess.NewPlanWithProgress(ctx, root, files, existing, report)
		report(postprocess.Progress{Phase: "Checking existing date folders"})
		return postprocessPlannedMsg{plan: plan, existing: plan.ExistingDates()}
	})
}
func (m *importModel) updateTargetOrganization(result postprocess.Result) {
	moved := make(map[string]string)
	for _, file := range result.Files {
		if file.Status == postprocess.Moved {
			moved[file.Original] = file.Final
		}
	}
	for i := range m.targetResults[m.postprocessTarget].Files {
		file := &m.targetResults[m.postprocessTarget].Files[i]
		if path, ok := moved[file.Destination]; ok {
			if file.PlannedDestination == "" {
				file.PlannedDestination = file.Destination
			}
			file.Destination = path
		}
	}
}
func (m *importModel) openMultipleResult() {
	var items []scrolllist.Item
	for target, result := range m.targetResults {
		root := expandHome(m.destinationRoots()[target])
		items = append(items, scrolllist.Item{ID: fmt.Sprintf("target-%d", target), Label: fmt.Sprintf("Destination %d/%d  %s", target+1, len(m.destinationRoots()), root)})
		for _, file := range result.Files {
			label := fmt.Sprintf("✓  %s  ·  %s  ·  SHA-256 MATCH", relativeTo(root, file.Destination), formatBytes(file.Size))
			if !imported(file.Phase) {
				label = fmt.Sprintf("!  %s  ·  %s  %s", relativeTo(root, file.Destination), file.Phase, file.Error)
			}
			if file.Existing || file.Phase == importer.PhaseAlreadyPresent {
				label += "  ·  already present"
			}
			if file.Warning != "" {
				label += "  ·  " + file.Warning
			}
			items = append(items, scrolllist.Item{ID: fmt.Sprintf("%d:%s", target, file.Source), Label: label})
		}
	}
	for target := len(m.targetResults); target < len(m.destinationRoots()); target++ {
		items = append(items, scrolllist.Item{ID: fmt.Sprintf("unstarted-%d", target), Label: fmt.Sprintf("! Destination %d/%d  not started  %s", target+1, len(m.destinationRoots()), m.destinationRoots()[target])})
	}
	for _, file := range m.processing.results {
		if file.Phase != importer.PhaseComplete {
			items = append(items, scrolllist.Item{ID: "overall:" + file.Source, Label: "!  " + filepath.Base(file.Source) + "  ·  " + string(file.Phase) + ": " + file.Error})
		}
	}
	m.resultList.SetItems(items)
	m.resultFields.Set("result-files")
	m.stage = resultStage
}
func (m *importModel) cleanupAllStates() bool {
	for i, root := range m.destinationRoots() {
		if i < len(m.targetIdentities) {
			if err := verifiedcopy.CheckRoot(expandHome(root), m.targetIdentities[i]); err != nil {
				m.actionNotice = err.Error()
				return false
			}
		}
		// Like the CLI, keep every state while one still records an earlier
		// Move whose sources were not deleted: it is what finishes that Move.
		if state, err := importer.LoadState(filepath.Join(expandHome(root), m.stateFilename)); err == nil && importer.EarlierSourceKept(state) > 0 {
			m.actionNotice = fmt.Sprintf("Destination %d state still records an earlier Move with sources to delete; uncheck Delete state to keep it", i+1)
			return false
		}
	}
	for i, root := range m.destinationRoots() {
		if i < len(m.targetIdentities) {
			if err := verifiedcopy.CheckRoot(expandHome(root), m.targetIdentities[i]); err != nil {
				m.actionNotice = err.Error()
				return false
			}
		}
		path := filepath.Join(expandHome(root), m.stateFilename)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			m.actionNotice = "Could not delete " + path + ": " + err.Error()
			return false
		}
	}
	m.actionNotice = "Deleted state in all destinations"
	return true
}

func (m importModel) destinationInventoryTitle() string {
	if len(m.extraDestinations) == 0 {
		return "Destination files"
	}
	return fmt.Sprintf("Destination %d/%d · [/]", m.selectedTarget+1, len(m.destinationRoots()))
}

func (m importModel) scanDestinationLines(width int) string {
	var lines []string
	for i, root := range m.destinationRoots() {
		label := "Destination"
		if len(m.extraDestinations) > 0 {
			label = fmt.Sprintf("Target %d/%d", i+1, len(m.destinationRoots()))
		}
		lines = append(lines, fmt.Sprintf("%-13s %s", label, ansi.Truncate(root, max(1, width-15), "…")))
	}
	return strings.Join(lines, "\n")
}

// skipTargetOrganization declines the current target's existing date folders
// and moves on to the next target, keeping every earlier target's outcome.
func (m importModel) skipTargetOrganization() (tea.Model, tea.Cmd) {
	m.postprocess.plan = nil
	m.postprocessTarget++
	if m.postprocessTarget < len(m.targetResults) {
		return m.planNextTargetOrganization()
	}
	m.postprocess.running = false
	return m, nil
}

// countMultipleConflicts reclassifies sources that failed overall only because
// some target held a different photo under the same name: an incremental
// conflict, as in a single-target run, not a transfer failure.
func (m *importModel) countMultipleConflicts(results []importer.FileResult) {
	for i, file := range results {
		if file.Phase != importer.PhaseFailed {
			continue
		}
		conflict := false
		other := false
		for _, target := range m.targetResults {
			if i >= len(target.Files) {
				other = true
				break
			}
			outcome := target.Files[i]
			switch {
			case outcome.Warning == importer.ConflictNotImported:
				conflict = true
			case !imported(outcome.Phase):
				other = true
			}
		}
		if conflict && !other {
			// Counted as a single-target run counts it: a skip and a conflict.
			m.processing.conflicts++
			m.processing.skipped++
			m.processing.failed--
		}
	}
}
