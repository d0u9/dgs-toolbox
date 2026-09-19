package recipients

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"filippo.io/age"
	"golang.org/x/crypto/ssh"
)

func ageKey(t *testing.T) string {
	t.Helper()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return identity.Recipient().String()
}

func sshKey(t *testing.T, public any) string {
	t.Helper()
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

func ed25519Key(t *testing.T) string {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return sshKey(t, public)
}

func rsaKey(t *testing.T, bits int) string {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return sshKey(t, &private.PublicKey)
}

func write(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hostJSON(keys ...[2]string) string {
	var entries []string
	for _, key := range keys {
		entries = append(entries, `{"public_key":`+quote(key[0])+`,"description":`+quote(key[1])+`}`)
	}
	return `{"keys":[` + strings.Join(entries, ",") + `]}`
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func load(t *testing.T, root string) Folder {
	t.Helper()
	folder, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return folder
}

// problems returns "severity file: message" lines, for asserting on.
func problems(folder Folder) []string {
	var lines []string
	for _, problem := range folder.Problems {
		lines = append(lines, problem.Severity.String()+" "+filepath.ToSlash(problem.File)+": "+problem.Message)
	}
	return lines
}

func hasProblem(t *testing.T, folder Folder, severity Severity, file, fragment string) {
	t.Helper()
	for _, problem := range folder.Problems {
		if problem.Severity == severity && filepath.ToSlash(problem.File) == file && strings.Contains(problem.Message, fragment) {
			return
		}
	}
	t.Errorf("no %s on %s containing %q; got %q", severity, file, fragment, problems(folder))
}

func TestParsePublicKey(t *testing.T) {
	x25519 := ageKey(t)
	ed := ed25519Key(t)
	for name, test := range map[string]struct {
		text    string
		typ     KeyType
		key     string
		refused string
	}{
		"age":                 {text: x25519, typ: TypeX25519, key: x25519},
		"ssh comment dropped": {text: ed + " root@nas", typ: TypeED25519, key: ed},
		"surrounding space":   {text: "  " + ed + "\n", typ: TypeED25519, key: ed},
		"rsa 2048":            {text: rsaKey(t, 2048), typ: TypeRSA},
		"rsa 1024":            {text: rsaKey(t, 1024), refused: "shorter than 2048"},
		"empty":               {text: " ", refused: "empty"},
		"garbage":             {text: "hello", refused: "not an age or SSH public key"},
		"bad age checksum":    {text: x25519[:len(x25519)-1] + "q", refused: "malformed"},
		"plugin":              {text: "age1yubikey1qwt50d05nh5vutpdzmlg5wn80xq5negm4uj9ghv0snvdd3yysf5yw3rhl3t", refused: "plugin recipients (age1yubikey1…)"},
		"post-quantum":        {text: "age1pq1qqqq", refused: "post-quantum"},
		"options":             {text: `no-pty ` + ed, refused: "options"},
		"two keys":            {text: ed + "\n" + ed, refused: "more than one"},
	} {
		key, err := ParsePublicKey(test.text)
		if test.refused != "" {
			if err == nil || !strings.Contains(err.Error(), test.refused) {
				t.Errorf("%s: want refusal containing %q, got %v", name, test.refused, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if key.Type != test.typ {
			t.Errorf("%s: type %s, want %s", name, key.Type, test.typ)
		}
		if test.key != "" && key.Key != test.key {
			t.Errorf("%s: key %q, want %q", name, key.Key, test.key)
		}
		if (key.Type == TypeX25519) != (key.Fingerprint == "") {
			t.Errorf("%s: fingerprint %q", name, key.Fingerprint)
		}
	}
}

func TestParsePublicKeyRefusesECDSA(t *testing.T) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePublicKey(sshKey(t, &private.PublicKey)); err == nil || !strings.Contains(err.Error(), "ecdsa-sha2-nistp256") {
		t.Errorf("ecdsa accepted or unexplained: %v", err)
	}
}

func TestLoadsHostsAndGroups(t *testing.T) {
	root := t.TempDir()
	a, b := ageKey(t), ed25519Key(t)
	write(t, root, "hosts/is-rkv-nas-linux-01.json", hostJSON([2]string{a, " Main age key "}, [2]string{b + " root@nas", "SSH host key"}))
	write(t, root, "hosts/laptop.json", hostJSON([2]string{ageKey(t), "Main"}))
	write(t, root, "groups/g-servers.json", `{"hosts":["IS-RKV-NAS-LINUX-01","laptop"]}`)

	folder := load(t, root)
	if len(folder.Problems) != 0 {
		t.Fatalf("problems: %q", problems(folder))
	}
	if len(folder.Hosts) != 2 || folder.Hosts[0].Name != "is-rkv-nas-linux-01" || folder.Hosts[1].Name != "laptop" {
		t.Fatalf("hosts: %+v", folder.Hosts)
	}
	nas := folder.Hosts[0]
	if len(nas.Keys) != 2 || nas.Keys[0].Description != "Main age key" || nas.Keys[1].Key != b {
		t.Errorf("nas keys: %+v", nas.Keys)
	}
	if filepath.ToSlash(nas.File) != "hosts/is-rkv-nas-linux-01.json" {
		t.Errorf("file %q", nas.File)
	}
	if len(folder.Groups) != 1 || strings.Join(folder.Groups[0].Hosts, ",") != "is-rkv-nas-linux-01,laptop" {
		t.Errorf("groups: %+v", folder.Groups)
	}
	if folder.Errors() {
		t.Error("Errors() on a clean folder")
	}
}

func TestMissingSubdirectoriesAreEmpty(t *testing.T) {
	folder := load(t, t.TempDir())
	if len(folder.Hosts)+len(folder.Groups)+len(folder.Problems) != 0 {
		t.Errorf("not empty: %+v", folder)
	}
}

func TestMissingRootIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("loaded a folder that does not exist")
	}
}

func TestIgnoredAndReportedEntries(t *testing.T) {
	root := t.TempDir()
	write(t, root, "hosts/.gitkeep", "")
	write(t, root, "hosts/.hidden.json", "not json")
	write(t, root, "hosts/README.md", "notes")
	write(t, root, "hosts/cloud/vps.json", hostJSON([2]string{ageKey(t), "x"}))
	folder := load(t, root)
	if len(folder.Hosts) != 0 {
		t.Errorf("hosts: %+v", folder.Hosts)
	}
	if lines := problems(folder); len(lines) != 1 {
		t.Fatalf("problems: %q", lines)
	}
	hasProblem(t, folder, Warning, "hosts/cloud", "subdirectories are not read")
}

func TestAFileWithAnErrorIsLeftOut(t *testing.T) {
	root := t.TempDir()
	good := ageKey(t)
	write(t, root, "hosts/good.json", hostJSON([2]string{good, "ok"}))
	cases := map[string]struct{ body, fragment string }{
		"bad-json":    {`{"keys":[`, "invalid JSON"},
		"unknown":     {`{"keys":[],"name":"x"}`, "unknown field"},
		"trailing":    {`{"keys":[]} {}`, "content after the object"},
		"no-desc":     {hostJSON([2]string{ageKey(t), "  "}), "description is required"},
		"bad-key":     {hostJSON([2]string{"nope", "x"}), "key 1: not an age or SSH public key"},
		"dup-key":     {hostJSON([2]string{good, "a"}, [2]string{good, "b"}), "key 2 is the same public key as key 1"},
		"unknown-key": {`{"keys":[{"public_key":"` + ageKey(t) + `","description":"x","kind":"age"}]}`, "unknown field"},
	}
	for name, c := range cases {
		write(t, root, "hosts/"+name+".json", c.body)
	}
	// dup-key's shared key must not also warn good.json, because dup-key is out.
	folder := load(t, root)
	if len(folder.Hosts) != 1 || folder.Hosts[0].Name != "good" {
		t.Errorf("hosts: %+v", folder.Hosts)
	}
	for name, c := range cases {
		hasProblem(t, folder, Error, "hosts/"+name+".json", c.fragment)
	}
	for _, problem := range folder.Problems {
		if problem.File == filepath.Join("hosts", "good.json") {
			t.Errorf("good.json reported: %s", problem.Message)
		}
	}
	if !folder.Errors() {
		t.Error("Errors() false")
	}
}

func TestNames(t *testing.T) {
	root := t.TempDir()
	key := func() string { return hostJSON([2]string{ageKey(t), "x"}) }
	write(t, root, "hosts/g-oops.json", key())
	write(t, root, "hosts/has space.json", key())
	write(t, root, "hosts/plain_name.v2.json", key())
	write(t, root, "groups/servers.json", `{"hosts":[]}`)
	write(t, root, "groups/g-ok.json", `{}`)

	folder := load(t, root)
	hasProblem(t, folder, Error, "hosts/g-oops.json", "kept for groups")
	hasProblem(t, folder, Error, "hosts/has space.json", "may use only")
	hasProblem(t, folder, Error, "groups/servers.json", "must start with g-")
	if len(folder.Hosts) != 1 || folder.Hosts[0].Name != "plain_name.v2" {
		t.Errorf("hosts: %+v", folder.Hosts)
	}
	if len(folder.Groups) != 1 || folder.Groups[0].Name != "g-ok" || len(folder.Groups[0].Hosts) != 0 {
		t.Errorf("groups: %+v", folder.Groups)
	}
}

// Names differing only in case can only both exist on a case-sensitive
// filesystem; on macOS's default one the second write replaces the first.
func TestNamesDifferingInCase(t *testing.T) {
	root := t.TempDir()
	write(t, root, "hosts/HostA.json", hostJSON([2]string{ageKey(t), "x"}))
	write(t, root, "hosts/hosta.json", hostJSON([2]string{ageKey(t), "x"}))
	write(t, root, "groups/g-Web.json", `{"hosts":["hosta"]}`)
	write(t, root, "groups/G-web.json", `{"hosts":[]}`)
	if entries, _ := os.ReadDir(filepath.Join(root, "hosts")); len(entries) != 2 {
		t.Skip("filesystem is case-insensitive")
	}
	folder := load(t, root)
	hasProblem(t, folder, Error, "hosts/HostA.json", "differ only in case")
	hasProblem(t, folder, Error, "hosts/hosta.json", "differ only in case")
	hasProblem(t, folder, Error, "groups/g-Web.json", "differ only in case")
	hasProblem(t, folder, Error, "groups/G-web.json", "differ only in case")
	if len(folder.Hosts)+len(folder.Groups) != 0 {
		t.Errorf("loaded: %+v", folder)
	}
}

func TestWarnings(t *testing.T) {
	root := t.TempDir()
	shared := ed25519Key(t)
	write(t, root, "hosts/empty.json", `{"keys":[]}`)
	write(t, root, "hosts/one.json", hostJSON([2]string{shared, "a"}))
	write(t, root, "hosts/two.json", hostJSON([2]string{shared + " other-comment", "b"}))
	write(t, root, "hosts/broken.json", `{`)
	write(t, root, "groups/g-all.json", `{"hosts":["one","ONE","ghost","broken","two"]}`)

	folder := load(t, root)
	hasProblem(t, folder, Warning, "hosts/empty.json", "no keys")
	hasProblem(t, folder, Warning, "hosts/one.json", "also listed by one, two")
	hasProblem(t, folder, Warning, "hosts/two.json", "also listed by one, two")
	hasProblem(t, folder, Warning, "groups/g-all.json", `"ONE" is listed twice`)
	hasProblem(t, folder, Warning, "groups/g-all.json", `"ghost" does not exist`)
	hasProblem(t, folder, Warning, "groups/g-all.json", `"broken" is left out`)
	if len(folder.Hosts) != 3 {
		t.Errorf("hosts: %+v", folder.Hosts)
	}
	if len(folder.Groups) != 1 || strings.Join(folder.Groups[0].Hosts, ",") != "one,two" {
		t.Errorf("groups: %+v", folder.Groups)
	}
}

// The example folder is what someone copies, so it has to load cleanly.
func TestExampleFolderLoads(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	folder := load(t, filepath.Join(filepath.Dir(file), "..", "..", "..", "examples", "cred", "recipients"))
	if len(folder.Problems) != 0 {
		t.Errorf("problems: %q", problems(folder))
	}
	if len(folder.Hosts) == 0 || len(folder.Groups) == 0 {
		t.Errorf("example has no hosts or groups: %+v", folder)
	}
}
