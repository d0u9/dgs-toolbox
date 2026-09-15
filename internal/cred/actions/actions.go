// Package actions is what can be done with an entry of an opened vault file:
// which Actions apply to which kinds, and for each the plan of what it writes,
// checked before anything is written. The catalogue is docs/apps/cred/actions.md.
package actions

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/opened"
	"dgs-toolbox/internal/cred/publish"
	"dgs-toolbox/internal/cred/sshagent"
	"dgs-toolbox/internal/cred/sshconfig"
)

// ID names an Action.
type ID string

const (
	SSHInstall ID = "ssh.install"
	SSHAgent   ID = "ssh.agent"
	SSHConfig  ID = "ssh.config"
	AgeInstall ID = "age.install"
	FileSave   ID = "file.save"
)

// Action is one registered Action.
type Action struct {
	ID    ID
	Label string
	Kinds []opened.Kind
}

// DefaultKeyDir is where ssh.install puts keys.
const DefaultKeyDir = "~/.ssh/keys"

// DefaultLifetime is how long ssh.agent asks the agent to keep a key.
const DefaultLifetime = time.Hour

// All returns every Action, in the order they are offered.
func All() []Action {
	return []Action{
		{ID: SSHInstall, Label: "Install for SSH login", Kinds: []opened.Kind{opened.KindSSHPrivate}},
		{ID: SSHAgent, Label: "Add to ssh-agent", Kinds: []opened.Kind{opened.KindSSHPrivate}},
		{ID: SSHConfig, Label: "Install as SSH configuration", Kinds: []opened.Kind{opened.KindText}},
		{ID: AgeInstall, Label: "Install as an age identity", Kinds: []opened.Kind{opened.KindAgeKey}},
		{ID: FileSave, Label: "Save to a folder", Kinds: []opened.Kind{opened.KindSSHPrivate, opened.KindSSHPublic, opened.KindAgeKey, opened.KindText}},
	}
}

// For returns the Actions that apply to kind.
func For(kind opened.Kind) []Action {
	var found []Action
	for _, action := range All() {
		for _, k := range action.Kinds {
			if k == kind {
				found = append(found, action)
			}
		}
	}
	return found
}

// Env is where this machine keeps things.
type Env struct {
	Home           string
	NewIdentityDir string
}

// Expand resolves a leading ~ against the home directory.
func (e Env) Expand(p string) string {
	if p == "~" {
		return e.Home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(e.Home, p[2:])
	}
	return p
}

func (e Env) sshDir() string { return filepath.Join(e.Home, ".ssh") }

// Write is one file a plan creates.
type Write struct {
	Path    string
	Data    []byte
	Perm    fs.FileMode
	DirPerm fs.FileMode
}

// Plan is what an Action will do, worked out and checked before anything is
// written.
type Plan struct {
	Writes []Write
	// Kept are existing files the Action would otherwise write, found to hold
	// the same content already.
	Kept []string
	// IncludeIn is the SSH configuration file to gain the config.d Include as
	// its first line; empty for none.
	IncludeIn string
}

// Apply carries the plan out. Every path is checked again first, so a file that
// appeared since planning makes it write nothing.
func (p Plan) Apply() error {
	for _, w := range p.Writes {
		if _, err := os.Lstat(w.Path); err == nil {
			return fmt.Errorf("%s %w", w.Path, publish.ErrExists)
		}
	}
	for _, w := range p.Writes {
		if err := publish.Create(w.Path, w.Data, w.Perm, w.DirPerm); err != nil {
			return err
		}
	}
	if p.IncludeIn != "" {
		current, err := os.ReadFile(p.IncludeIn)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(p.IncludeIn), 0o700); err != nil {
			return err
		}
		if err := publish.Replace(p.IncludeIn, sshconfig.WithInclude(current, sshconfig.IncludePattern), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Clear overwrites the plan's content once it is no longer needed.
func (p Plan) Clear() {
	for _, w := range p.Writes {
		clear(w.Data)
	}
}

func refuseExisting(paths ...string) error {
	for _, p := range paths {
		if _, err := os.Lstat(p); err == nil {
			return fmt.Errorf("%s already exists", p)
		}
	}
	return nil
}

// NeedsInclude reports whether ~/.ssh/config lacks the config.d Include.
func NeedsInclude(env Env) bool {
	has, err := sshconfig.Includes(filepath.Join(env.sshDir(), "config"), sshconfig.IncludePattern, env.Home)
	return err == nil && !has
}

func (p *Plan) include(env Env, add bool) {
	if add && NeedsInclude(env) {
		p.IncludeIn = filepath.Join(env.sshDir(), "config")
	}
}

// KeyLocations are the directories offered for ssh.install, the default first.
var KeyLocations = []string{DefaultKeyDir, "~/.ssh"}

// SSHInstallOptions are ssh.install's choices.
type SSHInstallOptions struct {
	// Name is the key's file name, and the Host alias when a Host entry is
	// written.
	Name string
	// KeyDir is where the key goes; empty means DefaultKeyDir.
	KeyDir string
	// WriteHost writes ~/.ssh/config.d/<name>.conf; Host supplies its fields,
	// and AddInclude adds the Include ~/.ssh/config needs to read it.
	WriteHost  bool
	Host       sshconfig.Host
	AddInclude bool
}

// PlanSSHInstall installs a private key with its public key, and optionally a
// Host entry for it.
func PlanSSHInstall(entry opened.Entry, options SSHInstallOptions, env Env) (Plan, error) {
	if entry.Kind != opened.KindSSHPrivate {
		return Plan{}, fmt.Errorf("%s is not an SSH private key", entry.Path)
	}
	keyDir := options.KeyDir
	if keyDir == "" {
		keyDir = DefaultKeyDir
	}
	host := options.Host
	host.Alias = options.Name
	host.IdentityFile = path.Join(keyDir, options.Name)
	if options.WriteHost {
		if err := host.Check(); err != nil {
			return Plan{}, err
		}
	} else if err := (sshconfig.Host{Alias: options.Name, HostName: "-"}).Check(); err != nil {
		return Plan{}, err
	}
	found := identities.Parse(entry.Data)
	if len(found) == 0 || found[0].Public.Key == "" {
		return Plan{}, errors.New("its public key cannot be derived")
	}
	public := found[0].Public.Key

	keyPath := filepath.Join(env.Expand(keyDir), options.Name)
	pubPath := keyPath + ".pub"
	confPath := filepath.Join(env.sshDir(), "config.d", options.Name+".conf")
	check := []string{keyPath}
	if options.WriteHost {
		check = append(check, confPath)
	}
	if err := refuseExisting(check...); err != nil {
		return Plan{}, err
	}
	plan := Plan{Writes: []Write{
		{Path: keyPath, Data: append([]byte(nil), entry.Data...), Perm: 0o600, DirPerm: 0o700},
	}}
	if existing, err := os.ReadFile(pubPath); err == nil {
		key, _, _, _, parseErr := ssh.ParseAuthorizedKey(existing)
		if parseErr != nil || strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))) != public {
			return Plan{}, fmt.Errorf("%s already exists and holds a different key", pubPath)
		}
		plan.Kept = append(plan.Kept, pubPath)
	} else {
		plan.Writes = append(plan.Writes, Write{Path: pubPath, Data: []byte(public + " " + options.Name + "\n"), Perm: 0o644, DirPerm: 0o700})
	}
	if options.WriteHost {
		plan.Writes = append(plan.Writes, Write{Path: confPath, Data: []byte(host.Block()), Perm: 0o600, DirPerm: 0o700})
		plan.include(env, options.AddInclude)
	}
	return plan, nil
}

