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
	rt             *ortRuntime
	session        uintptr
	inputCStrings  [][]byte
	inputPtrs      []*byte
	inputValues    []uintptr
	outputCStrings [][]byte
	outputPtrs     []*byte
	outputValues   []uintptr
	tensors        []*ortTensor
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
