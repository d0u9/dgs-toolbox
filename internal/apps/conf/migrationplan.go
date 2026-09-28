package conf

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"dgs-toolbox/internal/cred/publish"
	"gopkg.in/yaml.v3"
)

const migrationPlanVersion = 1

var migrationNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

type migrationPlan struct {
	Version    int                `yaml:"version"`
	SourceNode string             `yaml:"source_node"`
	Rows       []migrationPlanRow `yaml:"rows"`
}

type migrationPlanRow struct {
	Kind string `yaml:"kind"`
	Key  string `yaml:"key"`
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

func migrationRowKey(row migrationRow) string {
	switch row.kind {
	case "network", "address":
		return row.network
	case "published":
		return row.instance + ":" + row.port
	default:
		return row.before
	}
}

func migrationPlanFromTable(m *migrationTable) migrationPlan {
	p := migrationPlan{Version: migrationPlanVersion, SourceNode: m.selectedNode}
	for _, row := range m.rows {
		if row.kind == "secret" {
			continue
		}
		p.Rows = append(p.Rows, migrationPlanRow{Kind: row.kind, Key: migrationRowKey(row), From: row.before, To: row.after})
	}
	return p
}

func applyMigrationPlan(m *migrationTable, p migrationPlan) error {
	if p.Version != migrationPlanVersion {
		return fmt.Errorf("unsupported migration plan version %d", p.Version)
	}
	m.selectedNode = ""
	m.rows = nil
	m.selectNode(p.SourceNode)
	if m.selectedNode != p.SourceNode || len(m.editableRows()) != len(p.Rows) {
		return fmt.Errorf("migration source inventory has changed; create a new plan")
	}
	for i, saved := range p.Rows {
		current := m.editableRows()[i]
		if saved.Kind != current.kind || saved.Key != migrationRowKey(current) || saved.From != current.before {
			return fmt.Errorf("migration source inventory has changed at %s; create a new plan", saved.Key)
		}
		if saved.To == "" {
			return fmt.Errorf("migration plan has an empty target at %s", saved.Key)
		}
	}
	for i, saved := range p.Rows {
		m.rows[i].after = saved.To
	}
	m.refreshSecretRows()
	m.refreshRows()
	return nil
}

func migrationPlanPath(dir, name string) (string, error) {
	base := name
	ext := filepath.Ext(name)
	if ext == ".yaml" || ext == ".yml" {
		base = strings.TrimSuffix(name, ext)
	}
	if !migrationNamePattern.MatchString(base) || (ext != "" && ext != ".yaml" && ext != ".yml") {
		return "", fmt.Errorf("plan name must use letters, digits, hyphens or underscores")
	}
	if ext == "" {
		name += ".yaml"
	}
	return filepath.Join(dir, name), nil
}

func listMigrationPlans(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-len(".yaml")]
		if migrationNamePattern.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func readMigrationPlan(path string) (migrationPlan, [32]byte, error) {
	var p migrationPlan
	var digest [32]byte
	info, err := os.Lstat(path)
	if err != nil {
		return p, digest, err
	}
	if !info.Mode().IsRegular() {
		return p, digest, fmt.Errorf("migration plan is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return p, digest, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&p); err != nil {
		return p, digest, err
	}
	return p, sha256.Sum256(data), nil
}

func writeMigrationPlan(path string, p migrationPlan, known *[32]byte) ([32]byte, error) {
	var digest [32]byte
	data, err := yaml.Marshal(p)
	if err != nil {
		return digest, err
	}
	if known == nil {
		err = publish.Create(path, data, 0o600, 0o700)
	} else {
		_, current, readErr := readMigrationPlan(path)
		if readErr != nil {
			return digest, readErr
		}
		if current != *known {
			return digest, fmt.Errorf("migration plan changed on disk; reopen it before saving")
		}
		err = publish.Replace(path, data, 0o600)
	}
	if err != nil {
		return digest, err
	}
	return sha256.Sum256(data), nil
}
