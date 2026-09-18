package secretstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

func testInventory() (*inventory.Root, map[string]confgen.Manifest) {
	inv := &inventory.Root{
		Nodes: []inventory.Node{
			{
				ID:       "srv",
				Networks: inventory.Networks{"internet": "203.0.113.10"},
				Instances: []inventory.Instance{
					{ID: "ss-srv", Service: "shadowsocks-rust", Role: "server", Ports: map[string]int{"main": 38250}},
				},
			},
			{ID: "laptop", Owner: "doug"},
		},
		Users: map[string]inventory.User{
			"doug": {Access: []string{"sfo"}},
		},
		Routes:    map[string]inventory.Route{"sfo": {Hops: []string{"ss-srv:main"}}},
		Networks:  []string{"internet"},
		Universal: "internet",
	}
	manifests := map[string]confgen.Manifest{
		"shadowsocks-rust": {
			Secret: confgen.Secret{Kind: "base64", Bytes: 32},
			Roles: map[string]confgen.Role{
				"server":  {Auth: confgen.AuthPerPrincipal, ReachedBy: "ss-rust"},
				"ss-rust": {Auth: confgen.AuthNone},
			},
		},
	}
	return inv, manifests
}

func mustDerive(t *testing.T, inv *inventory.Root, manifests map[string]confgen.Manifest) *derive.Model {
	t.Helper()
	m, err := derive.Derive(inv, manifests)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return m
}

func TestImpliedPaths_OnePerPrincipalGrant(t *testing.T) {
	inv, manifests := testInventory()
	model := mustDerive(t, inv, manifests)

	paths := ImpliedPaths(inv, manifests, model)
	if len(paths) != 1 {
		t.Fatalf("ImpliedPaths = %+v, want 1", paths)
	}
	p := paths[0]
	if p.Instance != "ss-srv" || p.Port != "main" || p.Kind != "node" || p.Name != "laptop" {
		t.Fatalf("path = %+v, want ss-srv/main/node/laptop", p)
	}
	if p.String() != filepath.Join("ss-srv", "main", "node", "laptop") {
		t.Fatalf("String() = %q", p.String())
	}
}

func TestSync_MissingAndGenerate(t *testing.T) {
	inv, manifests := testInventory()
	model := mustDerive(t, inv, manifests)
	implied := ImpliedPaths(inv, manifests, model)

	root := t.TempDir()
	res, err := Sync(root, implied)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res.Missing) != 1 || len(res.Orphaned) != 0 {
		t.Fatalf("Sync = %+v, want one missing, none orphaned", res)
	}

	if err := Generate(root, res.Missing, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	full := filepath.Join(root, "ss-srv", "main", "node", "laptop")
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("reading generated secret: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("generated secret is empty")
	}

	// Generate refuses to overwrite.
	if err := Generate(root, res.Missing, inv, manifests); err == nil {
		t.Fatal("Generate: want an error the second time, the file already exists")
	}

	// A second Sync now finds nothing missing and nothing orphaned.
	res2, err := Sync(root, implied)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res2.Missing) != 0 || len(res2.Orphaned) != 0 {
		t.Fatalf("Sync = %+v, want none missing, none orphaned", res2)
	}
}

