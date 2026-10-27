package router

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type policyGuard struct {
	path string
	fail bool
}

func (g *policyGuard) Get() (Status, error)    { return Status{Access: AccessPolicy{Mode: "all"}}, nil }
func (g *policyGuard) Pause() (Status, error)  { return g.Get() }
func (g *policyGuard) Resume() (Status, error) { return g.Reconcile() }
func (g *policyGuard) Reconcile() (Status, error) {
	if g.fail {
		g.fail = false
		return Status{}, errors.New("firewall rejected policy")
	}
	p, err := readPreference(g.path)
	return Status{Enabled: p.Enabled, Access: p.Access}, err
}

func TestAccessPolicyValidationAndNormalization(t *testing.T) {
	p, err := (AccessPolicy{Mode: "include", Addresses: []string{"192.168.1.20", "192.168.1.20/32", "10.2.3.4/8"}, MACs: []string{"AA-BB-CC-DD-EE-02", "aa:bb:cc:dd:ee:02"}}).normalized()
	if err != nil || !reflect.DeepEqual(p.Addresses, []string{"192.168.1.20/32", "10.0.0.0/8"}) || len(p.MACs) != 1 {
		t.Fatal("device identity was not normalized", err)
	}
	for _, p := range []AccessPolicy{
		{Mode: "unknown"}, {Addresses: []string{"192.168.1.2 -j ACCEPT"}},
		{Addresses: []string{"::1"}}, {MACs: []string{"ff:ff:ff:ff:ff:ff"}}, {MACs: []string{"02:00:00:00:00:01;reboot"}},
	} {
		if _, err := p.normalized(); err == nil {
			t.Fatal("accepted invalid access policy")
		}
	}
}

func TestAccessEditsRollbackAndSurviveEnableChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.json")
	// Read the P0 file without a migration or silently changing enabled intent.
	if err := os.WriteFile(path, []byte(`{"enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	g := &policyGuard{path: path}
	m := &Manager{path: path, guard: g}
	p := AccessPolicy{Mode: "exclude", Addresses: []string{"192.168.1.10"}}
	if _, err := m.SetAccess(p); err != nil {
		t.Fatal(err)
	}
	if err := m.save(false); err != nil {
		t.Fatal(err)
	}
	before, err := readPreference(path)
	if err != nil || before.Enabled || before.Access.Mode != "exclude" {
		t.Fatal("toggle erased device policy", err)
	}
	g.fail = true
	if _, err := m.SetAccess(AccessPolicy{Mode: "include"}); err == nil {
		t.Fatal("accepted rejected firewall transaction")
	}
	after, _ := readPreference(path)
	if !reflect.DeepEqual(after, before) {
		t.Fatal("failed policy edit changed persisted policy")
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("access policy permissions")
	}
	var stored map[string]any
	body, _ := os.ReadFile(path)
	if json.Unmarshal(body, &stored) != nil || stored["access"] == nil {
		t.Fatal("policy was not persisted")
	}
}
