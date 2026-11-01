# Phase 0: Immediate Stabilization & Data Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore a 100% green test, linter, and `CGO_ENABLED=0` cross-platform build baseline while eliminating silent file-overwrite, audio-corruption, goroutine-leak, and batch-abort hazards across `zounds`.

**Architecture:** Keep reusable logic in `pkg/*` per the `AGENTS.md` strict layering invariant: migrate `pkg/clap`'s ONNX Runtime C API bridge from CGO (`yalue/onnxruntime_go`) to `ebitengine/purego`; add `facet:value` tag normalization in `pkg/core` paired with path sanitization and batch collision preflight in `pkg/rename`; harden `PCMBuffer.Validate()`, Ogg Vorbis float clamping, 8-bit signed/unsigned PCM scaling, and 32/64-bit IEEE Float WAV decoding in `pkg/audio`; cancel worker contexts on early errors in `pkg/scanner` and `pkg/dedup`; and introduce typed `audio.DecodeError` handling so batch CLI runs skip undecodable samples (e.g. `.aac`/`.m4a` or corrupt headers) with warnings.

**Tech Stack:** Go `1.26`, `github.com/ebitengine/purego` (`v0.10.0`), ONNX Runtime C API (`OrtGetApiBase`), `github.com/spf13/cobra`, `modernc.org/sqlite`, `golangci-lint` v2.

---

## File Structure

### Modified Files
- `cmd/zounds/commands/tag_test.go`: Update `TestTagCommandAutoAppliesRuleTags` to pass an explicit `--rule-file` fixture and remove stale default-rule comment.
- `cmd/zounds/commands/integration_test.go`: Pass an explicit `--rule-file` fixture in `TestEndToEndWorkflowScanAnalyzeTagAndCluster`.
- `pkg/clap/model.go`: Replace deprecated `reflect.Ptr` with `reflect.Pointer` (`govet` fix).
- `pkg/clap/onnx.go`: Replace `github.com/yalue/onnxruntime_go` with pure-Go ONNX Runtime C API session/tensor wrappers backed by `github.com/ebitengine/purego` and retryable `ensureORT` initialization.
- `pkg/clap/onnx_test.go`: Add unit tests for failed-init recovery, tensor/session lifecycle, and optional end-to-end inference when local `./onnxruntime.so` and `./onnx/` assets exist.
- `go.mod` / `go.sum`: Promote `github.com/ebitengine/purego` to a direct dependency and drop `github.com/yalue/onnxruntime_go`.
- `pkg/core/types.go`: Update `NormalizeTagName` to preserve `:` for `facet:value` namespaced tags while trimming surrounding whitespace.
- `pkg/rename/template.go`: Harden `slug()` against `:`, `/`, `\`, `..`, Windows-invalid filename characters, and control characters; add `ResolveTarget`, `RenameOp`, and `ValidatePlan` batch collision/overwrite/traversal preflight.
- `pkg/rename/template_test.go`: Add unit tests for `slug()` sanitization, dot-in-stem extension preservation (`kick.v2.wav`), path traversal rejection, duplicate-target preflight, and existing-file overwrite rejection.
- `cmd/zounds/commands/rename.go`: Delegate target resolution and full-batch preflight validation to `pkg/rename` before performing any `os.Rename`.
- `cmd/zounds/commands/rename_test.go`: Add CLI tests verifying dot-in-stem preservation, batch duplicate-target rejection, and existing-destination overwrite protection.
- `pkg/audio/buffer.go`: Reject `math.NaN()` and `math.IsInf()` in `PCMBuffer.Validate()`.
- `pkg/audio/buffer_test.go`: Add `NaN` and `±Inf` validation unit tests.
- `pkg/audio/ogg/codec.go`: Clamp decoded Ogg Vorbis float samples to `[-1.0, 1.0]` (and reject `NaN`).
- `pkg/audio/pcm.go`: Support unsigned 8-bit RIFF WAVE offset-binary (`[0..255]`, `128` = silence) helpers alongside signed two's-complement PCM helpers.
- `pkg/audio/aiff/codec.go`: Sign-extend 8-bit two's-complement samples returned as raw bytes by `go-audio/aiff` before scaling.
- `pkg/audio/aiff/codec_test.go`: Add 8-bit signed AIFF round-trip and silence/polarity accuracy tests.
- `pkg/audio/wav/codec.go`: Support unsigned 8-bit PCM scaling and native 32-bit/64-bit IEEE Float WAV decoding (`Format=3` and `WAVE_FORMAT_EXTENSIBLE` `0xFFFE`).
- `pkg/audio/wav/codec_test.go`: Add tests for 8-bit unsigned WAV round-trip, 32-bit IEEE Float WAV (`Format=3`), 64-bit IEEE Float WAV (`Format=3`), and `WAVE_FORMAT_EXTENSIBLE` (`0xFFFE`) IEEE Float WAV.
- `pkg/scanner/scanner.go`: Wrap `Scan` in `context.WithCancel(ctx)` and wait for workers/walker teardown so early errors never leak goroutines.
- `pkg/scanner/scanner_test.go`: Add early-error goroutine-leak regression test.
- `pkg/dedup/exact.go`: Wrap `ExactFinder.Find` in `context.WithCancel(ctx)` and drain worker shutdown so early errors never leak goroutines.
- `pkg/dedup/exact_test.go`: Add early-error goroutine-leak regression test.
- `pkg/audio/decoder.go`: Add `ErrUnsupportedFormat`, `DecodeError`, and `IsDecodeError` so callers can distinguish undecodable/unsupported sample files from context cancellation or DB errors.
- `cmd/zounds/commands/analyze.go`: In `analyze --all`, log a warning and skip samples failing with `audio.IsDecodeError(err)` while keeping `analyze --id` and `info <file>` strict.
- `cmd/zounds/commands/analyze_test.go`: Test `analyze --all` skipping `.m4a` and corrupt `.wav` files with warnings while analyzing valid samples.
- `cmd/zounds/commands/tag.go`: Pre-decode each sample once via `analysis.ContextWithDecodedSample` in `tag --auto`; in batch runs (`--path` omitted), warn and skip samples failing with `audio.IsDecodeError(err)`.
- `cmd/zounds/commands/dedup.go`: In `dedup --perceptual`, warn and skip samples failing with `audio.IsDecodeError(err)`.
- `cmd/zounds/commands/dedup_test.go`: Test `dedup --perceptual` skipping `.m4a` files with a warning while finding perceptual duplicates among decodable files.

### Created Files
- `pkg/clap/ort_loader_unix.go`: Unix (`!windows`) dynamic library loader (`purego.Dlopen` / `purego.Dlsym` / `purego.Dlclose`) and UTF-8 `ORTCHAR_T` model-path encoder.
- `pkg/clap/ort_loader_windows.go`: Windows (`windows`) dynamic library loader (`syscall.LoadLibrary` / `syscall.GetProcAddress` / `syscall.FreeLibrary`) and UTF-16 `ORTCHAR_T` model-path encoder.
- `pkg/core/types_test.go`: Unit tests for `NormalizeTagName` (including `facet:value` namespacing) and `DetectFormatFromExtension`.
- `pkg/audio/ogg/codec_test.go`: Unit tests verifying Ogg Vorbis decode clamps reconstructed peaks `> 1.0` so `PCMBuffer.Validate()` succeeds.
- `pkg/audio/decoder_test.go`: Unit tests for `DecodeError`, `ErrUnsupportedFormat`, and `IsDecodeError` (ensuring `context.Canceled` is never misclassified as undecodable).

---

### Task 1: Fix Failing `HEAD` CLI Tests & `govet` Deprecation

**Files:**
- Modify: `cmd/zounds/commands/tag_test.go:103-140`
- Modify: `cmd/zounds/commands/integration_test.go:13-30`
- Modify: `pkg/clap/model.go:62-67`

- [ ] **Step 1: Run existing failing CLI tests and `golangci-lint` to confirm baseline failures**

Run:
```bash
go test ./cmd/zounds/commands -run 'TestTagCommandAutoAppliesRuleTags|TestEndToEndWorkflowScanAnalyzeTagAndCluster' -v
golangci-lint run ./pkg/clap/...
```
Expected:
- `TestTagCommandAutoAppliesRuleTags` FAILS at `tag_test.go:138: missing rule tag "sub" in map[]`.
- `TestEndToEndWorkflowScanAnalyzeTagAndCluster` FAILS at `integration_test.go:53: expected tags for sample 1`.
- `golangci-lint` reports `pkg/clap/model.go:64:18: inline: Constant reflect.Ptr should be inlined (govet)`.

- [ ] **Step 2: Update `tag_test.go` and `integration_test.go` to pass explicit `--rule-file` fixtures**

In `cmd/zounds/commands/tag_test.go`, update `TestTagCommandAutoAppliesRuleTags` (lines 103–140) and line 194's assertion message so `RuleTagger` is tested via explicit `--rule-file` with namespaced tags:

```go
func TestTagCommandAutoAppliesRuleTags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rules.db")
	rulePath := filepath.Join(t.TempDir(), "sub-rules.json")
	samplePath := filepath.Join(root, "Synth", "tone.wav")
	writeSineWAVFixture(t, samplePath, 55, 1.0)

	err := os.WriteFile(rulePath, []byte(`{
  "rules": [
    {
      "tag": "sub",
      "expr": "Metrics[\"frequency_hz\"] >= 20 && Metrics[\"frequency_hz\"] < 80 && Metrics[\"confidence\"] > 0.5",
      "confidence": 0.9
    }
  ]
}`), 0o644)
	if err != nil {
		t.Fatalf("write rule file: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tagCmd := commands.NewRootCommand()
	tagCmd.SetArgs([]string{"--db", dbPath, "tag", "--auto", "--rule-file", rulePath})
	if err := tagCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute auto tag: %v", err)
	}

	repo := openRepoForTest(t, dbPath)
	sample, err := repo.FindSampleByPath(context.Background(), samplePath)
	if err != nil {
		t.Fatalf("find sample: %v", err)
	}
	got, err := repo.ListTagsForSample(context.Background(), sample.ID)
	if err != nil {
		t.Fatalf("list sample tags: %v", err)
	}

	names := make(map[string]struct{}, len(got))
	for _, tag := range got {
		names[tag.NormalizedName] = struct{}{}
	}
	if _, ok := names["sub"]; !ok {
		t.Fatalf("missing rule tag %q in %v", "sub", names)
	}
}
```

And in `TestTagCommandAutoUsesCustomRuleFile` (`cmd/zounds/commands/tag_test.go:193-195`), update the stale failure message:

```go
	if _, ok := names["sub"]; ok {
		t.Fatalf("unexpected unconfigured rule tag in %v", names)
	}
```

In `cmd/zounds/commands/integration_test.go`, add `"os"` to imports and write an explicit `--rule-file` in `TestEndToEndWorkflowScanAnalyzeTagAndCluster`:

```go
package commands_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkliquid/zounds/cmd/zounds/commands"
)

func TestEndToEndWorkflowScanAnalyzeTagAndCluster(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "workflow.db")
	rulePath := filepath.Join(t.TempDir(), "workflow-rules.json")
	writeSineWAVFixture(t, filepath.Join(root, "Bass", "sub.wav"), 55, 1.0)
	writeSineWAVFixture(t, filepath.Join(root, "Leads", "lead.wav"), 880, 1.0)

	if err := os.WriteFile(rulePath, []byte(`{
  "rules": [
    {
      "tag": "sub",
      "expr": "Metrics[\"frequency_hz\"] < 120 && Metrics[\"confidence\"] > 0.5",
      "confidence": 0.9
    },
    {
      "tag": "lead",
      "expr": "Metrics[\"frequency_hz\"] >= 400 && Metrics[\"confidence\"] > 0.5",
      "confidence": 0.9
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write workflow rules: %v", err)
	}

	runCommand(t, dbPath, "scan", root)
	runCommand(t, dbPath, "analyze", "--all")
	runCommand(t, dbPath, "tag", "--auto", "--rule-file", rulePath)
```

- [ ] **Step 3: Fix `reflect.Pointer` deprecation in `pkg/clap/model.go`**

In `pkg/clap/model.go:62-66`, replace `reflect.Ptr` with `reflect.Pointer`:

```go
func applyZeroValueDefaults(dst any, defaults any) {
	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Pointer || dv.IsNil() {
		return
	}
```

- [ ] **Step 4: Run CLI tests and `golangci-lint` to verify fixes**

Run:
```bash
go test ./cmd/zounds/commands/... -v
golangci-lint run
```
Expected:
- All tests in `./cmd/zounds/commands/...` PASS.
- `golangci-lint run` exits with `0 issues`.

- [ ] **Step 5: Commit Task 1**

```bash
git add cmd/zounds/commands/tag_test.go cmd/zounds/commands/integration_test.go pkg/clap/model.go
git commit -m "fix: update CLI auto-tag tests for explicit rule files and inline reflect.Pointer"
```

---

### Task 2: Migrate `pkg/clap` ONNX Runtime Bindings from CGO to `purego`

**Files:**
- Create: `pkg/clap/ort_loader_unix.go`
- Create: `pkg/clap/ort_loader_windows.go`
- Modify: `pkg/clap/onnx.go`
- Modify: `pkg/clap/onnx_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Write failing tests for retryable `ensureORT`, pure-Go tensor helper validation, and `CGO_ENABLED=0` compatibility in `pkg/clap/onnx_test.go`**

Replace `pkg/clap/onnx_test.go` with:

```go
package clap

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
)

func TestIsInvalidAttentionMaskInputError(t *testing.T) {
	t.Parallel()

	if !isInvalidAttentionMaskInputError(errors.New("Error running network: Invalid input name: attention_mask")) {
		t.Fatal("expected attention_mask error to be detected")
	}
	if isInvalidAttentionMaskInputError(errors.New("some other error")) {
		t.Fatal("did not expect unrelated error to be detected")
	}
}

func TestReorderMelFrameToFrameMel(t *testing.T) {
	t.Parallel()

	// 2 mel bins x 3 frames laid out as [mel][frame] -> [frame][mel]
	src := []float32{
		10, 20, 30, // mel 0: frames 0, 1, 2
		11, 21, 31, // mel 1: frames 0, 1, 2
	}
	dst := make([]float32, len(src))
	reorderMelFrameToFrameMel(dst, src, 2, 3)

	want := []float32{
		10, 11, // frame 0
		20, 21, // frame 1
		30, 31, // frame 2
	}
	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("dst[%d] = %v, want %v (full=%v)", i, dst[i], want[i], dst)
		}
	}
}

