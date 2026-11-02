//go:build windows

package pacaroute

import (
	"bytes"
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

var dpapiPrefix = []byte("PacaRoute-DPAPI-v1\x00")

func platformSeal(b []byte) ([]byte, bool, error) {
	if len(b) == 0 {
		return nil, true, errors.New("empty credential")
	}
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	if e := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); e != nil {
		return nil, true, e
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append(bytes.Clone(dpapiPrefix), unsafe.Slice(out.Data, out.Size)...), true, nil
}
func platformOpen(b []byte) ([]byte, bool, error) {
	if !bytes.HasPrefix(b, dpapiPrefix) {
		return nil, false, nil
	}
	b = b[len(dpapiPrefix):]
	if len(b) == 0 {
		return nil, true, errors.New("invalid protected credential")
	}
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	if e := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); e != nil {
		return nil, true, errors.New("Windows could not unlock this credential for the current user")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, out.Size)), true, nil
}
