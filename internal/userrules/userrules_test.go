package userrules

import "testing"

func TestCheck(t *testing.T) {
	ok := Rule{"DOMAIN-SUFFIX", "example.com", "Proxy"}
	if err := ok.Check(); err != nil {
		t.Error(err)
	}
	for _, r := range []Rule{
		{"MATCH", "x", "DIRECT"},
		{"DOMAIN", "a.com,no-resolve", "DIRECT"},
		{"DOMAIN", "a.com", "DIRECT\n- MATCH,REJECT"},
		{"DOMAIN", " ", "DIRECT"},
	} {
		if r.Check() == nil {
			t.Errorf("%q accepted", r.String())
		}
	}
}

func TestSaveAndAdded(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	if len(List()) != 0 {
		t.Fatal("rules before any were saved")
	}
	rs := Added(nil, Rule{"DOMAIN", "a.com", "DIRECT"})
	rs = Added(rs, Rule{"DOMAIN", "b.com", "DIRECT"})
	rs = Added(rs, Rule{"DOMAIN", "a.com", "Proxy"})
	if err := Save(rs); err != nil {
		t.Fatal(err)
	}
	got := List()
	if len(got) != 2 || got[0] != (Rule{"DOMAIN", "a.com", "Proxy"}) || got[1].Payload != "b.com" {
		t.Errorf("got %v", got)
	}
	if Save([]Rule{{"DOMAIN", "x,y", "DIRECT"}}) == nil {
		t.Error("a bad rule saved")
	}
}