func TestEnsureORTDoesNotLatchFailureOnMissingLibrary(t *testing.T) {
	err := ensureORT(filepath.Join(t.TempDir(), "does-not-exist-onnxruntime.so"))
	if err == nil {
		t.Fatal("expected error for missing ONNX Runtime shared library")
	}

	// Verify state did not latch as loaded after failed open.
	ortMu.Lock()
	loaded := ortState != nil
	ortMu.Unlock()
	if loaded {
		t.Fatal("expected ortState to remain nil after failed initialization")
	}
}

func TestLocalONNXRuntimeEndToEndWhenAssetsPresent(t *testing.T) {
	libPath := filepath.Clean("../../onnxruntime.so")
	modelDir := filepath.Clean("../../onnx")
	for _, required := range []string{
		libPath,
		filepath.Join(modelDir, "audio_model.onnx"),
		filepath.Join(modelDir, "text_model.onnx"),
		filepath.Join(modelDir, "tokenizer.json"),
	} {
		if _, err := os.Stat(required); err != nil {
			t.Skipf("skipping live ONNX Runtime smoke test (%s not present)", required)
		}
	}

	cfg := DefaultConfig()
	cfg.LibPath = libPath
	model, err := NewModel(modelDir, cfg)
	if err != nil {
		t.Fatalf("NewModel with purego ORT failed: %v", err)
	}
	defer func() {
		if closeErr := model.Close(); closeErr != nil {
			t.Fatalf("Close failed: %v", closeErr)
		}
	}()

	const sampleRate = 48000
	data := make([]float64, sampleRate/2)
	for i := range data {
		data[i] = 0.5 * math.Sin(2*math.Pi*60*float64(i)/sampleRate)
	}
	scores, err := model.ClassifyAudio(context.Background(), zaudio.PCMBuffer{
		SampleRate: sampleRate,
		Channels:   1,
		BitDepth:   16,
		Data:       data,
	}, []string{"bass drum kick", "hi hat cymbal"})
	if err != nil {
		t.Fatalf("ClassifyAudio failed: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("expected 2 label scores, got %d", len(scores))
	}
	for _, s := range scores {
		if math.IsNaN(float64(s.Score)) || math.IsInf(float64(s.Score), 0) {
			t.Fatalf("invalid score for %q: %v", s.Label, s.Score)
		}
	}
}
```

- [ ] **Step 2: Run `CGO_ENABLED=0 go test ./pkg/clap/...` to verify it fails**

Run:
```bash
CGO_ENABLED=0 go test ./pkg/clap/...
```
Expected: FAIL with `github.com/yalue/onnxruntime_go: build constraints exclude all Go files` (and under default `CGO_ENABLED=1`, `ortState` / `ortMu` undefined).

- [ ] **Step 3: Create platform dynamic library loaders `pkg/clap/ort_loader_unix.go` and `pkg/clap/ort_loader_windows.go`**

Create `pkg/clap/ort_loader_unix.go`:

```go
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
```

Create `pkg/clap/ort_loader_windows.go`:

```go
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
```

- [ ] **Step 4: Replace `pkg/clap/onnx.go` with pure-Go `purego` ONNX Runtime C API bindings**

Write `pkg/clap/onnx.go`:

```go
package clap

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// DefaultEmbedDim is the CLAP audio/text embedding size for
// laion/clap-htsat-unfused and laion/clap-htsat-fused.
const DefaultEmbedDim = 512

// ONNX Runtime C API constants (stable since ORT_API_VERSION = 1).
const (
	ortLoggingLevelError = 3

	ortArenaAllocator = 1
	ortMemTypeDefault = 0

	onnxTensorElementDataTypeFloat = 1
	onnxTensorElementDataTypeInt64 = 7
)

// OrtApiBase vtable layout from onnxruntime_c_api.h:
//
//	[0] GetApi(uint32_t version) -> const OrtApi*
//	[1] GetVersionString(void)   -> const char*
type ortApiBaseTable [2]uintptr

// OrtApi vtable prefix from onnxruntime_c_api.h (indices 0..100, frozen since ORT 1.0):
//
//	[2]   GetErrorMessage
//	[3]   CreateEnv
//	[7]   CreateSession
//	[9]   Run
//	[10]  CreateSessionOptions
//	[49]  CreateTensorWithDataAsOrtValue
//	[69]  CreateCpuMemoryInfo
//	[92]  ReleaseEnv
//	[93]  ReleaseStatus
//	[94]  ReleaseMemoryInfo
//	[95]  ReleaseSession
//	[96]  ReleaseValue
//	[100] ReleaseSessionOptions
type ortApiTable [101]uintptr

type ortRuntime struct {
	libPath   string
	libHandle uintptr
	env       uintptr
	memInfo   uintptr

	getErrorMessage                func(status uintptr) string
	createEnv                      func(logLevel int32, logID string, out *uintptr) uintptr
	createSession                  func(env uintptr, modelPath unsafe.Pointer, options uintptr, out *uintptr) uintptr
	run                            func(session uintptr, runOptions uintptr, inputNames **byte, inputs *uintptr, inputLen uintptr, outputNames **byte, outputLen uintptr, outputs *uintptr) uintptr
	createSessionOptions           func(out *uintptr) uintptr
	createTensorWithDataAsOrtValue func(memInfo uintptr, data unsafe.Pointer, dataByteLen uintptr, shape *int64, shapeLen uintptr, elemType int32, out *uintptr) uintptr
	createCpuMemoryInfo            func(allocType int32, memType int32, out *uintptr) uintptr
	releaseEnv                     func(env uintptr)
	releaseStatus                  func(status uintptr)
	releaseMemoryInfo              func(memInfo uintptr)
	releaseSession                 func(session uintptr)
	releaseValue                   func(value uintptr)
	releaseSessionOptions          func(options uintptr)
}

func (r *ortRuntime) checkStatus(status uintptr) error {
	if status == 0 {
		return nil
	}
	defer r.releaseStatus(status)
	return errors.New(r.getErrorMessage(status))
}

var (
	ortMu    sync.Mutex
	ortState *ortRuntime
)

// ensureORT initialises the ONNX Runtime environment using purego.
// Failed initialisation attempts do not latch, allowing subsequent retries.
func ensureORT(libPath string) error {
	ortMu.Lock()
	defer ortMu.Unlock()

	if ortState != nil {
		if libPath != "" && ortState.libPath != "" && ortState.libPath != libPath {
			return fmt.Errorf("onnxruntime already initialised with libPath %q, cannot reinitialise with %q", ortState.libPath, libPath)
		}
		return nil
	}

	candidates := defaultORTLibraryCandidates()
	if libPath != "" {
		candidates = []string{libPath}
	}

	var (
		handle     uintptr
		getBaseSym uintptr
		loadedPath string
		openErrs   []string
	)
	for _, candidate := range candidates {
		h, sym, err := openORTLibrary(candidate)
		if err != nil {
			openErrs = append(openErrs, fmt.Sprintf("%s: %v", candidate, err))
			continue
		}
		handle = h
		getBaseSym = sym
		loadedPath = candidate
		break
	}
	if handle == 0 {
		return fmt.Errorf("load ONNX Runtime shared library (%s)", strings.Join(openErrs, "; "))
	}

	rt, err := initORTRuntime(loadedPath, handle, getBaseSym)
	if err != nil {
		_ = closeORTLibrary(handle)
		return err
	}
	if libPath != "" {
		rt.libPath = libPath
	}
	ortState = rt
	return nil
}

func initORTRuntime(loadedPath string, handle, getBaseSym uintptr) (*ortRuntime, error) {
	var getApiBase func() *ortApiBaseTable
	purego.RegisterFunc(&getApiBase, getBaseSym)

	base := getApiBase()
	if base == nil || base[0] == 0 {
		return nil, fmt.Errorf("OrtGetApiBase in %q returned nil API base", loadedPath)
	}

	var getApi func(version uint32) *ortApiTable
	purego.RegisterFunc(&getApi, base[0])

	// Probe from modern ORT API versions down to v1; all vtable slots we use (0..100)
	// have been fixed since ORT_API_VERSION = 1.
	var api *ortApiTable
	for ver := uint32(24); ver >= 1; ver-- {
		api = getApi(ver)
		if api != nil {
			break
		}
	}
	if api == nil {
		return nil, fmt.Errorf("OrtApiBase.GetApi failed for %q", loadedPath)
	}

	requiredSlots := []int{2, 3, 7, 9, 10, 49, 69, 92, 93, 94, 95, 96, 100}
	for _, slot := range requiredSlots {
		if api[slot] == 0 {
			return nil, fmt.Errorf("OrtApi vtable slot %d is null in %q", slot, loadedPath)
		}
	}

	rt := &ortRuntime{
		libPath:   loadedPath,
		libHandle: handle,
	}
	purego.RegisterFunc(&rt.getErrorMessage, api[2])
	purego.RegisterFunc(&rt.createEnv, api[3])
	purego.RegisterFunc(&rt.createSession, api[7])
	purego.RegisterFunc(&rt.run, api[9])
	purego.RegisterFunc(&rt.createSessionOptions, api[10])
	purego.RegisterFunc(&rt.createTensorWithDataAsOrtValue, api[49])
	purego.RegisterFunc(&rt.createCpuMemoryInfo, api[69])
	purego.RegisterFunc(&rt.releaseEnv, api[92])
	purego.RegisterFunc(&rt.releaseStatus, api[93])
	purego.RegisterFunc(&rt.releaseMemoryInfo, api[94])
	purego.RegisterFunc(&rt.releaseSession, api[95])
	purego.RegisterFunc(&rt.releaseValue, api[96])
	purego.RegisterFunc(&rt.releaseSessionOptions, api[100])

	var env uintptr
	if err := rt.checkStatus(rt.createEnv(ortLoggingLevelError, "zounds-clap", &env)); err != nil {
		return nil, fmt.Errorf("create ORT env: %w", err)
	}
	rt.env = env

	var memInfo uintptr
	if err := rt.checkStatus(rt.createCpuMemoryInfo(ortArenaAllocator, ortMemTypeDefault, &memInfo)); err != nil {
		rt.releaseEnv(rt.env)
		return nil, fmt.Errorf("create ORT CPU memory info: %w", err)
	}
	rt.memInfo = memInfo

	return rt, nil
}

func currentORT() (*ortRuntime, error) {
	ortMu.Lock()
	defer ortMu.Unlock()
	if ortState == nil {
		return nil, errors.New("ONNX Runtime is not initialised")
	}
	return ortState, nil
}

type ortTensor struct {
	rt    *ortRuntime
	value uintptr
	shape []int64
	keep  any
}

func newFloat32Tensor(rt *ortRuntime, shape []int64, buf []float32) (*ortTensor, error) {
	if len(buf) == 0 || len(shape) == 0 {
		return nil, errors.New("float32 tensor buffer and shape must be non-empty")
	}
	shapeCopy := append([]int64(nil), shape...)
	var val uintptr
	status := rt.createTensorWithDataAsOrtValue(
		rt.memInfo,
		unsafe.Pointer(&buf[0]),
		uintptr(len(buf))*unsafe.Sizeof(buf[0]),
		&shapeCopy[0],
		uintptr(len(shapeCopy)),
		onnxTensorElementDataTypeFloat,
		&val,
	)
	if err := rt.checkStatus(status); err != nil {
		return nil, err
	}
	return &ortTensor{rt: rt, value: val, shape: shapeCopy, keep: buf}, nil
}

func newInt64Tensor(rt *ortRuntime, shape []int64, buf []int64) (*ortTensor, error) {
	if len(buf) == 0 || len(shape) == 0 {
		return nil, errors.New("int64 tensor buffer and shape must be non-empty")
	}
	shapeCopy := append([]int64(nil), shape...)
	var val uintptr
	status := rt.createTensorWithDataAsOrtValue(
		rt.memInfo,
		unsafe.Pointer(&buf[0]),
		uintptr(len(buf))*unsafe.Sizeof(buf[0]),
		&shapeCopy[0],
		uintptr(len(shapeCopy)),
		onnxTensorElementDataTypeInt64,
		&val,
	)
	if err := rt.checkStatus(status); err != nil {
		return nil, err
	}
	return &ortTensor{rt: rt, value: val, shape: shapeCopy, keep: buf}, nil
}

func (t *ortTensor) destroy() {
	if t != nil && t.value != 0 && t.rt != nil {
		t.rt.releaseValue(t.value)
		t.value = 0
	}
}

type ortBoundSession struct {
	rt            *ortRuntime
	session       uintptr
	inputCStrings [][]byte
	inputPtrs     []*byte
	inputValues   []uintptr
	outputCStrings [][]byte
	outputPtrs    []*byte
	outputValues  []uintptr
	tensors       []*ortTensor
}

func newORTBoundSession(rt *ortRuntime, modelPath string, inputNames, outputNames []string, inputs, outputs []*ortTensor) (*ortBoundSession, error) {
	if len(inputNames) == 0 || len(inputNames) != len(inputs) {
		return nil, fmt.Errorf("invalid ORT session inputs: %d names, %d tensors", len(inputNames), len(inputs))
	}
	if len(outputNames) == 0 || len(outputNames) != len(outputs) {
		return nil, fmt.Errorf("invalid ORT session outputs: %d names, %d tensors", len(outputNames), len(outputs))
	}

	var opts uintptr
	if err := rt.checkStatus(rt.createSessionOptions(&opts)); err != nil {
		return nil, fmt.Errorf("create ORT session options: %w", err)
	}
	defer rt.releaseSessionOptions(opts)

	encodedPath, pathKeep, err := encodeORTModelPath(modelPath)
	if err != nil {
		return nil, err
	}

	var session uintptr
	err = rt.checkStatus(rt.createSession(rt.env, encodedPath, opts, &session))
	runtime.KeepAlive(pathKeep)
	if err != nil {
		return nil, err
	}

	inCStrings, inPtrs := makeCStringArray(inputNames)
	outCStrings, outPtrs := makeCStringArray(outputNames)

	inVals := make([]uintptr, len(inputs))
	for i, tensor := range inputs {
		inVals[i] = tensor.value
	}
	outVals := make([]uintptr, len(outputs))
	for i, tensor := range outputs {
		outVals[i] = tensor.value
	}

	allTensors := make([]*ortTensor, 0, len(inputs)+len(outputs))
	allTensors = append(allTensors, inputs...)
	allTensors = append(allTensors, outputs...)

	return &ortBoundSession{
		rt:             rt,
		session:        session,
		inputCStrings:  inCStrings,
		inputPtrs:      inPtrs,
		inputValues:    inVals,
		outputCStrings: outCStrings,
		outputPtrs:     outPtrs,
		outputValues:   outVals,
		tensors:        allTensors,
	}, nil
}

func makeCStringArray(names []string) ([][]byte, []*byte) {
	bufs := make([][]byte, len(names))
	ptrs := make([]*byte, len(names))
	for i, name := range names {
		b := make([]byte, len(name)+1)
		copy(b, name)
		bufs[i] = b
		ptrs[i] = &b[0]
	}
	return bufs, ptrs
}

func (s *ortBoundSession) run() error {
	status := s.rt.run(
		s.session,
		0,
		&s.inputPtrs[0],
		&s.inputValues[0],
		uintptr(len(s.inputValues)),
		&s.outputPtrs[0],
		uintptr(len(s.outputValues)),
		&s.outputValues[0],
	)
	runtime.KeepAlive(s.inputCStrings)
	runtime.KeepAlive(s.outputCStrings)
	runtime.KeepAlive(s.inputPtrs)
	runtime.KeepAlive(s.outputPtrs)
	runtime.KeepAlive(s.inputValues)
	runtime.KeepAlive(s.outputValues)
	runtime.KeepAlive(s.tensors)
	return s.rt.checkStatus(status)
}

func (s *ortBoundSession) destroy() {
	if s != nil && s.session != 0 && s.rt != nil {
		s.rt.releaseSession(s.session)
		s.session = 0
	}
}

// audioEncoder wraps an ORT session for the CLAP audio encoder.
// Input:  input_features [1, 1, numFrames, numMelBins] float32
// Output: audio_embeds   [1, embedDim]                  float32
type audioEncoder struct {
	session    *ortBoundSession
	inputBuf   []float32
	outputBuf  []float32
	inTensor   *ortTensor
	outTensor  *ortTensor
	numMelBins int
	numFrames  int
	mu         sync.Mutex
}

func newAudioEncoder(modelPath string, numMelBins, numFrames, embedDim int, inputName, outputName string) (*audioEncoder, error) {
	rt, err := currentORT()
	if err != nil {
		return nil, err
	}

	inputBuf := make([]float32, numMelBins*numFrames)
	outputBuf := make([]float32, embedDim)

	inTensor, err := newFloat32Tensor(rt, []int64{1, 1, int64(numFrames), int64(numMelBins)}, inputBuf)
	if err != nil {
		return nil, fmt.Errorf("create audio input tensor: %w", err)
	}

	outTensor, err := newFloat32Tensor(rt, []int64{1, int64(embedDim)}, outputBuf)
	if err != nil {
		inTensor.destroy()
		return nil, fmt.Errorf("create audio output tensor: %w", err)
	}

	session, err := newORTBoundSession(
		rt,
		modelPath,
		[]string{inputName},
		[]string{outputName},
		[]*ortTensor{inTensor},
		[]*ortTensor{outTensor},
	)
	if err != nil {
		inTensor.destroy()
		outTensor.destroy()
		return nil, fmt.Errorf("create audio encoder session from %q: %w", modelPath, err)
	}

	return &audioEncoder{
		session:    session,
		inputBuf:   inputBuf,
		outputBuf:  outputBuf,
		inTensor:   inTensor,
		outTensor:  outTensor,
		numMelBins: numMelBins,
		numFrames:  numFrames,
	}, nil
}

// encode copies features into the input tensor, runs inference, and returns
// the embedding. features must be laid out as [mel_bin][time_frame] and have
// length numMelBins*numFrames.
func (e *audioEncoder) encode(features []float32) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(features) != len(e.inputBuf) {
		return nil, fmt.Errorf("audio encode: got %d features, want %d", len(features), len(e.inputBuf))
	}
	reorderMelFrameToFrameMel(e.inputBuf, features, e.numMelBins, e.numFrames)

	if err := e.session.run(); err != nil {
		return nil, fmt.Errorf("audio encoder run: %w", err)
	}

	out := make([]float32, len(e.outputBuf))
	copy(out, e.outputBuf)
	return out, nil
}