// TestSync_RenameProducesTheHint is the case that matters most: renaming a
// node produces one new implied path and leaves one orphaned file, and sync
// must say so rather than silently generating a new random value on one
// side of what was really one credential.
func TestSync_RenameProducesTheHint(t *testing.T) {
	inv, manifests := testInventory()
	model := mustDerive(t, inv, manifests)
	implied := ImpliedPaths(inv, manifests, model)

	root := t.TempDir()
	if err := Generate(root, implied, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	oldValue, err := os.ReadFile(filepath.Join(root, "ss-srv", "main", "node", "laptop"))
	if err != nil {
		t.Fatal(err)
	}

	// Rename the node in the inventory: laptop -> macbook. The old secret
	// file is still on disk under the old name.
	inv.Nodes[1].ID = "macbook"
	model2 := mustDerive(t, inv, manifests)
	implied2 := ImpliedPaths(inv, manifests, model2)

	res, err := Sync(root, implied2)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res.Missing) != 1 || len(res.Orphaned) != 1 {
		t.Fatalf("Sync = %+v, want exactly one missing and one orphaned", res)
	}
	if !res.RenameHint {
		t.Fatal("RenameHint = false, want true when counts match")
	}
	if res.Missing[0].Name != "macbook" || res.Orphaned[0].Name != "laptop" {
		t.Fatalf("Sync = %+v, want missing macbook and orphaned laptop", res)
	}

	// mv performs exactly that move, and a value carries over untouched.
	if err := Mv(root, res.Orphaned[0].String(), res.Missing[0].String()); err != nil {
		t.Fatalf("Mv: %v", err)
	}
	res3, err := Sync(root, implied2)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res3.Missing) != 0 || len(res3.Orphaned) != 0 {
		t.Fatalf("Sync after Mv = %+v, want none missing, none orphaned", res3)
	}
	newValue, err := os.ReadFile(filepath.Join(root, "ss-srv", "main", "node", "macbook"))
	if err != nil {
		t.Fatal(err)
	}
	if string(newValue) != string(oldValue) {
		t.Fatal("Mv changed the secret's value")
	}
}

// TestImpliedPaths_CombineOwn covers a role declaring combine_own: the
// instance's own secret of that name is implied once, regardless of how
// many ports or grants the instance has.
func TestImpliedPaths_CombineOwn(t *testing.T) {
	inv, manifests := testInventory()
	role := manifests["shadowsocks-rust"].Roles["server"]
	role.CombineOwn = "psk"
	manifests["shadowsocks-rust"].Roles["server"] = role
	model := mustDerive(t, inv, manifests)

	paths := ImpliedPaths(inv, manifests, model)
	var own *Path
	for i := range paths {
		if paths[i].Port == OwnPort {
			own = &paths[i]
		}
	}
	if own == nil {
		t.Fatalf("ImpliedPaths = %+v, want an own/psk path", paths)
	}
	if own.Instance != "ss-srv" || own.Name != "psk" || own.Kind != "" {
		t.Fatalf("own path = %+v, want ss-srv/own/psk", own)
	}
	if own.String() != filepath.Join("ss-srv", "own", "psk") {
		t.Fatalf("String() = %q", own.String())
	}
}

