package capture

import (
	"fmt"
	"io"
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
		fmt.Fprintf(out, "\n  %s\n", recipe.Name)
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
			fmt.Fprintf(out, "      %-28s %s\n", id, requirementList(def.Required))
		}
		if len(recipe.Fields) > 0 {
			fmt.Fprintf(out, "    also       %s\n", requirementList(recipe.Fields))
		}
	}
	writeFieldReference(out)
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

// writeFieldReference lists the logical fields a Recipe may name, because a
// requirement is written by field id and those ids exist nowhere else a reader
// can look them up.
func writeFieldReference(out io.Writer) {
	fields := map[organizer.FieldID]organizer.InputType{}
	for _, recipe := range organizer.Recipes() {
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

// loadRecipes reads the configured Recipe directory. Failures are carried
// rather than returned as an error: a bad Recipe file must not stop the session
// from opening, only that Recipe from being offered.
func loadRecipes(global config.Config) organizer.Loaded {
	return organizer.Load(expandHome(global.CaptureRecipesDir()))
}