func reorderMelFrameToFrameMel(dst, src []float32, numMelBins, numFrames int) {
	for mel := range numMelBins {
		srcBase := mel * numFrames
		for frame := range numFrames {
			dst[frame*numMelBins+mel] = src[srcBase+frame]
		}
	}
}

func (e *audioEncoder) close() error {
	e.session.destroy()
	e.inTensor.destroy()
	e.outTensor.destroy()
	return nil
}

// textEncoder wraps an ORT session for the CLAP text encoder.
// Inputs:  input_ids [1, maxTokens] int64, attention_mask [1, maxTokens] int64
// Output:  text_embeds [1, embedDim] float32
type textEncoder struct {
	rt                *ortRuntime
	session           *ortBoundSession
	idsBuf            []int64
	maskBuf           []int64
	outputBuf         []float32
	idsTensor         *ortTensor
	maskTensor        *ortTensor
	outTensor         *ortTensor
	modelPath         string
	inputIDsName      string
	attentionMaskName string
	outputName        string
	useAttentionMask  bool
	mu                sync.Mutex
}

func newTextEncoder(modelPath string, maxTokens, embedDim int, inputIDsName, attentionMaskName, outputName string) (*textEncoder, error) {
	rt, err := currentORT()
	if err != nil {
		return nil, err
	}

	idsBuf := make([]int64, maxTokens)
	maskBuf := make([]int64, maxTokens)
	outputBuf := make([]float32, embedDim)

	idsTensor, err := newInt64Tensor(rt, []int64{1, int64(maxTokens)}, idsBuf)
	if err != nil {
		return nil, fmt.Errorf("create ids tensor: %w", err)
	}
	maskTensor, err := newInt64Tensor(rt, []int64{1, int64(maxTokens)}, maskBuf)
	if err != nil {
		idsTensor.destroy()
		return nil, fmt.Errorf("create mask tensor: %w", err)
	}
	outTensor, err := newFloat32Tensor(rt, []int64{1, int64(embedDim)}, outputBuf)
	if err != nil {
		idsTensor.destroy()
		maskTensor.destroy()
		return nil, fmt.Errorf("create text output tensor: %w", err)
	}

	session, err := newORTBoundSession(
		rt,
		modelPath,
		[]string{inputIDsName, attentionMaskName},
		[]string{outputName},
		[]*ortTensor{idsTensor, maskTensor},
		[]*ortTensor{outTensor},
	)
	if err != nil {
		idsTensor.destroy()
		maskTensor.destroy()
		outTensor.destroy()
		return nil, fmt.Errorf("create text encoder session from %q: %w", modelPath, err)
	}

	return &textEncoder{
		rt:                rt,
		session:           session,
		idsBuf:            idsBuf,
		maskBuf:           maskBuf,
		outputBuf:         outputBuf,
		idsTensor:         idsTensor,
		maskTensor:        maskTensor,
		outTensor:         outTensor,
		modelPath:         modelPath,
		inputIDsName:      inputIDsName,
		attentionMaskName: attentionMaskName,
		outputName:        outputName,
		useAttentionMask:  true,
	}, nil
}

// encode fills the token tensors, runs inference, and returns the embedding.
// ids and mask must each have length maxTokens.
func (e *textEncoder) encode(ids, mask []int64) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(ids) != len(e.idsBuf) {
		return nil, fmt.Errorf("text encode: got %d ids, want %d", len(ids), len(e.idsBuf))
	}
	if len(mask) != len(e.maskBuf) {
		return nil, fmt.Errorf("text encode: got %d mask values, want %d", len(mask), len(e.maskBuf))
	}
	copy(e.idsBuf, ids)
	copy(e.maskBuf, mask)

	if err := e.session.run(); err != nil {
		if e.useAttentionMask && isInvalidAttentionMaskInputError(err) {
			if rebuildErr := e.rebuildSessionWithoutAttentionMask(); rebuildErr != nil {
				return nil, fmt.Errorf("text encoder reconfigure without attention mask: %w", rebuildErr)
			}
			if err = e.session.run(); err != nil {
				return nil, fmt.Errorf("text encoder run: %w", err)
			}
		} else {
			return nil, fmt.Errorf("text encoder run: %w", err)
		}
	}

	out := make([]float32, len(e.outputBuf))
	copy(out, e.outputBuf)
	return out, nil
}

func (e *textEncoder) rebuildSessionWithoutAttentionMask() error {
	e.session.destroy()

	session, err := newORTBoundSession(
		e.rt,
		e.modelPath,
		[]string{e.inputIDsName},
		[]string{e.outputName},
		[]*ortTensor{e.idsTensor},
		[]*ortTensor{e.outTensor},
	)
	if err != nil {
		return err
	}

	e.session = session
	e.useAttentionMask = false
	return nil
}

func isInvalidAttentionMaskInputError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Invalid input name: attention_mask")
}

func (e *textEncoder) close() error {
	e.session.destroy()
	e.idsTensor.destroy()
	e.maskTensor.destroy()
	e.outTensor.destroy()
	return nil
}
```

- [ ] **Step 5: Run `go mod tidy` and verify `CGO_ENABLED=0` builds and tests across Linux, macOS, and Windows targets**

Run:
```bash
go mod tidy
CGO_ENABLED=0 go test ./pkg/clap/... -v
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/zounds
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build ./cmd/zounds
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./cmd/zounds
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/zounds-test.exe ./cmd/zounds
rm -f zounds /tmp/zounds-test.exe
golangci-lint run ./pkg/clap/...
```
Expected:
- `go.mod` drops `github.com/yalue/onnxruntime_go` and promotes `github.com/ebitengine/purego` to direct `require`.
- `CGO_ENABLED=0 go test ./pkg/clap/... -v` PASSES (including `TestLocalONNXRuntimeEndToEndWhenAssetsPresent` when `./onnxruntime.so` and `./onnx/` exist locally).
- All 4 cross-compilation targets (`linux/amd64`, `linux/arm64`, `darwin/arm64`, `windows/amd64`) succeed with `CGO_ENABLED=0`.
- `golangci-lint run ./pkg/clap/...` exits with `0 issues`.

- [ ] **Step 6: Commit Task 2**

```bash
git add go.mod go.sum pkg/clap/onnx.go pkg/clap/onnx_test.go pkg/clap/ort_loader_unix.go pkg/clap/ort_loader_windows.go
git commit -m "feat(clap): replace cgo onnxruntime_go with purego C API bindings"
```

---

### Task 3: Namespaced Tag Normalization (`facet:value`) & Safe Collision-Checked Rename

**Files:**
- Create: `pkg/core/types_test.go`
- Modify: `pkg/core/types.go:116-121`
- Modify: `pkg/rename/template.go`
- Modify: `pkg/rename/template_test.go`
- Modify: `cmd/zounds/commands/rename.go`
- Modify: `cmd/zounds/commands/rename_test.go`

- [ ] **Step 1: Write failing unit tests for `core.NormalizeTagName`, `rename` `slug()`/`ResolveTarget`/`ValidatePlan`, and CLI `zounds rename`**

Create `pkg/core/types_test.go`:

```go
package core_test

