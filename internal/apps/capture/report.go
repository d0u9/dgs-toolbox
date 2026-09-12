package capture

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/config"
)

// writeRecipeReport answers "which Recipes does this binary offer, and what
// does each one need?" without opening the TUI. It is written from the registry
// rather than from a hand-kept list, so it cannot drift from what Route
// actually offers.
func writeRecipeReport(out io.Writer, global config.Config) error {
	loaded := loadRecipes(global)
	recipes := loaded.Set.All()
	fmt.Fprintf(out, "RECIPES  %d\n", len(recipes))
	writeRecipeSources(out, loaded)
	for _, recipe := range recipes {
		state := ""
		if recipe.Disabled {
			state = "  · disabled"
		}
		fmt.Fprintf(out, "\n  %s%s\n", recipe.Name, state)
		fmt.Fprintf(out, "    id         %s\n", recipe.ID)
		fmt.Fprintf(out, "    source     %s\n", recipe.Source)
		fmt.Fprintf(out, "    workflows  %s\n", workflowList(recipe))
		if len(recipe.Match.RequiresAny) > 0 {
			fmt.Fprintf(out, "    requires   %s\n", joinFields(recipe.Match.RequiresAny, " or "))
		}
		fmt.Fprintln(out, "    actions")
		for _, id := range recipe.Actions {
			def, ok := organizer.LookupAction(id)
			if !ok {
				fmt.Fprintf(out, "      %-28s ! unknown action\n", id)
				continue
			}
			off := ""
			if !recipe.DefaultEnabled()[id] {
				off = "  · off by default"
			}
			fmt.Fprintf(out, "      %-28s %s%s\n", id, requirementList(def.Required), off)
		}
		if len(recipe.Fields) > 0 {
			fmt.Fprintf(out, "    also       %s\n", requirementList(recipe.Fields))
		}
	}
	writeFieldReference(out, recipes)
	if len(loaded.Failures) > 0 {
		// A rejected file is the first thing its author needs to see, so it is
		// repeated at the end where the report stops scrolling.
		fmt.Fprintf(out, "\nFAILED  %d\n\n", len(loaded.Failures))
		for _, failure := range loaded.Failures {
			fmt.Fprintf(out, "  %s\n    %v\n", failure.Path, failure.Err)
		}
	}
	return nil
}

