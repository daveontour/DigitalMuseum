package main

// ArcFace face recognition (embedding) model.
//
// Model: buffalo_l's recognition/model.onnx (w600k_r50, ResNet50@WebFace600K).
// Its actual input/output tensor names and shapes were read directly off the
// bundled model file with onnxruntime_go's GetInputOutputInfo before writing
// this file, not assumed:
//
//	input:  "input.1"  [-1, 3, 112, 112]  NCHW float32 (dynamic batch — we
//	                                       always run batch size 1)
//	output: "683"      [1, 512]           the face embedding
//
// 512 matches internal/database/migrate.go's face_embeddings vec0 table
// dimension exactly, and the reference preprocessing
// ((pixel-127.5)/127.5, RGB order — insightface's ArcFaceONNX reads BGR via
// OpenCV and swaps to RGB before this formula; Go's image package already
// decodes to RGB, so no channel swap is needed here) is applied in Embed.
import (
	"fmt"
	"image"

	ort "github.com/yalue/onnxruntime_go"
)

type recognizer struct {
	session *ort.DynamicAdvancedSession
}

func newRecognizer(modelPath string) (*recognizer, error) {
	session, err := ort.NewDynamicAdvancedSession(modelPath, []string{"input.1"}, []string{"683"}, nil)
	if err != nil {
		return nil, fmt.Errorf("create recognition session: %w", err)
	}
	return &recognizer{session: session}, nil
}

func (r *recognizer) Close() {
	if r.session != nil {
		_ = r.session.Destroy()
	}
}

// Embed returns the 512-d ArcFace embedding for a 112x112 aligned face crop
// (see alignFace). The embedding is NOT normalized here — that happens on
// the Go server side (see NormalizeFaceEmbedding in
// internal/service/face_embedding.go), so this binary's output is exactly
// what the model produced, keeping normalization policy in one place.
func (r *recognizer) Embed(aligned *image.RGBA) ([]float32, error) {
	if aligned.Bounds().Dx() != alignedSize || aligned.Bounds().Dy() != alignedSize {
		return nil, fmt.Errorf("aligned face must be %dx%d, got %dx%d",
			alignedSize, alignedSize, aligned.Bounds().Dx(), aligned.Bounds().Dy())
	}

	const planeSize = alignedSize * alignedSize
	data := make([]float32, 3*planeSize)
	i := 0
	for y := 0; y < alignedSize; y++ {
		for x := 0; x < alignedSize; x++ {
			px := aligned.RGBAAt(x, y)
			data[0*planeSize+i] = (float32(px.R) - 127.5) / 127.5
			data[1*planeSize+i] = (float32(px.G) - 127.5) / 127.5
			data[2*planeSize+i] = (float32(px.B) - 127.5) / 127.5
			i++
		}
	}

	inputTensor, err := ort.NewTensor(ort.NewShape(1, 3, alignedSize, alignedSize), data)
	if err != nil {
		return nil, fmt.Errorf("build input tensor: %w", err)
	}
	defer func() { _ = inputTensor.Destroy() }()

	outputs := make([]ort.Value, 1)
	if err := r.session.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return nil, fmt.Errorf("run recognition session: %w", err)
	}
	defer func() {
		if outputs[0] != nil {
			_ = outputs[0].Destroy()
		}
	}()

	embT, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("unexpected output tensor type")
	}
	// Copy out of the tensor's own backing slice, which is invalidated by Destroy above.
	return append([]float32(nil), embT.GetData()...), nil
}
