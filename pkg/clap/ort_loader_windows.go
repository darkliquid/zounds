//go:build windows

package clap

import (
	"fmt"
	"syscall"
	"unsafe"
)

func defaultORTLibraryCandidates() []string {
	return []string{"onnxruntime.dll"}
}

func openORTLibrary(path string) (uintptr, uintptr, error) {
	handle, err := syscall.LoadLibrary(path)
	if err != nil {
		return 0, 0, err
	}
	sym, err := syscall.GetProcAddress(handle, "OrtGetApiBase")
	if err != nil {
		_ = syscall.FreeLibrary(handle)
		return 0, 0, fmt.Errorf("lookup OrtGetApiBase in %q: %w", path, err)
	}
	return uintptr(handle), sym, nil
}

func closeORTLibrary(handle uintptr) error {
	if handle == 0 {
		return nil
	}
	return syscall.FreeLibrary(syscall.Handle(handle))
}

func encodeORTModelPath(path string) (unsafe.Pointer, any, error) {
	utf16Path, err := syscall.UTF16FromString(path)
	if err != nil {
		return nil, nil, fmt.Errorf("encode Windows UTF-16 model path %q: %w", path, err)
	}
	return unsafe.Pointer(&utf16Path[0]), utf16Path, nil
}