// writeStarterRecipes lays the shipped Recipes down in the configured Recipe
// directory. They are written rather than loaded because a Recipe is something
// the reader owns: once it is a file in their directory, editing it is the
// obvious thing and nothing in the binary argues with what they wrote.
//
// A file that is already there is kept, never overwritten. Someone running this
// a second time is asking for what is missing, not for their own edits to be
// undone.
func writeStarterRecipes(out io.Writer, global config.Config) error {
	dir := expandHome(global.CaptureRecipesDir())
	if dir == "" {
		return errors.New("no recipe directory: configure capture.recipes, or a config_dir for it to sit under")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	fmt.Fprintf(out, "RECIPES  %s\n\n", dir)
	written := 0
	for _, starter := range organizer.Starters() {
		path := filepath.Join(dir, starter.Name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			fmt.Fprintf(out, "  kept     %s\n", starter.Name)
			continue
		}
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		if _, err := file.Write(starter.Data); err != nil {
			file.Close()
			return fmt.Errorf("write %s: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %s: %w", path, err)
		}
		fmt.Fprintf(out, "  written  %s\n", starter.Name)
		written++
	}
	fmt.Fprintf(out, "\n%d written, %d already there\n", written, len(organizer.Starters())-written)
	return nil
}

// writeActionReport answers "what can a Recipe name?" — every Action this
// binary carries, what it writes, what it needs, and what it can be told. It is
// written from the registry rather than from a hand-kept list, because a
// catalogue that drifts from the binary is worse than none: someone writing a
// Recipe against it gets an unknown-action error and no idea which name is
// right.
func writeActionReport(out io.Writer, _ config.Config) error {
	actions := organizer.Actions()
	fmt.Fprintf(out, "ACTIONS  %d\n", len(actions))
	for _, def := range actions {
		fmt.Fprintf(out, "\n  %s\n", def.Label)
		fmt.Fprintf(out, "    id         %s\n", def.ID)
		fmt.Fprintf(out, "    requires   %s\n", requirementList(def.Required))
		if def.Run == nil {
			fmt.Fprintf(out, "    state      declared, not implemented — a recipe naming it refuses to run\n")
		}
		for index, effect := range def.Effects {
			label := "writes"
			if index > 0 {
				label = ""
			}
			fmt.Fprintf(out, "    %-10s %s\n", label, effect)
		}
		for index, parameter := range def.Parameters {
			label := "settings"
			if index > 0 {
				label = ""
			}
			fmt.Fprintf(out, "    %-10s %s (%s)\n", label, parameter.Name, parameter.Label)
		}
	}
	writeActionFields(out, actions)
	return nil
}

// writeActionFields lists every field any Action may ask for, which is what a
// Recipe author needs when the Recipe does not exist yet.
func writeActionFields(out io.Writer, actions []organizer.ActionDefinition) {
	recipes := make([]organizer.Recipe, 0, len(actions))
	for _, def := range actions {
		recipes = append(recipes, organizer.Recipe{Actions: []organizer.ActionID{def.ID}})
	}
	writeFieldReference(out, recipes)
}

// writeRecipeSources says where the Set came from, which is the question a
// reader has when a Recipe they wrote is not in the list.
func writeRecipeSources(out io.Writer, loaded organizer.Loaded) {
	dir := loaded.Dir
	if dir == "" {
		dir = "· no recipe directory configured"
	}
	fmt.Fprintf(out, "  from     %s\n", dir)
	fmt.Fprintf(out, "  files    %d read, %d failed\n", loaded.Files-len(loaded.Failures), len(loaded.Failures))
}

// writeFieldReference lists the logical fields these Recipes name, because a
// requirement is written by field id and those ids exist nowhere else a reader
// can look them up.
func writeFieldReference(out io.Writer, recipes []organizer.Recipe) {
	fields := map[organizer.FieldID]organizer.InputType{}
	for _, recipe := range recipes {
		for _, req := range recipe.RequiredFields(recipe.Actions) {
			fields[req.Field] = req.Input
		}
	}
	ids := make([]string, 0, len(fields))
	for field := range fields {
		ids = append(ids, string(field))
	}
	sort.Strings(ids)

	fmt.Fprintf(out, "\nFIELDS  %d in use\n\n", len(ids))
	for _, id := range ids {
		fmt.Fprintf(out, "  %-24s %s\n", id, fields[organizer.FieldID(id)])
	}
}

func workflowList(recipe organizer.Recipe) string {
	if len(recipe.Match.Workflows) == 0 {
		return "any"
	}
	return strings.Join(recipe.Match.Workflows, ", ")
}

func joinFields(fields []organizer.FieldID, separator string) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, string(field))
	}
	return strings.Join(parts, separator)
}

// requirementList renders a requirement as the id a Recipe would write, marking
// the required ones and naming the condition on a conditional one.
func requirementList(requirements []organizer.FieldRequirement) string {
	if len(requirements) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(requirements))
	for _, req := range requirements {
		part := string(req.Field)
		if req.Required {
			part += "*"
		}
		if req.When != nil {
			part += " (conditional)"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

// obsidianSettings is where the Actions write, from the configuration. The
// folder and filename format are not configured: they are read from the vault,
// which already says what they are.
func obsidianSettings(global config.Config) organizer.Settings {
	obsidian := global.CaptureObsidian()
	settings := organizer.DefaultSettings()
	settings.ObsidianVault = expandHome(obsidian.Vault)
	settings.TemplateDir = expandHome(global.CaptureTemplatesDir())
	settings.DailyNote = obsidian.DailyNote
	settings.Mappings = global.Capture.Mappings
	if obsidian.Section != "" {
		settings.DailySection = obsidian.Section
	}
	settings.LocationNote = obsidian.LocationNote
	settings.LocationArchive = obsidian.LocationArchive
	settings.MapServices = obsidian.MapServices
	settings.CoordinateChoice = obsidian.CoordinateChoice
	return settings
}

// loadRecipes reads the configured Recipe directory. Failures are carried
// rather than returned as an error: a bad Recipe file must not stop the session
// from opening, only that Recipe from being offered.
func loadRecipes(global config.Config) organizer.Loaded {
	return organizer.Load(expandHome(global.CaptureRecipesDir()))
}