import (
	"testing"

	"github.com/darkliquid/zounds/pkg/core"
)

func TestNormalizeTagNamePreservesFacetNamespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{input: "instrument:kick", want: "instrument:kick"},
		{input: "  Instrument : Sub_Bass  ", want: "instrument:sub bass"},
		{input: "character :  dark-metallic ", want: "character:dark metallic"},
		{input: "Dark_Synth-Pad", want: "dark synth pad"},
		{input: " : ", want: ""},
		{input: ":kick", want: "kick"},
		{input: "instrument:", want: "instrument"},
	}

	for _, tc := range cases {
		if got := core.NormalizeTagName(tc.input); got != tc.want {
			t.Errorf("NormalizeTagName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
```

Update imports in `pkg/rename/template_test.go` to include `"os"` and `"path/filepath"`, and append new tests:

```go
func TestSlugSanitizesNamespacesAndPathTraversal(t *testing.T) {
	t.Parallel()

	data := rename.BuildTemplateData(core.Sample{
		FileName:  "kick.v2.wav",
		Extension: "wav",
		Format:    core.FormatWAV,
	}, []core.Tag{
		{Name: "instrument:sub_bass", NormalizedName: "instrument:sub bass", Source: "rules"},
	}, map[string]string{
		"unsafe": "../../etc/passwd:evil*name?",
	})

	rendered, err := rename.RenderTemplate(`{{slug (index .Tags 0)}}_{{slug .Attributes.unsafe}}_{{slug .Stem}}.{{.Extension}}`, data)
	if err != nil {
		t.Fatalf("render template: %v", err)
	}
	if rendered != "instrument-sub-bass_etc-passwd-evil-name_kick.v2.wav" {
		t.Fatalf("unexpected rendered template: %q", rendered)
	}
}

func TestResolveTargetPreservesExtensionForDottedStemAndRejectsTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	sample := core.Sample{
		SourceRoot: root,
		Path:       filepath.Join(root, "Drums", "kick.v2.wav"),
		FileName:   "kick.v2.wav",
		Extension:  "wav",
		Format:     core.FormatWAV,
	}

	dest, err := rename.ResolveTarget(sample, "renamed/kick.v2")
	if err != nil {
		t.Fatalf("ResolveTarget dotted stem: %v", err)
	}
	want := filepath.Join(root, "renamed", "kick.v2.wav")
	if dest != want {
		t.Fatalf("ResolveTarget = %q, want %q", dest, want)
	}

	if _, err := rename.ResolveTarget(sample, "../outside/kick.wav"); err == nil {
		t.Fatal("expected error when relative rename target escapes SourceRoot")
	}
}

func TestValidatePlanRejectsDuplicateTargetsAndExistingFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	existingTarget := filepath.Join(root, "existing.wav")
	if err := os.WriteFile(existingTarget, []byte("audio"), 0o644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	dupOps := []rename.RenameOp{
		{Sample: core.Sample{ID: 1, Path: filepath.Join(root, "a.wav")}, Destination: filepath.Join(root, "out", "same.wav")},
		{Sample: core.Sample{ID: 2, Path: filepath.Join(root, "b.wav")}, Destination: filepath.Join(root, "out", "same.wav")},
	}
	if err := rename.ValidatePlan(dupOps); err == nil {
		t.Fatal("expected duplicate destination error")
	}

	overwriteOps := []rename.RenameOp{
		{Sample: core.Sample{ID: 1, Path: filepath.Join(root, "a.wav")}, Destination: existingTarget},
	}
	if err := rename.ValidatePlan(overwriteOps); err == nil {
		t.Fatal("expected existing destination overwrite error")
	}
}
```

Append new CLI tests to `cmd/zounds/commands/rename_test.go` (adding `"os"` to imports):

```go
func TestRenameCommandPreservesDottedStemExtension(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rename-dotted.db")
	samplePath := filepath.Join(root, "Drums", "kick.v2.wav")
	writeWAVFixture(t, samplePath)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "rename", "--path", samplePath, "--template", `moved/{{.Stem}}`})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute rename: %v", err)
	}

	wantPath := filepath.Join(root, "moved", "kick.v2.wav")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected renamed file at %s: %v", wantPath, err)
	}
}

func TestRenameCommandAbortsBatchBeforeMovingOnCollision(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rename-collision.db")
	first := filepath.Join(root, "A", "one.wav")
	second := filepath.Join(root, "B", "two.wav")
	writeWAVFixture(t, first)
	writeWAVFixture(t, second)

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "rename", "--template", `flat/colliding_name`})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected collision error for duplicate batch targets")
	}

	for _, orig := range []string{first, second} {
		if _, err := os.Stat(orig); err != nil {
			t.Fatalf("expected original file %s untouched after failed preflight: %v", orig, err)
		}
	}
}

func TestRenameCommandRefusesToOverwriteExistingFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "rename-overwrite.db")
	source := filepath.Join(root, "source.wav")
	existing := filepath.Join(root, "target.wav")
	writeWAVFixture(t, source)
	if err := os.WriteFile(existing, []byte("precious-unindexed-file"), 0o644); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	cmd.SetArgs([]string{"--db", dbPath, "rename", "--path", source, "--template", `target.wav`})
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected error refusing to overwrite existing target.wav")
	}

	gotBytes, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("read existing target: %v", err)
	}
	if string(gotBytes) != "precious-unindexed-file" {
		t.Fatalf("existing file was overwritten: %q", string(gotBytes))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./pkg/core/... ./pkg/rename/... ./cmd/zounds/commands -run 'TestNormalizeTagName|TestSlug|TestResolveTarget|TestValidatePlan|TestRenameCommand' -v
```
Expected: FAIL (`NormalizeTagName("  Instrument : Sub_Bass  ")` returns `"instrument : sub bass"`, `rename.ResolveTarget` / `rename.ValidatePlan` / `rename.RenameOp` undefined, CLI rename overwrites/collides).

- [ ] **Step 3: Implement `NormalizeTagName` namespace handling in `pkg/core/types.go`**

Update `NormalizeTagName` in `pkg/core/types.go:116-121`:

```go
func NormalizeTagName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	if !strings.Contains(name, ":") {
		return strings.Join(strings.Fields(name), " ")
	}

	parts := strings.Split(name, ":")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		segment := strings.Join(strings.Fields(part), " ")
		if segment != "" {
			cleaned = append(cleaned, segment)
		}
	}
	return strings.Join(cleaned, ":")
}
```

- [ ] **Step 4: Implement sanitized `slug()`, `ResolveTarget`, `RenameOp`, and `ValidatePlan` in `pkg/rename/template.go` and wire `cmd/zounds/commands/rename.go`**

Update `pkg/rename/template.go` (`slug` + target resolution + batch preflight):

```go
package rename

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/darkliquid/zounds/pkg/core"
)

type TemplateData struct {
	ID           int64
	Name         string
	Stem         string
	Extension    string
	Format       string
	Path         string
	RelativePath string
	SourceRoot   string
	SizeBytes    int64
	Tags         []string
	TagsBySource map[string][]string
	Attributes   map[string]string
	Metadata     map[string]string
}

type RenameOp struct {
	Sample      core.Sample
	Destination string
}

func BuildTemplateData(sample core.Sample, tags []core.Tag, attributes map[string]string) TemplateData {
	tagNames := make([]string, 0, len(tags))
	bySource := make(map[string][]string)
	for _, tag := range tags {
		name := tag.NormalizedName
		if name == "" {
			name = core.NormalizeTagName(tag.Name)
		}
		if name == "" {
			continue
		}
		tagNames = append(tagNames, name)
		if tag.Source != "" {
			bySource[tag.Source] = append(bySource[tag.Source], name)
		}
	}

	slices.Sort(tagNames)
	for source := range bySource {
		slices.Sort(bySource[source])
	}

	return TemplateData{
		ID:           sample.ID,
		Name:         sample.FileName,
		Stem:         strings.TrimSuffix(sample.FileName, filepath.Ext(sample.FileName)),
		Extension:    sample.Extension,
		Format:       string(sample.Format),
		Path:         sample.Path,
		RelativePath: sample.RelativePath,
		SourceRoot:   sample.SourceRoot,
		SizeBytes:    sample.SizeBytes,
		Tags:         tagNames,
		TagsBySource: bySource,
		Attributes:   cloneStringMap(attributes),
		Metadata:     cloneStringMap(sample.Metadata),
	}
}

func RenderTemplate(text string, data TemplateData) (string, error) {
	tmpl, err := template.New("rename").Funcs(template.FuncMap{
		"join":  strings.Join,
		"slug":  slug,
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
	}).Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse rename template: %w", err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("execute rename template: %w", err)
	}
	return out.String(), nil
}

func ResolveTarget(sample core.Sample, rendered string) (string, error) {
	rendered = strings.TrimSpace(rendered)
	if rendered == "" {
		return "", fmt.Errorf("rename template rendered an empty path")
	}
	if strings.ContainsRune(rendered, '\x00') {
		return "", fmt.Errorf("rename template rendered NUL byte")
	}
	if strings.HasSuffix(rendered, "/") || strings.HasSuffix(rendered, `\`) {
		return "", fmt.Errorf("rename template rendered directory path %q without filename", rendered)
	}

	if sample.Extension != "" {
		wantExt := "." + sample.Extension
		if !strings.EqualFold(filepath.Ext(rendered), wantExt) {
			rendered += wantExt
		}
	}

	var dest string
	if filepath.IsAbs(rendered) {
		dest = filepath.Clean(rendered)
	} else {
		if strings.TrimSpace(sample.SourceRoot) == "" {
			return "", fmt.Errorf("sample %q has empty SourceRoot for relative rename target %q", sample.Path, rendered)
		}
		rootClean := filepath.Clean(sample.SourceRoot)
		dest = filepath.Clean(filepath.Join(rootClean, rendered))
		rel, err := filepath.Rel(rootClean, dest)
		if err != nil {
			return "", fmt.Errorf("resolve relative rename target %q: %w", rendered, err)
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("rename target %q escapes source root %q", rendered, rootClean)
		}
	}

	base := filepath.Base(dest)
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return "", fmt.Errorf("rename target %q does not name a valid file", dest)
	}

	return dest, nil
}

func ValidatePlan(ops []RenameOp) error {
	seenTargets := make(map[string]string, len(ops))
	for _, op := range ops {
		dest := filepath.Clean(op.Destination)
		src := filepath.Clean(op.Sample.Path)
		if prevSrc, exists := seenTargets[dest]; exists && prevSrc != src {
			return fmt.Errorf("batch rename target collision: both %q and %q map to %q", prevSrc, src, dest)
		}
		seenTargets[dest] = src

		if src == dest {
			continue
		}
		if _, err := os.Lstat(dest); err == nil {
			return fmt.Errorf("rename destination %q already exists (refusing to overwrite from %q)", dest, src)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check rename destination %q: %w", dest, err)
		}
	}
	return nil
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsControl(r):
			b.WriteByte(' ')
		case strings.ContainsRune(` _-:/\<>|"?*`, r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}

	tokens := strings.Fields(b.String())
	cleaned := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		for strings.Contains(tok, "..") {
			tok = strings.ReplaceAll(tok, "..", ".")
		}
		tok = strings.Trim(tok, ".")
		if tok != "" {
			cleaned = append(cleaned, tok)
		}
	}
	return strings.Join(cleaned, "-")
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
```

Update `cmd/zounds/commands/rename.go` to build and validate the full batch plan BEFORE printing/executing any `os.Rename`:

```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/darkliquid/zounds/pkg/core"
	zrename "github.com/darkliquid/zounds/pkg/rename"
)

