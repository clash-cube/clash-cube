package modules

import (
	"reflect"
	"testing"
)

func TestSaveAndCheck(t *testing.T) {
	t.Setenv("MIHOMOBAR_HOME", t.TempDir())
	if len(List()) != 0 {
		t.Fatal("modules before any were saved")
	}
	ms := []Module{{Name: " DNS ", Enabled: true, Body: "dns: {enable: true}\n+rules: []\nsecret: x"}, {Name: "Empty"}}
	if err := Save(ms); err != nil {
		t.Fatal(err)
	}
	got := List()
	if len(got) != 2 || got[0].Name != "DNS" || got[0].ID == "" || got[0].ID == got[1].ID {
		t.Fatalf("got %+v", got)
	}
	if k := got[0].Keys(); !reflect.DeepEqual(k, []string{"+rules", "dns"}) {
		t.Errorf("keys = %v", k)
	}
	for _, bad := range []Module{{Name: "", Body: ""}, {Name: "list", Body: "- a"}, {Name: "yaml", Body: "a: [b"}} {
		if Save([]Module{bad}) == nil {
			t.Errorf("%+v saved", bad)
		}
	}
	if len(List()) != 2 {
		t.Error("a refused save changed the modules")
	}
}
