package modules

import "testing"

func TestTailscaleScopeAndYAML(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	config := &Tailscale{Identity: "identity-001", Hostname: "clashcube", Routes: []string{"192.168.50.0/24", "fd7a:115c:a1e0::/48"}}
	m := Module{Name: "Private network", Enabled: true, Profile: "one", Tailscale: config}
	body, err := config.Generate("one", nil)
	if err != nil {
		t.Fatal(err)
	}
	// A converted YAML module has the same identity and scope restrictions.
	yaml := Module{Name: "Converted", Enabled: true, Profile: "one", Body: body}
	if err := Save([]Module{m, yaml}); err == nil {
		t.Fatal("accepted overlapping tailnets after conversion")
	}
	yaml.Profile = "two"
	if err := Save([]Module{m, yaml}); err != nil {
		t.Fatal(err)
	}
	if err := CopyProfile("one", "copy"); err != nil {
		t.Fatal(err)
	}
	for _, mod := range List() {
		if mod.Profile == "copy" && mod.Tailscale.Identity != config.Identity {
			t.Fatal("profile copy replaced the persistent identity")
		}
	}
	m.Profile = ""
	if err := Save([]Module{m, yaml}); err == nil {
		t.Fatal("accepted global and profile-specific tailnets")
	}
}

func TestTailscaleRejectsInvalidDestinations(t *testing.T) {
	for _, route := range []string{"0.0.0.0/0", "::/0", "192.168.50.10/24", "127.0.0.0/8", "::ffff:192.168.50.0/120", "224.0.0.0/4", "garbage"} {
		if err := (&Tailscale{Identity: "identity-001", Hostname: "clashcube", Routes: []string{route}}).Check(); err == nil {
			t.Errorf("accepted %q", route)
		}
	}
	for _, url := range []string{"http://headscale.example", "https://user:pass@headscale.example", "https://headscale.example/?key=secret"} {
		if err := (&Tailscale{Identity: "identity-001", Hostname: "clashcube", ControlURL: url}).Check(); err == nil {
			t.Errorf("accepted %q", url)
		}
	}
}