func newRenameCommand(cfg *Config) *cobra.Command {
	var (
		templateText string
		targetPath   string
	)

	cmd := &cobra.Command{
		Use:   "rename --template <template>",
		Short: "Mass rename and reorganize library files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(templateText) == "" {
				return fmt.Errorf("--template is required")
			}

			ctx := cmd.Context()
			repo, closer, err := openRepository(ctx, cfg)
			if err != nil {
				return err
			}
			defer func() { _ = closer.Close() }()

			samples, err := selectSamplesForTagging(ctx, repo, targetPath)
			if err != nil {
				return err
			}

			ops := make([]zrename.RenameOp, 0, len(samples))
			for _, sample := range samples {
				tags, err := repo.ListTagsForSample(ctx, sample.ID)
				if err != nil {
					return err
				}
				rendered, err := zrename.RenderTemplate(templateText, zrename.BuildTemplateData(sample, tags, nil))
				if err != nil {
					return err
				}
				destination, err := zrename.ResolveTarget(sample, rendered)
				if err != nil {
					return err
				}
				ops = append(ops, zrename.RenameOp{
					Sample:      sample,
					Destination: destination,
				})
			}

			if err := zrename.ValidatePlan(ops); err != nil {
				return err
			}

			for _, op := range ops {
				sample := op.Sample
				destination := op.Destination
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", sample.Path, destination); err != nil {
					return err
				}

				if cfg.DryRun || filepath.Clean(sample.Path) == destination {
					continue
				}

				if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
					return fmt.Errorf("create target directory: %w", err)
				}
				if err := os.Rename(sample.Path, destination); err != nil {
					return fmt.Errorf("rename %q -> %q: %w", sample.Path, destination, err)
				}

				info, err := os.Stat(destination)
				if err != nil {
					return fmt.Errorf("stat renamed file %q: %w", destination, err)
				}
				relativePath, err := filepath.Rel(sample.SourceRoot, destination)
				if err != nil {
					return fmt.Errorf("compute relative path for %q: %w", destination, err)
				}
				if err := repo.UpdateSampleLocation(ctx, sample.ID, destination, filepath.ToSlash(relativePath), filepath.Base(destination), core.DetectFormatFromExtension(destination), info.Size(), info.ModTime()); err != nil {
					return err
				}
			}

			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&templateText, "template", "", "Go template for the new relative path or file name")
	flags.StringVar(&targetPath, "path", "", "rename only one indexed sample path")

	return cmd
}
```

- [ ] **Step 5: Run tests and linter to verify Task 3**

Run:
```bash
go test ./pkg/core/... ./pkg/rename/... ./pkg/tags/... ./cmd/zounds/commands/... -v
golangci-lint run ./pkg/core/... ./pkg/rename/... ./cmd/zounds/commands/...
```
Expected: All tests PASS and `golangci-lint` reports `0 issues`.

- [ ] **Step 6: Commit Task 3**

```bash
git add pkg/core/types.go pkg/core/types_test.go pkg/rename/template.go pkg/rename/template_test.go cmd/zounds/commands/rename.go cmd/zounds/commands/rename_test.go
git commit -m "fix(rename,core): support facet:value tag namespacing and add batch rename collision preflight"
```

---

### Task 4: Audio Buffer & Codec Data-Safety Fixes (`Validate()`, OGG Clamping, 8-Bit PCM, IEEE Float WAV)

**Files:**
- Modify: `pkg/audio/buffer.go:61-67`
- Modify: `pkg/audio/buffer_test.go`
- Modify: `pkg/audio/ogg/codec.go:39-44`
- Create: `pkg/audio/ogg/codec_test.go`
- Modify: `pkg/audio/pcm.go`
- Modify: `pkg/audio/aiff/codec.go:36-46`
- Modify: `pkg/audio/aiff/codec_test.go`
- Modify: `pkg/audio/wav/codec.go`
- Modify: `pkg/audio/wav/codec_test.go`

- [ ] **Step 1: Write failing tests for `PCMBuffer.Validate()` (`NaN`/`Inf`), OGG overshoot clamping, 8-bit WAV & AIFF round-trip, and 32/64-bit IEEE Float WAV decoding**

Append to `pkg/audio/buffer_test.go` (adding `"math"` to imports):

```go
func TestPCMBufferValidateRejectsNaNAndInf(t *testing.T) {
	t.Parallel()

	for name, bad := range map[string]float64{
		"NaN":  math.NaN(),
		"+Inf": math.Inf(1),
		"-Inf": math.Inf(-1),
	} {
		buf := zaudio.PCMBuffer{
			SampleRate: 44100,
			Channels:   1,
			BitDepth:   32,
			Data:       []float64{0.0, bad, 0.25},
		}
		if err := buf.Validate(); err == nil {
			t.Fatalf("expected Validate() to reject %s", name)
		}
	}
}
```

Create `pkg/audio/ogg/codec_test.go`:

```go
package ogg

import (
	"math"
	"testing"
)

func TestClampDecodedSamplesClampsOvershootAndRejectsNaN(t *testing.T) {
	t.Parallel()

	data, err := clampVorbisSamples([]float32{-1.04, -0.5, 0.0, 0.5, 1.003})
	if err != nil {
		t.Fatalf("clampVorbisSamples: %v", err)
	}
	want := []float64{-1.0, -0.5, 0.0, 0.5, 1.0}
	for i := range want {
		if math.Abs(data[i]-want[i]) > 1e-6 {
			t.Fatalf("data[%d] = %v, want %v", i, data[i], want[i])
		}
	}

	if _, err := clampVorbisSamples([]float32{0.1, float32(math.NaN())}); err == nil {
		t.Fatal("expected error when Vorbis stream produces NaN")
	}
}
```

Append to `pkg/audio/aiff/codec_test.go` (adding `"bytes"` and `"math"` to imports):

```go
func TestAIFFCodec8BitSignedRoundTrip(t *testing.T) {
	t.Parallel()

	codec := zaiff.New()
	want := []float64{-1.0, -0.5, 0.0, 0.5, 127.0 / 128.0}
	buffer := zaudio.PCMBuffer{
		SampleRate: 22050,
		Channels:   1,
		BitDepth:   8,
		Data:       want,
	}

	filePath := filepath.Join(t.TempDir(), "eightbit.aiff")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create 8-bit aiff: %v", err)
	}
	if err := codec.Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode 8-bit aiff: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close 8-bit aiff: %v", err)
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read raw 8-bit aiff: %v", err)
	}
	ssndIdx := bytes.Index(raw, []byte("SSND"))
	if ssndIdx < 0 || ssndIdx+16+len(want) > len(raw) {
		t.Fatalf("SSND chunk missing or truncated in 8-bit aiff")
	}
	// Sample index 2 is 0.0 -> must be 0x00 in signed two's complement AIFF (not 0x80).
	if raw[ssndIdx+16+2] != 0x00 {
		t.Fatalf("expected 0x00 silence byte in 8-bit AIFF SSND, got 0x%02x", raw[ssndIdx+16+2])
	}

	opened, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open 8-bit aiff: %v", err)
	}
	defer func() { _ = opened.Close() }()

	decoded, err := codec.Decode(context.Background(), opened)
	if err != nil {
		t.Fatalf("decode 8-bit aiff: %v", err)
	}
	if err := decoded.Buffer.Validate(); err != nil {
		t.Fatalf("validate decoded 8-bit aiff: %v", err)
	}
	for i := range want {
		if math.Abs(decoded.Buffer.Data[i]-want[i]) > 0.02 {
			t.Fatalf("sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
		}
	}
}
```

Append to `pkg/audio/wav/codec_test.go` (adding `"bytes"`, `"encoding/binary"`, and `"math"` to imports):

```go
func TestWAVCodec8BitUnsignedRoundTrip(t *testing.T) {
	t.Parallel()

	codec := zwav.New()
	want := []float64{-1.0, -0.5, 0.0, 0.5, 127.0 / 128.0}
	buffer := zaudio.PCMBuffer{
		SampleRate: 22050,
		Channels:   1,
		BitDepth:   8,
		Data:       want,
	}

	filePath := filepath.Join(t.TempDir(), "eightbit.wav")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create 8-bit wav: %v", err)
	}
	if err := codec.Encode(context.Background(), file, buffer); err != nil {
		t.Fatalf("encode 8-bit wav: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close 8-bit wav: %v", err)
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read raw 8-bit wav: %v", err)
	}
	dataIdx := bytes.Index(raw, []byte("data"))
	if dataIdx < 0 || dataIdx+8+len(want) > len(raw) {
		t.Fatalf("data chunk missing in 8-bit wav")
	}
	// Sample index 2 is 0.0 -> must be 0x80 (128) in unsigned RIFF WAVE 8-bit PCM.
	if raw[dataIdx+8+2] != 128 {
		t.Fatalf("expected 128 silence byte in 8-bit WAV, got %d", raw[dataIdx+8+2])
	}

	opened, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open 8-bit wav: %v", err)
	}
	defer func() { _ = opened.Close() }()

	decoded, err := codec.Decode(context.Background(), opened)
	if err != nil {
		t.Fatalf("decode 8-bit wav: %v", err)
	}
	if err := decoded.Buffer.Validate(); err != nil {
		t.Fatalf("validate decoded 8-bit wav: %v", err)
	}
	for i := range want {
		if math.Abs(decoded.Buffer.Data[i]-want[i]) > 0.02 {
			t.Fatalf("sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
		}
	}
}

func TestWAVCodecDecodesIEEEFloat32AndFloat64AndExtensible(t *testing.T) {
	t.Parallel()

	codec := zwav.New()
	samples := []float64{-1.0, -0.25, 0.0, 0.25, 0.875, 1.05} // includes slight DAW overshoot 1.05 -> clamped to 1.0
	want := []float64{-1.0, -0.25, 0.0, 0.25, 0.875, 1.0}

	t.Run("float32_format3", func(t *testing.T) {
		t.Parallel()
		wavBytes := buildFloatWAVFixture(t, 3, 32, 48000, 2, samples)
		decoded, err := codec.Decode(context.Background(), bytes.NewReader(wavBytes))
		if err != nil {
			t.Fatalf("decode float32 wav: %v", err)
		}
		if err := decoded.Buffer.Validate(); err != nil {
			t.Fatalf("validate float32 wav: %v", err)
		}
		if decoded.Info.SampleRate != 48000 || decoded.Info.Channels != 2 || decoded.Info.BitDepth != 32 {
			t.Fatalf("unexpected stream info: %+v", decoded.Info)
		}
		for i := range want {
			if math.Abs(decoded.Buffer.Data[i]-want[i]) > 1e-6 {
				t.Fatalf("float32 sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
			}
		}
	})

	t.Run("float64_format3", func(t *testing.T) {
		t.Parallel()
		wavBytes := buildFloatWAVFixture(t, 3, 64, 96000, 1, samples)
		decoded, err := codec.Decode(context.Background(), bytes.NewReader(wavBytes))
		if err != nil {
			t.Fatalf("decode float64 wav: %v", err)
		}
		if err := decoded.Buffer.Validate(); err != nil {
			t.Fatalf("validate float64 wav: %v", err)
		}
		for i := range want {
			if math.Abs(decoded.Buffer.Data[i]-want[i]) > 1e-12 {
				t.Fatalf("float64 sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
			}
		}
	})

	t.Run("float32_extensible_fffe", func(t *testing.T) {
		t.Parallel()
		wavBytes := buildFloatWAVFixture(t, 0xFFFE, 32, 44100, 2, samples)
		decoded, err := codec.Decode(context.Background(), bytes.NewReader(wavBytes))
		if err != nil {
			t.Fatalf("decode extensible float32 wav: %v", err)
		}
		if err := decoded.Buffer.Validate(); err != nil {
			t.Fatalf("validate extensible float32 wav: %v", err)
		}
		for i := range want {
			if math.Abs(decoded.Buffer.Data[i]-want[i]) > 1e-6 {
				t.Fatalf("extensible float32 sample[%d] = %v, want %v", i, decoded.Buffer.Data[i], want[i])
			}
		}
	})
}

func buildFloatWAVFixture(t *testing.T, formatTag uint16, bitsPerSample uint16, sampleRate uint32, channels uint16, samples []float64) []byte {
	t.Helper()

	bytesPerSample := int(bitsPerSample / 8)
	dataPayload := new(bytes.Buffer)
	for _, s := range samples {
		switch bitsPerSample {
		case 32:
			_ = binary.Write(dataPayload, binary.LittleEndian, float32(s))
		case 64:
			_ = binary.Write(dataPayload, binary.LittleEndian, s)
		default:
			t.Fatalf("unsupported test bitsPerSample %d", bitsPerSample)
		}
	}

	fmtPayload := new(bytes.Buffer)
	blockAlign := channels * uint16(bytesPerSample)
	byteRate := sampleRate * uint32(blockAlign)
	_ = binary.Write(fmtPayload, binary.LittleEndian, formatTag)
	_ = binary.Write(fmtPayload, binary.LittleEndian, channels)
	_ = binary.Write(fmtPayload, binary.LittleEndian, sampleRate)
	_ = binary.Write(fmtPayload, binary.LittleEndian, byteRate)
	_ = binary.Write(fmtPayload, binary.LittleEndian, blockAlign)
	_ = binary.Write(fmtPayload, binary.LittleEndian, bitsPerSample)
	if formatTag == 0xFFFE {
		_ = binary.Write(fmtPayload, binary.LittleEndian, uint16(22))          // cbSize
		_ = binary.Write(fmtPayload, binary.LittleEndian, bitsPerSample)       // wValidBitsPerSample
		_ = binary.Write(fmtPayload, binary.LittleEndian, uint32(0x3))         // dwChannelMask
		subFormatIEEEFloat := [16]byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}
		_, _ = fmtPayload.Write(subFormatIEEEFloat[:])
	}

	out := new(bytes.Buffer)
	_, _ = out.WriteString("RIFF")
	riffSize := uint32(4 + (8 + fmtPayload.Len()) + (8 + dataPayload.Len()))
	_ = binary.Write(out, binary.LittleEndian, riffSize)
	_, _ = out.WriteString("WAVE")
	_, _ = out.WriteString("fmt ")
	_ = binary.Write(out, binary.LittleEndian, uint32(fmtPayload.Len()))
	_, _ = out.Write(fmtPayload.Bytes())
	_, _ = out.WriteString("data")
	_ = binary.Write(out, binary.LittleEndian, uint32(dataPayload.Len()))
	_, _ = out.Write(dataPayload.Bytes())
	return out.Bytes()
}
```

- [ ] **Step 2: Run audio package tests to verify every new test fails**

Run:
```bash
go test ./pkg/audio/... -v
```
Expected: FAIL across `TestPCMBufferValidateRejectsNaNAndInf`, `pkg/audio/ogg` (`clampVorbisSamples` undefined), `TestAIFFCodec8BitSignedRoundTrip`, `TestWAVCodec8BitUnsignedRoundTrip`, and `TestWAVCodecDecodesIEEEFloat32AndFloat64AndExtensible`.

- [ ] **Step 3: Implement `PCMBuffer.Validate()`, OGG clamping, 8-bit PCM helpers, AIFF 8-bit sign-extension, and IEEE Float WAV decoding**

1. Update `pkg/audio/buffer.go` (add `"math"` to imports):

```go
	for i, sample := range b.Data {
		if math.IsNaN(sample) || math.IsInf(sample, 0) || sample < -1.0 || sample > 1.0 {
			return fmt.Errorf("sample at index %d is out of range [-1, 1]: %v", i, sample)
		}
	}
```

2. Update `pkg/audio/ogg/codec.go` (add `"math"` to imports):

```go
	data, err := clampVorbisSamples(samples)
	if err != nil {
		return zaudio.DecodeResult{}, err
	}
```

and add helper in `pkg/audio/ogg/codec.go`:

```go
func clampVorbisSamples(samples []float32) ([]float64, error) {
	data := make([]float64, len(samples))
	for i, sample := range samples {
		v := float64(sample)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("decode ogg/vorbis: invalid sample value at index %d: %v", i, v)
		}
		if v < -1.0 {
			v = -1.0
		} else if v > 1.0 {
			v = 1.0
		}
		data[i] = v
	}
	return data, nil
}
```

3. Update `pkg/audio/pcm.go` to add `Unsigned8IntBufferToPCM` and `PCMToUnsigned8IntBuffer` (and refine signed `PCMToIntBuffer` scaling so `-1.0` maps cleanly to `minInt`):

```go
package audio