// PlanSSHConfig installs text as ~/.ssh/config.d/<name>.conf.
func PlanSSHConfig(entry opened.Entry, name string, addInclude bool, env Env) (Plan, error) {
	if entry.Kind != opened.KindText {
		return Plan{}, fmt.Errorf("%s is not text", entry.Path)
	}
	if err := checkName(name); err != nil {
		return Plan{}, err
	}
	confPath := filepath.Join(env.sshDir(), "config.d", name+".conf")
	if err := refuseExisting(confPath); err != nil {
		return Plan{}, err
	}
	plan := Plan{Writes: []Write{{Path: confPath, Data: append([]byte(nil), entry.Data...), Perm: 0o600, DirPerm: 0o700}}}
	plan.include(env, addInclude)
	return plan, nil
}

// PlanAgeInstall installs an age identity in the new identity directory.
func PlanAgeInstall(entry opened.Entry, file string, env Env) (Plan, error) {
	if entry.Kind != opened.KindAgeKey {
		return Plan{}, fmt.Errorf("%s is not an age identity", entry.Path)
	}
	if err := checkName(file); err != nil {
		return Plan{}, err
	}
	for _, identity := range identities.Parse(entry.Data) {
		if identity.Status != identities.Usable {
			return Plan{}, fmt.Errorf("line %d: %s", identity.Line, identity.Message)
		}
	}
	target := filepath.Join(env.NewIdentityDir, file)
	if err := refuseExisting(target); err != nil {
		return Plan{}, err
	}
	return Plan{Writes: []Write{{Path: target, Data: append([]byte(nil), entry.Data...), Perm: 0o600, DirPerm: 0o700}}}, nil
}

// PlanSave saves an entry to dir/name. Keys are written 0600; other content
// keeps its mode.
func PlanSave(entry opened.Entry, dir, name string, env Env) (Plan, error) {
	if len(For(entry.Kind)) == 0 || entry.Data == nil {
		return Plan{}, fmt.Errorf("%s cannot be saved", entry.Path)
	}
	if err := checkName(name); err != nil {
		return Plan{}, err
	}
	target := filepath.Join(env.Expand(dir), name)
	if err := refuseExisting(target); err != nil {
		return Plan{}, err
	}
	perm := entry.Mode.Perm()
	if entry.Kind.Sensitive() || perm == 0 {
		perm = 0o600
	}
	return Plan{Writes: []Write{{Path: target, Data: append([]byte(nil), entry.Data...), Perm: perm, DirPerm: 0o755}}}, nil
}

func checkName(name string) error {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return fmt.Errorf("%q is not a single file name", name)
	}
	return nil
}

// AddToAgent adds a private key entry to the agent at socket.
func AddToAgent(entry opened.Entry, socket, comment string, lifetime time.Duration, confirm bool) error {
	if entry.Kind != opened.KindSSHPrivate {
		return fmt.Errorf("%s is not an SSH private key", entry.Path)
	}
	return sshagent.Add(socket, sshagent.Key{PEM: entry.Data, Comment: comment, Lifetime: lifetime, Confirm: confirm})
}
