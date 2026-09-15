package actions

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"filippo.io/age"
	"golang.org/x/crypto/ssh"

	"dgs-toolbox/internal/cred/opened"
	"dgs-toolbox/internal/cred/sshconfig"
)

func sshEntry(t *testing.T) (opened.Entry, string) {
	t.Helper()
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(private, "")
	signer, _ := ssh.NewSignerFromKey(private)
	public := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	data := pem.EncodeToMemory(block)
	return opened.Entry{Path: "server1/server1", Kind: opened.KindSSHPrivate, Data: data, Mode: 0o600}, public
}

func TestDocumentNamesEveryAction(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	document, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "apps", "cred", "actions.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range All() {
		if !strings.Contains(string(document), "`"+string(action.ID)+"`") {
			t.Errorf("docs/apps/cred/actions.md does not name %s — add it there in the same change", action.ID)
		}
	}
}

func TestFor(t *testing.T) {
	var ids []string
	for _, a := range For(opened.KindSSHPrivate) {
		ids = append(ids, string(a.ID))
	}
	if strings.Join(ids, ",") != "ssh.install,ssh.agent,file.save" {
		t.Errorf("ssh private %v", ids)
	}
	if len(For(opened.KindOther)) != 0 || len(For(opened.KindDirectory)) != 0 || len(For(opened.KindUnsafe)) != 0 {
		t.Error("actions offered for binary, directories or unsafe entries")
	}
}