import (
	"fmt"
	"math"

	rawaudio "github.com/go-audio/audio"
)

func IntBufferToPCM(buffer *rawaudio.IntBuffer, bitDepth int) (PCMBuffer, error) {
	if buffer == nil || buffer.Format == nil {
		return PCMBuffer{}, fmt.Errorf("int buffer is missing format information")
	}

	if bitDepth <= 0 {
		bitDepth = buffer.SourceBitDepth
	}
	if bitDepth <= 0 {
		bitDepth = 16
	}

	scale := math.Pow(2, float64(bitDepth-1))
	data := make([]float64, len(buffer.Data))
	for i, sample := range buffer.Data {
		data[i] = clamp(float64(sample)/scale, -1, 1)
	}

	return PCMBuffer{
		SampleRate: buffer.Format.SampleRate,
		Channels:   buffer.Format.NumChannels,
		BitDepth:   bitDepth,
		Data:       data,
	}, nil
}

func Unsigned8IntBufferToPCM(buffer *rawaudio.IntBuffer) (PCMBuffer, error) {
	if buffer == nil || buffer.Format == nil {
		return PCMBuffer{}, fmt.Errorf("int buffer is missing format information")
	}

	data := make([]float64, len(buffer.Data))
	for i, sample := range buffer.Data {
		u8 := clamp(float64(sample), 0, 255)
		data[i] = clamp((u8-128.0)/128.0, -1, 1)
	}

	return PCMBuffer{
		SampleRate: buffer.Format.SampleRate,
		Channels:   buffer.Format.NumChannels,
		BitDepth:   8,
		Data:       data,
	}, nil
}

func PCMToIntBuffer(buffer PCMBuffer) (*rawaudio.IntBuffer, error) {
	if err := buffer.Validate(); err != nil {
		return nil, err
	}

	bitDepth := buffer.BitDepth
	if bitDepth <= 0 {
		bitDepth = 16
	}

	scale := math.Pow(2, float64(bitDepth-1))
	maxInt := scale - 1
	minInt := -scale

	ints := make([]int, len(buffer.Data))
	for i, sample := range buffer.Data {
		var value float64
		if sample < 0 {
			value = math.Round(sample * scale)
		} else {
			value = math.Round(sample * maxInt)
		}
		value = clamp(value, minInt, maxInt)
		ints[i] = int(value)
	}

	return &rawaudio.IntBuffer{
		Format: &rawaudio.Format{
			NumChannels: buffer.Channels,
			SampleRate:  buffer.SampleRate,
		},
		SourceBitDepth: bitDepth,
		Data:           ints,
	}, nil
}

func PCMToUnsigned8IntBuffer(buffer PCMBuffer) (*rawaudio.IntBuffer, error) {
	if err := buffer.Validate(); err != nil {
		return nil, err
	}

	ints := make([]int, len(buffer.Data))
	for i, sample := range buffer.Data {
		value := clamp(math.Round(sample*128.0+128.0), 0, 255)
		ints[i] = int(value)
	}

	return &rawaudio.IntBuffer{
		Format: &rawaudio.Format{
			NumChannels: buffer.Channels,
			SampleRate:  buffer.SampleRate,
		},
		SourceBitDepth: 8,
		Data:           ints,
	}, nil
}

func clamp[T ~float64](value, min, max T) T {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
```

4. Update `pkg/audio/aiff/codec.go` lines 36–45 to sign-extend 8-bit AIFF two's-complement bytes returned by `go-audio/aiff`:

```go
	intBuffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode aiff pcm buffer: %w", err)
	}

	bitDepth := int(decoder.SampleBitDepth())
	if bitDepth == 8 {
		for i, v := range intBuffer.Data {
			intBuffer.Data[i] = int(int8(uint8(v)))
		}
	}

	buffer, err := zaudio.IntBufferToPCM(intBuffer, bitDepth)
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode aiff pcm conversion: %w", err)
	}
```

5. Update `pkg/audio/wav/codec.go` to handle IEEE Float WAV (`Format == 3` and `WAVE_FORMAT_EXTENSIBLE == 0xFFFE`), extensible integer PCM (`0xFFFE` subformat `1`), and unsigned 8-bit WAV PCM:

```go
package wav

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	rawaudio "github.com/go-audio/audio"
	gowav "github.com/go-audio/wav"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/core"
)

const (
	waveFormatPCM        uint16 = 0x0001
	waveFormatIEEEFloat  uint16 = 0x0003
	waveFormatExtensible uint16 = 0xFFFE
)

var ksDataFormatSubtypeTail = [14]byte{
	0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71,
}

type wavFormatHeader struct {
	audioFormat   uint16
	effectiveFmt  uint16
	channels      uint16
	sampleRate    uint32
	blockAlign    uint16
	bitsPerSample uint16
	dataOffset    int64
	dataSize      uint32
}

type Codec struct{}

func New() Codec {
	return Codec{}
}

func (Codec) Format() core.AudioFormat {
	return core.FormatWAV
}

func (Codec) Decode(ctx context.Context, reader io.ReadSeeker) (zaudio.DecodeResult, error) {
	select {
	case <-ctx.Done():
		return zaudio.DecodeResult{}, ctx.Err()
	default:
	}

	hdr, err := parseWAVHeader(reader)
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav: %w", err)
	}

	if hdr.effectiveFmt == waveFormatIEEEFloat {
		return decodeIEEEFloatWAV(reader, hdr)
	}
	if hdr.effectiveFmt != waveFormatPCM {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav: unsupported wave format tag 0x%04x", hdr.effectiveFmt)
	}

	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav rewind: %w", err)
	}

	decoder := gowav.NewDecoder(reader)
	if !decoder.IsValidFile() {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav: invalid file")
	}

	intBuffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav pcm buffer: %w", err)
	}

	var buffer zaudio.PCMBuffer
	if intBuffer.SourceBitDepth == 8 {
		buffer, err = zaudio.Unsigned8IntBufferToPCM(intBuffer)
	} else {
		buffer, err = zaudio.IntBufferToPCM(intBuffer, intBuffer.SourceBitDepth)
	}
	if err != nil {
		return zaudio.DecodeResult{}, fmt.Errorf("decode wav pcm conversion: %w", err)
	}
	buffer.Metadata = map[string]string{"codec": "wav"}

	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatWAV,
			SampleRate: buffer.SampleRate,
			Channels:   buffer.Channels,
			BitDepth:   buffer.BitDepth,
			Frames:     buffer.Frames(),
		},
	}, nil
}

func parseWAVHeader(reader io.ReadSeeker) (wavFormatHeader, error) {
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return wavFormatHeader{}, err
	}

	var riffHdr [12]byte
	if _, err := io.ReadFull(reader, riffHdr[:]); err != nil {
		return wavFormatHeader{}, fmt.Errorf("invalid RIFF header: %w", err)
	}
	if string(riffHdr[0:4]) != "RIFF" || string(riffHdr[8:12]) != "WAVE" {
		return wavFormatHeader{}, errors.New("not a RIFF/WAVE stream")
	}

	var (
		hdr      wavFormatHeader
		foundFmt bool
		foundDat bool
	)
	for !foundFmt || !foundDat {
		var chunkHdr [8]byte
		if _, err := io.ReadFull(reader, chunkHdr[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return wavFormatHeader{}, fmt.Errorf("read RIFF chunk header: %w", err)
		}
		chunkID := string(chunkHdr[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHdr[4:8])
		chunkStart, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return wavFormatHeader{}, err
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return wavFormatHeader{}, fmt.Errorf("fmt chunk too small (%d bytes)", chunkSize)
			}
			fmtBytes := make([]byte, chunkSize)
			if _, err := io.ReadFull(reader, fmtBytes); err != nil {
				return wavFormatHeader{}, fmt.Errorf("read fmt chunk: %w", err)
			}
			hdr.audioFormat = binary.LittleEndian.Uint16(fmtBytes[0:2])
			hdr.effectiveFmt = hdr.audioFormat
			hdr.channels = binary.LittleEndian.Uint16(fmtBytes[2:4])
			hdr.sampleRate = binary.LittleEndian.Uint32(fmtBytes[4:8])
			hdr.blockAlign = binary.LittleEndian.Uint16(fmtBytes[12:14])
			hdr.bitsPerSample = binary.LittleEndian.Uint16(fmtBytes[14:16])

			if hdr.audioFormat == waveFormatExtensible {
				if len(fmtBytes) < 40 {
					return wavFormatHeader{}, fmt.Errorf("extensible fmt chunk too small (%d bytes)", len(fmtBytes))
				}
				var tail [14]byte
				copy(tail[:], fmtBytes[26:40])
				if tail != ksDataFormatSubtypeTail {
					return wavFormatHeader{}, errors.New("unsupported WAVE_FORMAT_EXTENSIBLE subformat GUID")
				}
				hdr.effectiveFmt = binary.LittleEndian.Uint16(fmtBytes[24:26])
			}
			foundFmt = true

		case "data":
			hdr.dataOffset = chunkStart
			hdr.dataSize = chunkSize
			foundDat = true
		}

		nextOffset := chunkStart + int64(chunkSize)
		if chunkSize%2 == 1 {
			nextOffset++
		}
		if !foundFmt || !foundDat {
			if _, err := reader.Seek(nextOffset, io.SeekStart); err != nil {
				break
			}
		}
	}

	if !foundFmt || !foundDat {
		return wavFormatHeader{}, errors.New("missing fmt or data chunk")
	}
	if hdr.channels == 0 || hdr.sampleRate == 0 || hdr.bitsPerSample < 8 {
		return wavFormatHeader{}, errors.New("invalid wav stream parameters")
	}

	return hdr, nil
}

