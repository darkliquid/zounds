//go:build !windows

package clap

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

func defaultORTLibraryCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"libonnxruntime.dylib", "onnxruntime.dylib"}
	default:
		return []string{"onnxruntime.so", "libonnxruntime.so", "libonnxruntime.so.1"}
	}
}

func openORTLibrary(path string) (uintptr, uintptr, error) {
	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return 0, 0, err
	}
	sym, err := purego.Dlsym(handle, "OrtGetApiBase")
	if err != nil {
		_ = purego.Dlclose(handle)
		return 0, 0, fmt.Errorf("lookup OrtGetApiBase in %q: %w", path, err)
	}
	return handle, sym, nil
}

func closeORTLibrary(handle uintptr) error {
	if handle == 0 {
		return nil
	}
	return purego.Dlclose(handle)
}

func encodeORTModelPath(path string) (unsafe.Pointer, any, error) {
	buf := make([]byte, len(path)+1)
	copy(buf, path)
	return unsafe.Pointer(&buf[0]), buf, nil
}