func TestSSHInstall(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home}
	entry, public := sshEntry(t)
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte("Host old\n  User me\n"), 0o644)

	options := SSHInstallOptions{Name: "server1", WriteHost: true, Host: sshconfig.Host{HostName: "203.0.113.10", User: "root"}, AddInclude: true}
	plan, err := PlanSSHInstall(entry, options, env)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(home, ".ssh", "keys", "server1")
	if data, _ := os.ReadFile(keyPath); string(data) != string(entry.Data) {
		t.Error("key not written")
	}
	if info, _ := os.Stat(keyPath); info.Mode().Perm() != 0o600 {
		t.Errorf("key mode %04o", info.Mode().Perm())
	}
	if info, _ := os.Stat(filepath.Dir(keyPath)); info.Mode().Perm() != 0o700 {
		t.Errorf("keys dir mode %04o", info.Mode().Perm())
	}
	if data, _ := os.ReadFile(keyPath + ".pub"); string(data) != public+" server1\n" {
		t.Errorf("pub %q", data)
	}
	conf, _ := os.ReadFile(filepath.Join(home, ".ssh", "config.d", "server1.conf"))
	if !strings.Contains(string(conf), "HostName 203.0.113.10") || !strings.Contains(string(conf), "IdentityFile ~/.ssh/keys/server1") || !strings.Contains(string(conf), "IdentitiesOnly yes") {
		t.Errorf("conf:\n%s", conf)
	}
	config, _ := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if string(config) != "Include ~/.ssh/config.d/*\n\nHost old\n  User me\n" {
		t.Errorf("config:\n%s", config)
	}
	if info, _ := os.Stat(filepath.Join(home, ".ssh", "config")); info.Mode().Perm() != 0o644 {
		t.Errorf("config mode changed: %04o", info.Mode().Perm())
	}

	// Again: refused before writing anything.
	if _, err := PlanSSHInstall(entry, options, env); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second install: %v", err)
	}
	// Another alias for the same key keeps an identical .pub only if named alike;
	// here the include is already present.
	plan, err = PlanSSHInstall(entry, SSHInstallOptions{Name: "server2", KeyDir: "~/.ssh/keys", WriteHost: true, Host: sshconfig.Host{HostName: "example.com"}, AddInclude: true}, env)
	if err != nil || plan.IncludeIn != "" {
		t.Errorf("include planned twice: %+v %v", plan, err)
	}

	// A different key already in the .pub refuses.
	os.WriteFile(filepath.Join(home, ".ssh", "keys", "server3.pub"), []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGK03FOmAmtKfe0bcskMOsZfg/vkZZn+XoSZu15AZlVV x\n"), 0o644)
	if _, err := PlanSSHInstall(entry, SSHInstallOptions{Name: "server3", WriteHost: true, Host: sshconfig.Host{HostName: "h"}}, env); err == nil || !strings.Contains(err.Error(), "different key") {
		t.Errorf("different pub: %v", err)
	}
	if _, err := PlanSSHInstall(entry, SSHInstallOptions{Name: "server4", WriteHost: true}, env); err == nil || !strings.Contains(err.Error(), "host name is required") {
		t.Errorf("no host name: %v", err)
	}

	// Without a Host entry: the key and its .pub only, anywhere, no host name.
	plan, err = PlanSSHInstall(entry, SSHInstallOptions{Name: "work", KeyDir: "~/Secrets/ssh", AddInclude: true}, Env{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Writes) != 2 || plan.IncludeIn != "" || !strings.HasSuffix(plan.Writes[0].Path, "Secrets/ssh/work") || !strings.HasSuffix(plan.Writes[1].Path, "work.pub") {
		t.Errorf("plan without host %+v", plan)
	}
	if _, err := PlanSSHInstall(entry, SSHInstallOptions{Name: "bad name"}, env); err == nil {
		t.Error("bad name accepted without a Host entry")
	}
}

func TestSSHConfigAgeAndSave(t *testing.T) {
	home := t.TempDir()
	env := Env{Home: home, NewIdentityDir: filepath.Join(home, ".config", "age")}

	text := opened.Entry{Path: "server1/config", Kind: opened.KindText, Data: []byte("Host server1\n  HostName h\n"), Mode: 0o644}
	plan, err := PlanSSHConfig(text, "server1", false, env)
	if err != nil || plan.IncludeIn != "" {
		t.Fatalf("%+v %v", plan, err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "config")); err == nil {
		t.Error("config created without asking")
	}

	identity, _ := age.GenerateX25519Identity()
	ageEntry := opened.Entry{Path: "k", Kind: opened.KindAgeKey, Data: []byte(identity.String() + "\n")}
	plan, err = PlanAgeInstall(ageEntry, "server1.agekey", env)
	if err != nil {
		t.Fatal(err)
	}
	plan.Apply()
	if info, err := os.Stat(filepath.Join(env.NewIdentityDir, "server1.agekey")); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("age install: %v", err)
	}

	sshKey, _ := sshEntry(t)
	sshKey.Mode = 0o644
	plan, err = PlanSave(sshKey, "~/out", "server1", env)
	if err != nil {
		t.Fatal(err)
	}
	plan.Apply()
	if info, _ := os.Stat(filepath.Join(home, "out", "server1")); info.Mode().Perm() != 0o600 {
		t.Errorf("saved key mode %04o", info.Mode().Perm())
	}
	plan, _ = PlanSave(text, "~/out", "config", env)
	plan.Apply()
	if info, _ := os.Stat(filepath.Join(home, "out", "config")); info.Mode().Perm() != 0o644 {
		t.Errorf("saved text mode %04o", info.Mode().Perm())
	}
	if _, err := PlanSave(text, "~/out", "config", env); err == nil {
		t.Error("save over an existing file planned")
	}
	if _, err := PlanSave(opened.Entry{Path: "b", Kind: opened.KindOther, Data: []byte{0}}, "~/out", "b", env); err == nil {
		t.Error("binary save planned")
	}
}

func TestApplyWritesNothingWhenAPathAppears(t *testing.T) {
	home := t.TempDir()
	entry, _ := sshEntry(t)
	plan, err := PlanSSHInstall(entry, SSHInstallOptions{Name: "s", WriteHost: true, Host: sshconfig.Host{HostName: "h"}}, Env{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(home, ".ssh", "config.d", "s.conf")
	os.MkdirAll(filepath.Dir(conf), 0o700)
	os.WriteFile(conf, []byte("mine"), 0o600)
	if err := plan.Apply(); err == nil {
		t.Fatal("applied over a file that appeared")
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh", "keys", "s")); err == nil {
		t.Error("key written although the plan was refused")
	}
}