func decodeIEEEFloatWAV(reader io.ReadSeeker, hdr wavFormatHeader) (zaudio.DecodeResult, error) {
	if hdr.bitsPerSample != 32 && hdr.bitsPerSample != 64 {
		return zaudio.DecodeResult{}, fmt.Errorf("decode ieee float wav: unsupported bit depth %d", hdr.bitsPerSample)
	}
	if _, err := reader.Seek(hdr.dataOffset, io.SeekStart); err != nil {
		return zaudio.DecodeResult{}, err
	}

	bytesPerSample := int(hdr.bitsPerSample / 8)
	frameBytes := bytesPerSample * int(hdr.channels)
	numFrames := int(hdr.dataSize) / frameBytes
	if numFrames <= 0 {
		return zaudio.DecodeResult{}, errors.New("decode ieee float wav: empty data chunk")
	}

	raw := make([]byte, numFrames*frameBytes)
	n, err := io.ReadFull(reader, raw)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return zaudio.DecodeResult{}, fmt.Errorf("read ieee float wav data: %w", err)
	}
	usableFrames := n / frameBytes
	if usableFrames <= 0 {
		return zaudio.DecodeResult{}, errors.New("decode ieee float wav: truncated data chunk")
	}
	raw = raw[:usableFrames*frameBytes]

	totalSamples := usableFrames * int(hdr.channels)
	data := make([]float64, totalSamples)
	for i := range totalSamples {
		offset := i * bytesPerSample
		var v float64
		if bytesPerSample == 4 {
			v = float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[offset : offset+4])))
		} else {
			v = math.Float64frombits(binary.LittleEndian.Uint64(raw[offset : offset+8]))
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return zaudio.DecodeResult{}, fmt.Errorf("decode ieee float wav: non-finite sample at index %d", i)
		}
		if v < -1.0 {
			v = -1.0
		} else if v > 1.0 {
			v = 1.0
		}
		data[i] = v
	}

	buffer := zaudio.PCMBuffer{
		SampleRate: int(hdr.sampleRate),
		Channels:   int(hdr.channels),
		BitDepth:   int(hdr.bitsPerSample),
		Data:       data,
		Metadata:   map[string]string{"codec": "wav"},
	}
	return zaudio.DecodeResult{
		Buffer: buffer,
		Info: zaudio.StreamInfo{
			Format:     core.FormatWAV,
			SampleRate: buffer.SampleRate,
			Channels:   buffer.Channels,
			BitDepth:   buffer.BitDepth,
			Frames:     buffer.Frames(),
		},
	}, nil
}

func (Codec) Encode(ctx context.Context, writer io.WriteSeeker, buffer zaudio.PCMBuffer) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := buffer.Validate(); err != nil {
		return fmt.Errorf("encode wav: %w", err)
	}

	var (
		intBuffer *rawaudio.IntBuffer
		err       error
	)
	if buffer.BitDepth == 8 {
		intBuffer, err = zaudio.PCMToUnsigned8IntBuffer(buffer)
	} else {
		intBuffer, err = zaudio.PCMToIntBuffer(buffer)
	}
	if err != nil {
		return fmt.Errorf("encode wav: %w", err)
	}

	encoder := gowav.NewEncoder(writer, buffer.SampleRate, intBuffer.SourceBitDepth, buffer.Channels, 1)
	if err := encoder.Write(intBuffer); err != nil {
		return fmt.Errorf("encode wav buffer: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finalize wav file: %w", err)
	}

	return nil
}

