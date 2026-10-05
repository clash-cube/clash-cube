package updatesig

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Sign a copy as bundling does: Intel test binaries may be unsigned.
func testExe(t *testing.T) []byte {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "clashcube")
	if err := os.WriteFile(p, b, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("codesign", "--force", "--sign", "-", p).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v: %s", err, out)
	}
	b, err = os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSignatureSurvivesCodesign(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	exe := testExe(t)
	sig, err := Sign(priv, exe, 42)
	if err != nil {
		t.Fatal(err)
	}
	if st, err := Verify(pub, exe, sig); err != nil || st != 42 {
		t.Fatalf("verify: %d %v", st, err)
	}
	// bundling re-signs the executable, which changes its bytes
	p := filepath.Join(t.TempDir(), "clashcube")
	os.WriteFile(p, exe, 0o755)
	if out, err := exec.Command("codesign", "--force", "--sign", "-", "--identifier", "x.y.z", p).CombinedOutput(); err != nil {
		t.Skipf("codesign: %s", out)
	}
	signed, _ := os.ReadFile(p)
	if string(signed) == string(exe) {
		t.Fatal("codesign changed nothing")
	}
	if _, err := Verify(pub, signed, sig); err != nil {
		t.Fatalf("re-signed build refused: %v", err)
	}
}

func TestVerifyRefuses(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	exe := testExe(t)
	sig, err := Sign(priv, exe, 42)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Verify(other, exe, sig); err == nil {
		t.Error("accepted another key's signature")
	}
	if _, err := Verify(nil, exe, sig); err == nil {
		t.Error("accepted without a key")
	}
	changed := append([]byte(nil), exe...)
	changed[len(changed)/3] ^= 1
	if _, err := Verify(pub, changed, sig); err == nil {
		t.Error("accepted a changed executable")
	}
	forged := []byte(`{"stamp":43,"sig":"` + string(sig[len(`{"stamp":42,"sig":"`):]))
	if _, err := Verify(pub, exe, forged); err == nil {
		t.Error("accepted a changed stamp")
	}
	if _, err := Verify(pub, []byte("#!/bin/sh\n"), sig); err == nil {
		t.Error("accepted a script")
	}
}

func TestBuiltInKey(t *testing.T) {
	if Key() == nil {
		t.Fatal("PublicKey does not decode")
	}
}
