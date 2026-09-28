package conf

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/inventory"
)

// reportServiceReferences finds literal old facts in authored service files.
// These are review candidates: the same string may be a comment, a historical
// example, or a hostname which intentionally stays unchanged after the move.
func reportServiceReferences(out io.Writer, root, oldID string, oldNode inventory.Node, changes []networkChange, instances []instanceChange, routes []routeChange) error {
	terms := map[string]bool{oldID: true}
	for _, change := range changes {
		if change.From != change.To {
			terms[change.From] = true
		}
		if change.Address != "" && change.Address != oldNode.Networks[change.From] {
			terms[oldNode.Networks[change.From]] = true
		}
	}
	delete(terms, "")
	matches, err := scanMigrationText(root, "services", terms)
	if err != nil {
		return fmt.Errorf("scanning service references: %w", err)
	}
	fmt.Fprintln(out, "Service references to review:")
	printMigrationMatches(out, matches)
	ids := map[string]bool{}
	for _, change := range instances {
		ids[change.From] = true
	}
	for _, change := range routes {
		ids[change.From] = true
	}
	if len(ids) != 0 {
		matches, err := scanMigrationText(root, "nodes", ids)
		if err != nil {
			return fmt.Errorf("scanning node references: %w", err)
		}
		fmt.Fprintln(out, "Node-file references to review (includes typed and opaque fields):")
		printMigrationMatches(out, matches)
	}
	var instanceMatches []string
	renamed := map[string]bool{}
	for _, change := range instances {
		renamed[change.From] = true
	}
	for _, inst := range oldNode.Instances {
		if renamed[inst.ID] {
			continue
		}
		for term := range terms {
			if strings.Contains(inst.ID, term) {
				instanceMatches = append(instanceMatches, fmt.Sprintf("  %s: %s contains %q (ID unchanged)", inst.Path, inst.ID, term))
			}
		}
	}
	sort.Strings(instanceMatches)
	fmt.Fprintln(out, "Instance IDs to review:")
	printMigrationMatches(out, instanceMatches)
	return nil
}

func scanMigrationText(root, subtree string, terms map[string]bool) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(filepath.Join(root, subtree), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.IndexByte(string(data), 0) >= 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for line := 1; scanner.Scan(); line++ {
			kind := "配置/模板内容"
			if strings.HasPrefix(strings.TrimSpace(scanner.Text()), "#") {
				kind = "注释"
			}
			for term := range terms {
				if strings.Contains(scanner.Text(), term) {
					matches = append(matches, fmt.Sprintf("  %s:%d [%s]: %q", rel, line, kind, term))
				}
			}
		}
		return scanner.Err()
	})
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

func printMigrationMatches(out io.Writer, matches []string) {
	if len(matches) == 0 {
		fmt.Fprintln(out, "  none found")
	} else {
		for _, match := range matches {
			fmt.Fprintln(out, match)
		}
	}
}
