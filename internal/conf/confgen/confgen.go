// Package confgen reads a generator root: one directory per service, each
// declaring how its instances render. It only discovers what is there — the
// manifests, the roles, the instances and which of them are broken. It does
// not render anything.
//
// The rules are in docs/apps/conf/export.md.
package confgen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestFilename names the file that marks a subdirectory of the root as a
// service, and declares that service's roles.
const ManifestFilename = "confgen.yaml"

// DefaultsFilename is the defaults file inside every role directory.
const DefaultsFilename = "defaults.yaml"

// DefaultsDocument and DefaultsElement are the two values roles.<role>.defaults
// may take. See docs/apps/conf/export.md#two-kinds-of-defaults.
const (
	DefaultsDocument = "document"
	DefaultsElement  = "element"
)

// Role is one entry of a manifest's roles map. The map key, not stored here,
// is both the role's name and the directory its value files are in.
type Role struct {
	// Template is the template rendered for this role, relative to the
	// service directory.
	Template string `yaml:"template"`
	// Defaults is DefaultsDocument or DefaultsElement.
	Defaults string `yaml:"defaults"`
	// Output is the name the rendered file is written under.
	Output string `yaml:"output"`
}

// Manifest is a service's confgen.yaml.
type Manifest struct {
	// Secrets is the secrets file this service draws on, named relative to
	// conf.secrets.
	Secrets string `yaml:"secrets"`
	// Roles is one entry per kind of instance the service generates, keyed
	// by role name.
	Roles map[string]Role `yaml:"roles"`
}

// Instance is one *.yaml file found in a role's directory, other than
// defaults.yaml.
type Instance struct {
	// Name is the file name without its extension.
	Name string
	// Path is the file's path, relative to the generator root.
	Path string
	// Broken is the parse error if the file is not valid YAML or not a
	// mapping, and empty otherwise.
	Broken string
}

// RoleInstances is one role of a service, with the instances found in its
// directory.
type RoleInstances struct {
	Name      string
	Role      Role
	Instances []Instance
}

// Service is one subdirectory of the generator root that holds a
// confgen.yaml.
type Service struct {
	// Name is the subdirectory's name.
	Name string
	// Dir is the subdirectory's path, relative to the generator root.
	Dir string
	// Manifest is the parsed confgen.yaml, valid only when Broken is empty.
	Manifest Manifest
	// Roles is the service's roles, each with the instances found for it,
	// sorted by role name. Empty when Broken is not.
	Roles []RoleInstances
	// Broken is the manifest's parse error if confgen.yaml is not valid YAML,
	// not a mapping, or holds an unknown key, and empty otherwise.
	Broken string
}

// Root is a discovered generator root.
type Root struct {
	// Services is every subdirectory holding a confgen.yaml, sorted by name.
	Services []Service
}

// Load discovers a generator root: every subdirectory holding a
// confgen.yaml is a service, and is read whether or not it parses cleanly. A
// subdirectory without a confgen.yaml — a README, a scratch folder, a
// service still being written — is skipped rather than half-read.
func Load(root string) (*Root, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("confgen: reading root: %w", err)
	}

	var services []Service
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		dir := filepath.Join(root, name)
		manifestPath := filepath.Join(dir, ManifestFilename)
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}

		svc := Service{Name: name, Dir: name}

		manifest, err := loadManifest(manifestPath)
		if err != nil {
			svc.Broken = err.Error()
			services = append(services, svc)
			continue
		}
		svc.Manifest = *manifest

		roleNames := make([]string, 0, len(manifest.Roles))
		for roleName := range manifest.Roles {
			roleNames = append(roleNames, roleName)
		}
		sort.Strings(roleNames)

		for _, roleName := range roleNames {
			role := manifest.Roles[roleName]
			instances, err := loadInstances(root, dir, roleName)
			if err != nil {
				return nil, err
			}
			svc.Roles = append(svc.Roles, RoleInstances{
				Name:      roleName,
				Role:      role,
				Instances: instances,
			})
		}

		services = append(services, svc)
	}

	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return &Root{Services: services}, nil
}

func loadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var m Manifest
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &m, nil
}

// loadInstances lists every *.yaml in <serviceDir>/<roleName> other than
// defaults.yaml, sorted by name. A role directory that does not exist yields
// no instances rather than an error, since a role may be declared before any
// instance of it exists.
func loadInstances(root, serviceDir, roleName string) ([]Instance, error) {
	roleDir := filepath.Join(serviceDir, roleName)
	entries, err := os.ReadDir(roleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("confgen: reading %s: %w", roleDir, err)
	}

	var instances []Instance
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := entry.Name()
		if filepath.Ext(fileName) != ".yaml" {
			continue
		}
		if fileName == DefaultsFilename {
			continue
		}

		path := filepath.Join(roleDir, fileName)
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			relPath = path
		}
		inst := Instance{
			Name: strings.TrimSuffix(fileName, ".yaml"),
			Path: relPath,
		}

		data, err := os.ReadFile(path)
		if err != nil {
			inst.Broken = err.Error()
			instances = append(instances, inst)
			continue
		}
		var probe map[string]any
		if err := yaml.Unmarshal(data, &probe); err != nil {
			inst.Broken = err.Error()
		} else if probe == nil {
			inst.Broken = fmt.Sprintf("%s: not a mapping", path)
		}

		instances = append(instances, inst)
	}

	sort.Slice(instances, func(i, j int) bool { return instances[i].Name < instances[j].Name })
	return instances, nil
}
