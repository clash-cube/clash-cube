// Package updatesig signs builds so that the root helper can replace itself
// with a newer build without an administrator's password. The app is only
// signed ad hoc, so the helper can't ask macOS who built a binary; it checks
// an ed25519 signature made by scripts/signhelper instead.
//
// codesign rewrites the executable when the app is bundled, so the signature
// covers a canonical hash that leaves out what codesign changes: the code
// signature itself and the two load command fields that record its size.
package updatesig

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"debug/macho"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
)

// PublicKey verifies update signatures. The private half stays with
// whoever builds releases.
const PublicKey = "QIILcDwwy55u3ArB49zjrnb6DQQDZ6kzdLjN5A2L4yg="

// Stamp is this build's time in unix seconds, set by the build like the
// version; an update must carry a later one.
var Stamp = "0"

// OwnStamp is Stamp as a number.
func OwnStamp() int64 {
	n, _ := strconv.ParseInt(Stamp, 10, 64)
	return n
}

// Key is PublicKey, or nil if it is unset (no build is accepted).
func Key() ed25519.PublicKey {
	k, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil || len(k) != ed25519.PublicKeySize {
		return nil
	}
	return k
}

// SigPath is where a bundled executable's signature lives:
// Contents/Resources/helper.sig next to Contents/MacOS/<exe>.
func SigPath(exe string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(exe)), "Resources", "helper.sig")
}

type sigFile struct {
	Stamp int64  `json:"stamp"` // build time, against downgrades
	Sig   []byte `json:"sig"`
}

func message(hash []byte, stamp int64) []byte {
	return []byte("clashcube-helper-update\n" + fmt.Sprintf("%x", hash) + "\n" + strconv.FormatInt(stamp, 10))
}

// Sign signs the executable exe, built at stamp.
func Sign(key ed25519.PrivateKey, exe []byte, stamp int64) ([]byte, error) {
	h, err := CodeHash(exe)
	if err != nil {
		return nil, err
	}
	return json.Marshal(sigFile{Stamp: stamp, Sig: ed25519.Sign(key, message(h, stamp))})
}

// Verify checks sig against exe and returns the build stamp it carries.
func Verify(pub ed25519.PublicKey, exe, sig []byte) (int64, error) {
	if len(pub) != ed25519.PublicKeySize {
		return 0, errors.New("no update key")
	}
	var s sigFile
	if err := json.Unmarshal(sig, &s); err != nil {
		return 0, errors.New("bad signature file")
	}
	h, err := CodeHash(exe)
	if err != nil {
		return 0, err
	}
	if !ed25519.Verify(pub, message(h, s.Stamp), s.Sig) {
		return 0, errors.New("bad signature")
	}
	return s.Stamp, nil
}

// CodeHash hashes a Mach-O executable, thin or universal, leaving out its
// code signature.
func CodeHash(b []byte) ([]byte, error) {
	if len(b) >= 4 && binary.BigEndian.Uint32(b) == macho.MagicFat {
		f, err := macho.NewFatFile(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		for _, a := range f.Arches {
			end := uint64(a.Offset) + uint64(a.Size)
			if end > uint64(len(b)) {
				return nil, errors.New("truncated universal binary")
			}
			sh, err := sliceHash(b[a.Offset:end])
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(h, "%d/%d:%x\n", a.Cpu, a.SubCpu, sh)
		}
		return h.Sum(nil), nil
	}
	return sliceHash(b)
}

const (
	lcSegment64     = 0x19
	lcCodeSignature = 0x1d
)

// sliceHash hashes one 64-bit Mach-O up to its code signature, with the
// signature's size and __LINKEDIT's sizes zeroed. The signature must end
// __LINKEDIT, so nothing that is mapped escapes the hash.
func sliceHash(b []byte) ([]byte, error) {
	f, err := macho.NewFile(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if f.Magic != macho.Magic64 {
		return nil, errors.New("not a 64-bit executable")
	}
	bo := f.ByteOrder
	off := uint64(32) // mach_header_64
	sigCmd, linkCmd := uint64(0), uint64(0)
	var sigOff, sigEnd, linkEnd uint64
	for _, l := range f.Loads {
		raw := l.Raw()
		switch bo.Uint32(raw) {
		case lcCodeSignature:
			if len(raw) < 16 {
				return nil, errors.New("bad code signature command")
			}
			sigCmd = off
			sigOff = uint64(bo.Uint32(raw[8:]))
			sigEnd = sigOff + uint64(bo.Uint32(raw[12:]))
		case lcSegment64:
			if len(raw) >= 56 && string(bytes.TrimRight(raw[8:24], "\x00")) == "__LINKEDIT" {
				linkCmd = off
				linkEnd = bo.Uint64(raw[40:]) + bo.Uint64(raw[48:])
			}
		}
		off += uint64(len(raw))
	}
	if sigCmd == 0 || linkCmd == 0 {
		return nil, errors.New("no code signature")
	}
	if sigEnd != linkEnd || sigEnd > uint64(len(b)) || sigOff < off {
		return nil, errors.New("code signature is not at the end")
	}
	c := bytes.Clone(b[:sigOff])
	clear(c[sigCmd+12 : sigCmd+16])   // datasize
	clear(c[linkCmd+32 : linkCmd+40]) // vmsize
	clear(c[linkCmd+48 : linkCmd+56]) // filesize
	h := sha256.Sum256(c)
	return h[:], nil
}