// TestSync_CombineOwnGeneratedAndNotOrphaned pins the two directions
// combine_own must get right: Generate can create the file Sync reports
// missing, and once it exists Sync does not report it orphaned even though
// own/ paths are skipped in general.
func TestSync_CombineOwnGeneratedAndNotOrphaned(t *testing.T) {
	inv, manifests := testInventory()
	role := manifests["shadowsocks-rust"].Roles["server"]
	role.CombineOwn = "psk"
	manifests["shadowsocks-rust"].Roles["server"] = role
	model := mustDerive(t, inv, manifests)
	implied := ImpliedPaths(inv, manifests, model)

	root := t.TempDir()
	res, err := Sync(root, implied)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	foundMissing := false
	for _, p := range res.Missing {
		if p.Port == OwnPort {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("Sync.Missing = %+v, want ss-srv/own/psk among them", res.Missing)
	}

	if err := Generate(root, res.Missing, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	res2, err := Sync(root, implied)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res2.Missing) != 0 || len(res2.Orphaned) != 0 {
		t.Fatalf("Sync after Generate = %+v, want none missing, none orphaned", res2)
	}
}

// TestSync_CombineOwnNotOrphanedOnceNoLongerImplied documents a real limit:
// dropping combine_own from a role leaves its own/ file on disk with no
// implied path pointing at it, but Sync cannot tell that apart from an
// ordinary own/ file it never computed an identity for in the first place —
// both are just "an own/ path nothing currently implies". It is left alone
// like any other, rather than guessed at.
func TestSync_CombineOwnNotOrphanedOnceNoLongerImplied(t *testing.T) {
	inv, manifests := testInventory()
	role := manifests["shadowsocks-rust"].Roles["server"]
	role.CombineOwn = "psk"
	manifests["shadowsocks-rust"].Roles["server"] = role
	model := mustDerive(t, inv, manifests)
	implied := ImpliedPaths(inv, manifests, model)

	root := t.TempDir()
	if err := Generate(root, implied, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	role.CombineOwn = ""
	manifests["shadowsocks-rust"].Roles["server"] = role
	model2 := mustDerive(t, inv, manifests)
	implied2 := ImpliedPaths(inv, manifests, model2)

	res, err := Sync(root, implied2)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res.Orphaned) != 0 {
		t.Fatalf("Sync.Orphaned = %+v, want none — own/ paths are not tracked historically", res.Orphaned)
	}
}

func TestSync_OwnPathsAreNeverOrphaned(t *testing.T) {
	inv, manifests := testInventory()
	model := mustDerive(t, inv, manifests)
	implied := ImpliedPaths(inv, manifests, model)

	root := t.TempDir()
	if err := Generate(root, implied, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	ownPath := filepath.Join(root, "ss-srv", OwnPort, "instance", "auth_password")
	if err := os.MkdirAll(filepath.Dir(ownPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ownPath, []byte("hunter2"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Sync(root, implied)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res.Orphaned) != 0 {
		t.Fatalf("Sync = %+v, want the own/ file never reported orphaned", res)
	}
}

func TestSync_PreviousFilesAreNeverOrphaned(t *testing.T) {
	inv, manifests := testInventory()
	model := mustDerive(t, inv, manifests)
	implied := ImpliedPaths(inv, manifests, model)

	root := t.TempDir()
	if err := Generate(root, implied, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	prev := filepath.Join(root, "ss-srv", "main", "node", "laptop"+PreviousSuffix)
	if err := os.WriteFile(prev, []byte("old-value"), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := Sync(root, implied)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(res.Orphaned) != 0 {
		t.Fatalf("Sync = %+v, want a .previous file never reported orphaned", res)
	}
}

func TestReadPrevious(t *testing.T) {
	inv, manifests := testInventory()
	model := mustDerive(t, inv, manifests)
	implied := ImpliedPaths(inv, manifests, model)
	p := implied[0]

	root := t.TempDir()
	if err := Generate(root, implied, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, ok, err := ReadPrevious(root, p); err != nil || ok {
		t.Fatalf("ReadPrevious with no .previous file = %v, %v, want false, nil", ok, err)
	}

	if err := os.WriteFile(filepath.Join(root, p.String())+PreviousSuffix, []byte("old-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, ok, err := ReadPrevious(root, p)
	if err != nil {
		t.Fatalf("ReadPrevious: %v", err)
	}
	if !ok || value != "old-value" {
		t.Fatalf("ReadPrevious = %q, %v, want %q, true", value, ok, "old-value")
	}
}

func TestPreviousModTimes(t *testing.T) {
	inv, manifests := testInventory()
	model := mustDerive(t, inv, manifests)
	implied := ImpliedPaths(inv, manifests, model)
	p := implied[0]

	root := t.TempDir()
	if err := Generate(root, implied, inv, manifests); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	previousPath := filepath.Join(root, p.String()) + PreviousSuffix
	if err := os.WriteFile(previousPath, []byte("old-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-9 * 24 * time.Hour)
	if err := os.Chtimes(previousPath, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	got, err := PreviousModTimes(root)
	if err != nil {
		t.Fatalf("PreviousModTimes: %v", err)
	}
	mtime, ok := got[p.String()]
	if !ok {
		t.Fatalf("PreviousModTimes = %v, want an entry for %s", got, p)
	}
	if !mtime.Equal(stamp) {
		t.Fatalf("PreviousModTimes[%s] = %v, want %v", p, mtime, stamp)
	}
}
