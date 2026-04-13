package clap

import (
	"fmt"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// DefaultEmbedDim is the CLAP audio/text embedding size for
// laion/clap-htsat-unfused and laion/clap-htsat-fused.
const DefaultEmbedDim = 512

var (
	ortOnce       sync.Once
	ortErr        error
	ortConfigMu   sync.Mutex
	ortLibPath    string
	ortLibPathSet bool
)

// ensureORT initialises the ONNX Runtime environment exactly once.
// libPath is the path to the onnxruntime shared library; an empty string uses
// the platform default ("onnxruntime.so" on Linux).
func ensureORT(libPath string) error {
	ortConfigMu.Lock()
	if ortLibPathSet {
		if ortLibPath != libPath {
			ortConfigMu.Unlock()
			return fmt.Errorf("onnxruntime already initialised with libPath %q, cannot reinitialise with %q", ortLibPath, libPath)
		}
	} else {
		ortLibPath = libPath
		ortLibPathSet = true
	}
	selectedLibPath := ortLibPath
	ortConfigMu.Unlock()

	ortOnce.Do(func() {
		if selectedLibPath != "" {
			ort.SetSharedLibraryPath(selectedLibPath)
		}
		ortErr = ort.InitializeEnvironment()
	})
	return ortErr
}

// audioEncoder wraps an AdvancedSession for the CLAP audio encoder.
// Input:  input_features [1, 1, numMelBins, numFrames] float32
// Output: audio_embeds   [1, embedDim]               float32
type audioEncoder struct {
	session   *ort.AdvancedSession
	inputBuf  []float32
	outputBuf []float32
	inTensor  *ort.Tensor[float32]
	outTensor *ort.Tensor[float32]
	mu        sync.Mutex
}

func newAudioEncoder(modelPath string, numMelBins, numFrames, embedDim int, inputName, outputName string) (*audioEncoder, error) {
	inputBuf := make([]float32, numMelBins*numFrames)
	outputBuf := make([]float32, embedDim)

	inTensor, err := ort.NewTensor(ort.NewShape(1, 1, int64(numMelBins), int64(numFrames)), inputBuf)
	if err != nil {
		return nil, fmt.Errorf("create audio input tensor: %w", err)
	}

	outTensor, err := ort.NewTensor(ort.NewShape(1, int64(embedDim)), outputBuf)
	if err != nil {
		_ = inTensor.Destroy()
		return nil, fmt.Errorf("create audio output tensor: %w", err)
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{inputName},
		[]string{outputName},
		[]ort.Value{inTensor},
		[]ort.Value{outTensor},
		nil,
	)
	if err != nil {
		_ = inTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("create audio encoder session from %q: %w", modelPath, err)
	}

	return &audioEncoder{
		session:   session,
		inputBuf:  inTensor.GetData(),
		outputBuf: outTensor.GetData(),
		inTensor:  inTensor,
		outTensor: outTensor,
	}, nil
}

// encode copies features into the input tensor, runs inference, and returns
// the embedding.  features must have length numMelBins*numFrames.
func (e *audioEncoder) encode(features []float32) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(features) != len(e.inputBuf) {
		return nil, fmt.Errorf("audio encode: got %d features, want %d", len(features), len(e.inputBuf))
	}
	copy(e.inputBuf, features)

	if err := e.session.Run(); err != nil {
		return nil, fmt.Errorf("audio encoder run: %w", err)
	}

	out := make([]float32, len(e.outputBuf))
	copy(out, e.outputBuf)
	return out, nil
}

func (e *audioEncoder) close() error {
	var firstErr error

	if e.session != nil {
		if err := e.session.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if e.inTensor != nil {
		if err := e.inTensor.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if e.outTensor != nil {
		if err := e.outTensor.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// textEncoder wraps an AdvancedSession for the CLAP text encoder.
// Inputs:  input_ids [1, maxTokens] int64, attention_mask [1, maxTokens] int64
// Output:  text_embeds [1, embedDim] float32
type textEncoder struct {
	session   *ort.AdvancedSession
	idsBuf    []int64
	maskBuf   []int64
	outputBuf []float32
	idsTensor  *ort.Tensor[int64]
	maskTensor *ort.Tensor[int64]
	outTensor  *ort.Tensor[float32]
	mu         sync.Mutex
}

func newTextEncoder(modelPath string, maxTokens, embedDim int, inputIDsName, attentionMaskName, outputName string) (*textEncoder, error) {
	idsBuf := make([]int64, maxTokens)
	maskBuf := make([]int64, maxTokens)
	outputBuf := make([]float32, embedDim)

	idsTensor, err := ort.NewTensor(ort.NewShape(1, int64(maxTokens)), idsBuf)
	if err != nil {
		return nil, fmt.Errorf("create ids tensor: %w", err)
	}
	maskTensor, err := ort.NewTensor(ort.NewShape(1, int64(maxTokens)), maskBuf)
	if err != nil {
		_ = idsTensor.Destroy()
		return nil, fmt.Errorf("create mask tensor: %w", err)
	}
	outTensor, err := ort.NewTensor(ort.NewShape(1, int64(embedDim)), outputBuf)
	if err != nil {
		_ = idsTensor.Destroy()
		_ = maskTensor.Destroy()
		return nil, fmt.Errorf("create text output tensor: %w", err)
	}

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{inputIDsName, attentionMaskName},
		[]string{outputName},
		[]ort.Value{idsTensor, maskTensor},
		[]ort.Value{outTensor},
		nil,
	)
	if err != nil {
		_ = idsTensor.Destroy()
		_ = maskTensor.Destroy()
		_ = outTensor.Destroy()
		return nil, fmt.Errorf("create text encoder session from %q: %w", modelPath, err)
	}

	return &textEncoder{
		session:    session,
		idsBuf:     idsTensor.GetData(),
		maskBuf:    maskTensor.GetData(),
		outputBuf:  outTensor.GetData(),
		idsTensor:  idsTensor,
		maskTensor: maskTensor,
		outTensor:  outTensor,
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

	if err := e.session.Run(); err != nil {
		return nil, fmt.Errorf("text encoder run: %w", err)
	}

	out := make([]float32, len(e.outputBuf))
	copy(out, e.outputBuf)
	return out, nil
}

func (e *textEncoder) close() error {
	var firstErr error

	if e.session != nil {
		if err := e.session.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if e.idsTensor != nil {
		if err := e.idsTensor.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if e.maskTensor != nil {
		if err := e.maskTensor.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if e.outTensor != nil {
		if err := e.outTensor.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}