func Register(registry *zaudio.Registry) error {
	codec := New()
	if err := registry.RegisterDecoder(codec); err != nil {
		return err
	}
	if err := registry.RegisterEncoder(codec); err != nil {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run audio and analysis test suites + linter to verify Task 4**

Run:
```bash
go test ./pkg/audio/... ./pkg/analysis/... ./pkg/convert/... -v
golangci-lint run ./pkg/audio/...
```
Expected: All tests PASS and `golangci-lint` reports `0 issues`.

- [ ] **Step 5: Commit Task 4**

```bash
git add pkg/audio/buffer.go pkg/audio/buffer_test.go pkg/audio/ogg/codec.go pkg/audio/ogg/codec_test.go pkg/audio/pcm.go pkg/audio/aiff/codec.go pkg/audio/aiff/codec_test.go pkg/audio/wav/codec.go pkg/audio/wav/codec_test.go
git commit -m "fix(audio): reject NaN/Inf, clamp OGG overshoot, fix 8-bit PCM, and decode IEEE float WAV"
```

---

### Task 5: Fix Worker Goroutine Leaks & Skip Undecodable Samples in Batch Pipelines

**Files:**
- Modify: `pkg/scanner/scanner.go:76-149`
- Modify: `pkg/scanner/scanner_test.go`
- Modify: `pkg/dedup/exact.go:59-139`
- Modify: `pkg/dedup/exact_test.go`
- Modify: `pkg/audio/decoder.go`
- Create: `pkg/audio/decoder_test.go`
- Modify: `cmd/zounds/commands/analyze.go:49-74`
- Modify: `cmd/zounds/commands/analyze_test.go`
- Modify: `cmd/zounds/commands/tag.go:87-216`
- Modify: `cmd/zounds/commands/tag_test.go`
- Modify: `cmd/zounds/commands/dedup.go:59-116`
- Modify: `cmd/zounds/commands/dedup_test.go`

- [ ] **Step 1: Write failing tests for goroutine leak teardown, `audio.IsDecodeError`, and batch undecodable skipping (`analyze --all`, `tag --auto`, `dedup --perceptual`)**

1. Append to `pkg/scanner/scanner_test.go` (adding `"fmt"`, `"runtime"`, `"time"` to imports):

```go
func TestScanDoesNotLeakWorkerGoroutinesOnEarlyError(t *testing.T) {
	validRoot := t.TempDir()
	for i := range 32 {
		mustWriteFile(t, filepath.Join(validRoot, fmt.Sprintf("sample_%02d.wav", i)))
	}
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")

	runtime.GC()
	before := runtime.NumGoroutine()

	s := scanner.New(scanner.Options{Workers: 4})
	if _, err := s.Scan(context.Background(), validRoot, missingRoot); err == nil {
		t.Fatal("expected error for missing second scan root")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= before+1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutine leak after failed Scan: before=%d after=%d", before, runtime.NumGoroutine())
}
```

2. Append to `pkg/dedup/exact_test.go` (adding `"fmt"`, `"runtime"` to imports):

```go
func TestExactFinderDoesNotLeakGoroutinesOnMissingFileError(t *testing.T) {
	root := t.TempDir()
	samples := make([]core.Sample, 0, 24)
	// First sample points to a missing file of size 4; remaining samples are real files of size 4.
	samples = append(samples, core.Sample{
		Path:      filepath.Join(root, "missing.wav"),
		SizeBytes: 4,
	})
	for i := range 23 {
		p := filepath.Join(root, fmt.Sprintf("dup_%02d.wav", i))
		writeFile(t, p, []byte("same"))
		samples = append(samples, sampleFromPath(t, p))
	}

	runtime.GC()
	before := runtime.NumGoroutine()

	finder := dedup.NewExactFinder(4)
	if _, err := finder.Find(context.Background(), samples); err == nil {
		t.Fatal("expected error when hashing missing file")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= before+1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("goroutine leak after failed ExactFinder.Find: before=%d after=%d", before, runtime.NumGoroutine())
}
```

3. Create `pkg/audio/decoder_test.go`:

```go
package audio_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	zaudio "github.com/darkliquid/zounds/pkg/audio"
	"github.com/darkliquid/zounds/pkg/audio/codecs"
)

func TestDecodeFileClassifiesUnsupportedAndCorruptFilesButNotCanceledContext(t *testing.T) {
	t.Parallel()

	registry, err := codecs.NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	dir := t.TempDir()
	m4aPath := filepath.Join(dir, "clip.m4a")
	if err := os.WriteFile(m4aPath, []byte("fake-m4a"), 0o644); err != nil {
		t.Fatalf("write m4a: %v", err)
	}

	_, err = zaudio.DecodeFile(context.Background(), registry, m4aPath)
	if !errors.Is(err, zaudio.ErrUnsupportedFormat) || !zaudio.IsDecodeError(err) {
		t.Fatalf("expected IsDecodeError + ErrUnsupportedFormat for .m4a, got: %v", err)
	}

	corruptWav := filepath.Join(dir, "corrupt.wav")
	if err := os.WriteFile(corruptWav, []byte("not-a-riff-wave"), 0o644); err != nil {
		t.Fatalf("write corrupt wav: %v", err)
	}
	_, err = zaudio.DecodeFile(context.Background(), registry, corruptWav)
	if !zaudio.IsDecodeError(err) {
		t.Fatalf("expected IsDecodeError for corrupt .wav, got: %v", err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = zaudio.DecodeFile(canceledCtx, registry, corruptWav)
	if zaudio.IsDecodeError(err) {
		t.Fatalf("expected canceled context error NOT to be classified as DecodeError, got: %v", err)
	}
}
```

4. Append batch-skip CLI tests to `cmd/zounds/commands/analyze_test.go` (adding `"os"` to imports), `cmd/zounds/commands/tag_test.go`, and `cmd/zounds/commands/dedup_test.go`:

In `cmd/zounds/commands/analyze_test.go`:

```go
func TestAnalyzeCommandAllSkipsUndecodableSamplesWithWarning(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "analyze-skip.db")
	validWav := filepath.Join(root, "valid.wav")
	unsupportedM4A := filepath.Join(root, "unsupported.m4a")
	corruptWav := filepath.Join(root, "corrupt.wav")

	writeWAVFixture(t, validWav)
	if err := os.WriteFile(unsupportedM4A, []byte("fake-m4a"), 0o644); err != nil {
		t.Fatalf("write m4a: %v", err)
	}
	if err := os.WriteFile(corruptWav, []byte("corrupt-riff"), 0o644); err != nil {
		t.Fatalf("write corrupt wav: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	analyze := commands.NewRootCommand()
	var stdout, stderr bytes.Buffer
	analyze.SetOut(&stdout)
	analyze.SetErr(&stderr)
	analyze.SetArgs([]string{"--db", dbPath, "analyze", "--all"})
	if err := analyze.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("expected analyze --all to succeed while skipping undecodables, got: %v", err)
	}

	if !strings.Contains(stdout.String(), "analyzed 1 samples") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	for _, skipped := range []string{unsupportedM4A, corruptWav} {
		if !strings.Contains(stderr.String(), skipped) {
			t.Fatalf("expected warning mentioning %q in stderr %q", skipped, stderr.String())
		}
	}
}
```

In `cmd/zounds/commands/tag_test.go`:

```go
func TestTagCommandAutoSkipsUndecodableSamplesInBatch(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "tag-skip.db")
	rulePath := filepath.Join(t.TempDir(), "rules.json")
	validWav := filepath.Join(root, "tone.wav")
	unsupportedAAC := filepath.Join(root, "voice.aac")

	writeSineWAVFixture(t, validWav, 55, 1.0)
	if err := os.WriteFile(unsupportedAAC, []byte("fake-aac"), 0o644); err != nil {
		t.Fatalf("write aac: %v", err)
	}
	if err := os.WriteFile(rulePath, []byte(`{"rules":[{"tag":"instrument:sub","expr":"Metrics[\"frequency_hz\"] < 80"}]}`), 0o644); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	tagCmd := commands.NewRootCommand()
	var stdout, stderr bytes.Buffer
	tagCmd.SetOut(&stdout)
	tagCmd.SetErr(&stderr)
	tagCmd.SetArgs([]string{"--db", dbPath, "tag", "--auto", "--rule-file", rulePath})
	if err := tagCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("expected tag --auto batch to skip undecodable .aac, got: %v", err)
	}

	if !strings.Contains(stdout.String(), "processed 1 samples and applied 1 tags") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), unsupportedAAC) {
		t.Fatalf("expected warning for %q in stderr %q", unsupportedAAC, stderr.String())
	}
}
```

In `cmd/zounds/commands/dedup_test.go`:

```go
func TestDedupCommandPerceptualSkipsUndecodableSamplesWithWarning(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "dedup-perceptual-skip.db")
	one := filepath.Join(root, "one.wav")
	two := filepath.Join(root, "two.wav")
	badM4A := filepath.Join(root, "clip.m4a")

	writeSineWAVFixture(t, one, 220, 1.2)
	writeSineWAVFixture(t, two, 220, 1.2)
	if err := os.WriteFile(badM4A, []byte("fake-m4a"), 0o644); err != nil {
		t.Fatalf("write m4a: %v", err)
	}

	scan := commands.NewRootCommand()
	scan.SetArgs([]string{"--db", dbPath, "scan", root})
	if err := scan.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute scan: %v", err)
	}

	cmd := commands.NewRootCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"--db", dbPath, "--dry-run", "dedup", "--perceptual"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("expected dedup --perceptual to skip .m4a, got: %v", err)
	}

	if !strings.Contains(stdout.String(), one) || !strings.Contains(stdout.String(), two) {
		t.Fatalf("expected perceptual duplicates in stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), badM4A) {
		t.Fatalf("expected warning for %q in stderr: %q", badM4A, stderr.String())
	}
}
```

- [ ] **Step 2: Run targeted tests to confirm failures**

Run:
```bash
go test ./pkg/scanner/... ./pkg/dedup/... ./pkg/audio/... ./cmd/zounds/commands -run 'TestScanDoesNotLeak|TestExactFinderDoesNotLeak|TestDecodeFileClassifies|TestAnalyzeCommandAllSkips|TestTagCommandAutoSkipsUndecodable|TestDedupCommandPerceptualSkips' -v
```
Expected: FAIL (`zaudio.ErrUnsupportedFormat` / `zaudio.IsDecodeError` undefined, goroutine leak assertions fail, batch commands abort on `.m4a`/`.aac`).

- [ ] **Step 3: Fix worker goroutine cancellation/teardown in `pkg/scanner/scanner.go` and `pkg/dedup/exact.go`**

In `pkg/scanner/scanner.go`, update `Scan` (lines 76–149):

```go
func (s *Scanner) Scan(ctx context.Context, roots ...string) ([]core.Sample, error) {
	if len(roots) == 0 {
		return nil, errors.New("scan requires at least one root")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.logf("starting scan with %d root(s) and %d worker(s)", len(roots), s.workers)

	candidates := make(chan candidate)
	results := make(chan core.Sample)
	errCh := make(chan error, 1)

	var workerWG sync.WaitGroup
	for i := 0; i < s.workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-candidates:
					if !ok {
						return
					}
					sample, err := s.statCandidate(item.root, item.path)
					if err != nil {
						select {
						case errCh <- err:
						default:
						}
						cancel()
						return
					}

					select {
					case <-ctx.Done():
						return
					case results <- sample:
					}
				}
			}
		}()
	}

	go func() {
		workerWG.Wait()
		close(results)
	}()

	var walkWG sync.WaitGroup
	walkWG.Add(1)
	go func() {
		defer walkWG.Done()
		defer close(candidates)
		for _, root := range roots {
			if err := s.walkRoot(ctx, root, candidates); err != nil {
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					select {
					case errCh <- err:
					default:
					}
					cancel()
				}
				return
			}
		}
	}()

	shutdown := func() {
		cancel()
		for range results {
		}
		walkWG.Wait()
	}

	var samples []core.Sample
	for {
		select {
		case <-ctx.Done():
			select {
			case err := <-errCh:
				shutdown()
				return nil, err
			default:
			}
			shutdown()
			return nil, ctx.Err()
		case err := <-errCh:
			if err != nil {
				shutdown()
				return nil, err
			}
		case sample, ok := <-results:
			if !ok {
				walkWG.Wait()
				select {
				case err := <-errCh:
					if err != nil {
						return nil, err
					}
				default:
				}
				sort.Slice(samples, func(i, j int) bool {
					return samples[i].Path < samples[j].Path
				})
				s.logf("scan complete: discovered %d audio file(s)", len(samples))
				return samples, nil
			}
			samples = append(samples, sample)
		}
	}
}
```

In `pkg/dedup/exact.go`, add `"errors"` to imports and update `ExactFinder.Find` (lines 59–139):

```go
func (f ExactFinder) Find(ctx context.Context, samples []core.Sample) ([]DuplicateGroup, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sizeBuckets := make(map[int64][]core.Sample)
	for _, sample := range samples {
		sizeBuckets[sample.SizeBytes] = append(sizeBuckets[sample.SizeBytes], sample)
	}

	type job struct {
		sample core.Sample
	}

	jobs := make(chan job)
	results := make(chan FileHash)
	errCh := make(chan error, 1)

	workers := f.Workers
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-jobs:
					if !ok {
						return
					}
					if f.Logger != nil {
						f.Logger.Printf("hashing file %s", item.sample.Path)
					}
					hash, err := hashFile(ctx, item.sample.Path)
					if err != nil {
						if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
							select {
							case errCh <- fmt.Errorf("hash %q: %w", item.sample.Path, err):
							default:
							}
							cancel()
						}
						return
					}

					select {
					case <-ctx.Done():
						return
					case results <- FileHash{Sample: item.sample, SHA256: hash}:
					}
				}
			}
		}()
	}

	var feedWG sync.WaitGroup
	feedWG.Add(1)
	go func() {
		defer feedWG.Done()
		defer close(jobs)
		for _, bucket := range sizeBuckets {
			if len(bucket) < 2 {
				continue
			}
			for _, sample := range bucket {
				select {
				case <-ctx.Done():
					return
				case jobs <- job{sample: sample}:
				}
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	shutdown := func() {
		cancel()
		for range results {
		}
		feedWG.Wait()
	}

	grouped := map[string][]core.Sample{}
	sizeByHash := map[string]int64{}
	for {
		select {
		case <-ctx.Done():
			select {
			case err := <-errCh:
				shutdown()
				return nil, err
			default:
			}
			shutdown()
			return nil, ctx.Err()
		case err := <-errCh:
			if err != nil {
				shutdown()
				return nil, err
			}
		case result, ok := <-results:
			if !ok {
				feedWG.Wait()
				select {
				case err := <-errCh:
					if err != nil {
						return nil, err
					}
				default:
				}
				return buildDuplicateGroups(grouped, sizeByHash), nil
			}
			grouped[result.SHA256] = append(grouped[result.SHA256], result.Sample)
			sizeByHash[result.SHA256] = result.Sample.SizeBytes
		}
	}
}
```

- [ ] **Step 4: Add `ErrUnsupportedFormat`, `DecodeError`, and `IsDecodeError` to `pkg/audio/decoder.go` and skip undecodables with warnings in batch CLI pipelines**

1. Update `pkg/audio/decoder.go`:

```go
package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/darkliquid/zounds/pkg/core"
)

var ErrUnsupportedFormat = errors.New("unsupported audio format")

type DecodeError struct {
	Path   string
	Format core.AudioFormat
	Err    error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("decode %q (%s): %v", e.Path, e.Format, e.Err)
}

func (e *DecodeError) Unwrap() error {
	return e.Err
}

func IsDecodeError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var target *DecodeError
	return errors.As(err, &target)
}

type StreamInfo struct {
	Format     core.AudioFormat
	SampleRate int
	Channels   int
	BitDepth   int
	Bitrate    int
	Frames     int
}

type DecodeResult struct {
	Buffer PCMBuffer
	Info   StreamInfo
}

type Decoder interface {
	Format() core.AudioFormat
	Decode(ctx context.Context, reader io.ReadSeeker) (DecodeResult, error)
}

func DecodeFile(ctx context.Context, registry *Registry, path string) (DecodeResult, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return DecodeResult{}, err
		}
	}
	if registry == nil {
		return DecodeResult{}, fmt.Errorf("decode %q: nil registry", path)
	}

	format := core.DetectFormatFromExtension(path)
	decoder, ok := registry.Decoder(format)
	if !ok {
		return DecodeResult{}, &DecodeError{
			Path:   path,
			Format: format,
			Err:    fmt.Errorf("%w: no decoder registered for %s", ErrUnsupportedFormat, format),
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return DecodeResult{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	decoded, err := decoder.Decode(ctx, file)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return DecodeResult{}, err
		}
		return DecodeResult{}, &DecodeError{
			Path:   path,
			Format: format,
			Err:    err,
		}
	}
	return decoded, nil
}
```

2. Update `cmd/zounds/commands/analyze.go` lines 51–70 (`analyze --all` skips `zaudio.IsDecodeError(err)` with warning on stderr; `analyze --id` still returns the error):

```go
			processed := 0
			for _, sample := range samples {
				verbosef(logger, "analyzing sample %s", sample.Path)
				results, vector, err := analyzeSample(ctx, sample, builder)
				if err != nil {
					if analyzeAll && zaudio.IsDecodeError(err) {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipping undecodable sample %s: %v\n", sample.Path, err)
						continue
					}
					return err
				}
				_ = results
```

3. Update `cmd/zounds/commands/tag.go` (`runAutoTagging`): add `zaudio "github.com/darkliquid/zounds/pkg/audio"` and `"github.com/darkliquid/zounds/pkg/audio/codecs"` to imports, pre-decode once per sample with `analysis.ContextWithDecodedSample`, and when `strings.TrimSpace(targetPath) == ""` (batch mode), warn and skip on `zaudio.IsDecodeError(err)`:

```go
	registry, err := codecs.NewRegistry()
	if err != nil {
		return fmt.Errorf("create codec registry: %w", err)
	}

	analyzer, err := analysis.NewMetadataAnalyzer(registry)
	if err != nil {
		return err
	}
	spectralAnalyzer, err := analysis.NewSpectralAnalyzer(registry)
	if err != nil {
		return err
	}
	dynamicsAnalyzer, err := analysis.NewDynamicsAnalyzer(registry)
	if err != nil {
		return err
	}
	pitchAnalyzer, err := analysis.NewPitchAnalyzer(registry)
	if err != nil {
		return err
	}
	keyAnalyzer, err := analysis.NewKeyAnalyzer(registry)
	if err != nil {
		return err
	}
	harmonicsAnalyzer, err := analysis.NewHarmonicsAnalyzer(registry)
	if err != nil {
		return err
	}
```

and inside the `for _, sample := range samples` loop in `runAutoTagging`:

```go
	isBatch := strings.TrimSpace(targetPath) == ""
	processed := 0
	applied := 0
	for _, sample := range samples {
		verbosef(logger, "tagging sample %s", sample.Path)
		decoded, err := zaudio.DecodeFile(ctx, registry, sample.Path)
		if err != nil {
			if isBatch && zaudio.IsDecodeError(err) {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipping undecodable sample %s: %v\n", sample.Path, err)
				continue
			}
			return err
		}
		sampleCtx := analysis.ContextWithDecodedSample(ctx, sample.Path, decoded)

		results := make([]core.AnalysisResult, 0, 6)
		for _, a := range []core.Analyzer{
			analyzer,
			spectralAnalyzer,
			dynamicsAnalyzer,
			pitchAnalyzer,
			keyAnalyzer,
			harmonicsAnalyzer,
		} {
			res, analyzeErr := a.Analyze(sampleCtx, sample)
			if analyzeErr != nil {
				return analyzeErr
			}
			results = append(results, res)
		}

		combined := combineAnalysisResults(sample.ID, results...)

		var generated []core.Tag
		metadataTags, err := metadataTagger.Tags(sampleCtx, sample, combined)
		if err != nil {
			return err
		}
		ruleTags, err := ruleTagger.Tags(sampleCtx, sample, combined)
		if err != nil {
			return err
		}
		generated = append(generated, metadataTags...)
		generated = append(generated, ruleTags...)
		if clapTagger != nil {
			clapTags, err := clapTagger.Tags(sampleCtx, sample, combined)
			if err != nil {
				return err
			}
			generated = append(generated, clapTags...)
		}
		generated = dedupeGeneratedTags(generated)
		verbosef(logger, "generated %d tags for %s: %s", len(generated), sample.Path, strings.Join(tagNames(generated), ", "))
		processed++

		if cfg.DryRun {
			applied += len(generated)
			verbosef(logger, "skipping tag persistence for %s (dry run)", sample.Path)
			continue
		}

		for _, tag := range generated {
			verbosef(logger, "attaching tag %s to %s", normalizedTagName(tag), sample.Path)
			tagID, err := repo.EnsureTag(ctx, tag)
			if err != nil {
				return err
			}
			if err := repo.AttachTagToSample(ctx, sample.ID, tagID); err != nil {
				return err
			}
			applied++
		}
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "processed %d samples and applied %d tags\n", processed, applied)
	return err
```

4. Update `cmd/zounds/commands/dedup.go` (add `"io"` and `zaudio "github.com/darkliquid/zounds/pkg/audio"` to imports) so `buildPerceptualHashes` warns to `cmd.ErrOrStderr()` and skips `zaudio.IsDecodeError(err)`:

```go
			} else {
				hashes, err := buildPerceptualHashes(ctx, samples, cmd.ErrOrStderr(), logger)
				if err != nil {
					return err
				}
				groups, err := dedup.NewPerceptualFinder(threshold).Find(hashes)
				if err != nil {
					return err
				}
				actions = dedup.PlanPerceptualCull(groups, keepStrategy)
			}
```

and:

```go
func buildPerceptualHashes(ctx context.Context, samples []core.Sample, errOut io.Writer, logger *log.Logger) ([]dedup.PerceptualHash, error) {
	analyzer, err := analysis.NewPerceptualHashAnalyzer(nil)
	if err != nil {
		return nil, err
	}

	hashes := make([]dedup.PerceptualHash, 0, len(samples))
	for _, sample := range samples {
		verbosef(logger, "building perceptual hash for %s", sample.Path)
		result, err := analyzer.Analyze(ctx, sample)
		if err != nil {
			if zaudio.IsDecodeError(err) {
				_, _ = fmt.Fprintf(errOut, "warning: skipping undecodable sample %s: %v\n", sample.Path, err)
				continue
			}
			return nil, err
		}
		hashes = append(hashes, dedup.PerceptualHash{
			Sample: sample,
			Hash:   result.Attributes["perceptual_hash"],
		})
	}

	return hashes, nil
}
```

- [ ] **Step 5: Run the full mandatory project verification suite (`AGENTS.md` §5.6)**

Run:
```bash
go test ./...
CGO_ENABLED=0 go build ./...
golangci-lint run
git status
```
Expected:
1. `go test ./...` PASSES across all packages with 0 failures.
2. `CGO_ENABLED=0 go build ./...` SUCCEEDS with 0 errors.
3. `golangci-lint run` exits cleanly with `0 issues`.
4. `git status` confirms no binary `.onnx`, `onnxruntime.so`, `.db`, or compiled binaries are staged.

- [ ] **Step 6: Commit Task 5**

```bash
git add pkg/scanner/scanner.go pkg/scanner/scanner_test.go pkg/dedup/exact.go pkg/dedup/exact_test.go pkg/audio/decoder.go pkg/audio/decoder_test.go cmd/zounds/commands/analyze.go cmd/zounds/commands/analyze_test.go cmd/zounds/commands/tag.go cmd/zounds/commands/tag_test.go cmd/zounds/commands/dedup.go cmd/zounds/commands/dedup_test.go
git commit -m "fix(pipeline): prevent worker goroutine leaks and skip undecodable samples in batch runs"
```
