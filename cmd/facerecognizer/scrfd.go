package main

// SCRFD face detector.
//
// Model: buffalo_l's det_10g.onnx (SCRFD-10GF, "bnkps" variant — 5-point
// landmarks). Its actual input/output tensor names and shapes were read
// directly off the bundled model file with onnxruntime_go's
// GetInputOutputInfo before writing this file, not assumed:
//
//	input:  "input.1"  [1, 3, -1, -1]  NCHW float32 (dynamic H/W — we always
//	                                    feed exactly 640x640, see letterbox640)
//	outputs (9 tensors, stride 8/16/32, 2 anchors per spatial location):
//	  score  stride  8: "448" [12800, 1]
//	  score  stride 16: "471" [ 3200, 1]
//	  score  stride 32: "494" [  800, 1]
//	  bbox   stride  8: "451" [12800, 4]
//	  bbox   stride 16: "474" [ 3200, 4]
//	  bbox   stride 32: "497" [  800, 4]
//	  kps    stride  8: "454" [12800,10]
//	  kps    stride 16: "477" [ 3200,10]
//	  kps    stride 32: "500" [  800,10]
//
// (12800 = (640/8)^2 * 2 anchors, 3200 = (640/16)^2 * 2, 800 = (640/32)^2 * 2
// — confirms input size 640 and 2 anchors/location.) The decode formulas
// (distance2bbox/distance2kps, anchor center generation, score-as-probability)
// match the reference insightface scrfd.py implementation, which is the
// canonical, widely reimplemented algorithm for this exact model family.
import (
	"fmt"
	"image"
	"sort"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	scrfdInputSize   = 640
	scrfdNumAnchors  = 2
	scrfdScoreThresh = 0.5
	scrfdNMSThresh   = 0.4
)

var scrfdStrides = [3]int{8, 16, 32}

// scrfdOutputNames is passed to NewDynamicAdvancedSession so Run() always
// returns outputs in this exact grouped order: [score x3, bbox x3, kps x3],
// one per stride in scrfdStrides order. This is what lets Detect index
// outputs[0:3]/[3:6]/[6:9] by stride below without re-deriving order from
// tensor names at runtime.
var scrfdOutputNames = []string{
	"448", "471", "494", // score, stride 8/16/32
	"451", "474", "497", // bbox,  stride 8/16/32
	"454", "477", "500", // kps,   stride 8/16/32
}

type detector struct {
	session *ort.DynamicAdvancedSession
}

func newDetector(modelPath string) (*detector, error) {
	session, err := ort.NewDynamicAdvancedSession(modelPath, []string{"input.1"}, scrfdOutputNames, nil)
	if err != nil {
		return nil, fmt.Errorf("create detection session: %w", err)
	}
	return &detector{session: session}, nil
}

func (d *detector) Close() {
	if d.session != nil {
		_ = d.session.Destroy()
	}
}

// detectedFace is one face found by the detector, in ORIGINAL image pixel
// coordinates (already un-letterboxed — see letterbox640/detScale).
type detectedFace struct {
	X1, Y1, X2, Y2 float64
	Score          float64
	Landmarks      [5][2]float64
}

// Detect finds faces in img, returning results in img's own pixel coordinate
// space (not the internal 640x640 letterboxed space used for inference).
func (d *detector) Detect(img image.Image) ([]detectedFace, error) {
	chw, scale := letterbox640(img)

	inputTensor, err := ort.NewTensor(ort.NewShape(1, 3, scrfdInputSize, scrfdInputSize), chw)
	if err != nil {
		return nil, fmt.Errorf("build input tensor: %w", err)
	}
	defer func() { _ = inputTensor.Destroy() }()

	outputs := make([]ort.Value, len(scrfdOutputNames))
	if err := d.session.Run([]ort.Value{inputTensor}, outputs); err != nil {
		return nil, fmt.Errorf("run detection session: %w", err)
	}
	defer func() {
		for _, o := range outputs {
			if o != nil {
				_ = o.Destroy()
			}
		}
	}()

	var candidates []detectedFace
	for si, stride := range scrfdStrides {
		scoreT, ok := outputs[si].(*ort.Tensor[float32])
		if !ok {
			return nil, fmt.Errorf("unexpected type for score output %d", si)
		}
		bboxT, ok := outputs[3+si].(*ort.Tensor[float32])
		if !ok {
			return nil, fmt.Errorf("unexpected type for bbox output %d", si)
		}
		kpsT, ok := outputs[6+si].(*ort.Tensor[float32])
		if !ok {
			return nil, fmt.Errorf("unexpected type for kps output %d", si)
		}

		scores := scoreT.GetData()
		bboxes := bboxT.GetData()
		kps := kpsT.GetData()

		featSize := scrfdInputSize / stride
		idx := 0
		for gy := 0; gy < featSize; gy++ {
			for gx := 0; gx < featSize; gx++ {
				cx := float64(gx * stride)
				cy := float64(gy * stride)
				for a := 0; a < scrfdNumAnchors; a++ {
					score := float64(scores[idx])
					if score >= scrfdScoreThresh {
						s := float64(stride)
						x1 := cx - float64(bboxes[idx*4+0])*s
						y1 := cy - float64(bboxes[idx*4+1])*s
						x2 := cx + float64(bboxes[idx*4+2])*s
						y2 := cy + float64(bboxes[idx*4+3])*s

						var lmk [5][2]float64
						for k := 0; k < 5; k++ {
							lmk[k][0] = cx + float64(kps[idx*10+2*k])*s
							lmk[k][1] = cy + float64(kps[idx*10+2*k+1])*s
						}

						candidates = append(candidates, detectedFace{
							X1: x1, Y1: y1, X2: x2, Y2: y2,
							Score:     score,
							Landmarks: lmk,
						})
					}
					idx++
				}
			}
		}
	}

	kept := nmsFaces(candidates, scrfdNMSThresh)

	// Un-letterbox: map from the 640x640 inference space back to img's own
	// pixel coordinates.
	b := img.Bounds()
	origW, origH := float64(b.Dx()), float64(b.Dy())
	for i := range kept {
		kept[i].X1 = clamp(kept[i].X1/scale, 0, origW)
		kept[i].Y1 = clamp(kept[i].Y1/scale, 0, origH)
		kept[i].X2 = clamp(kept[i].X2/scale, 0, origW)
		kept[i].Y2 = clamp(kept[i].Y2/scale, 0, origH)
		for k := range kept[i].Landmarks {
			kept[i].Landmarks[k][0] /= scale
			kept[i].Landmarks[k][1] /= scale
		}
	}
	return kept, nil
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// nmsFaces runs standard greedy IoU-based non-max suppression, keeping
// higher-score candidates and discarding lower-score ones that overlap them
// by more than thresh.
func nmsFaces(candidates []detectedFace, thresh float64) []detectedFace {
	if len(candidates) == 0 {
		return nil
	}
	sorted := append([]detectedFace(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })

	kept := make([]detectedFace, 0, len(sorted))
	suppressed := make([]bool, len(sorted))
	for i := range sorted {
		if suppressed[i] {
			continue
		}
		kept = append(kept, sorted[i])
		for j := i + 1; j < len(sorted); j++ {
			if suppressed[j] {
				continue
			}
			if iou(sorted[i], sorted[j]) > thresh {
				suppressed[j] = true
			}
		}
	}
	return kept
}

func iou(a, b detectedFace) float64 {
	x1 := max64(a.X1, b.X1)
	y1 := max64(a.Y1, b.Y1)
	x2 := min64(a.X2, b.X2)
	y2 := min64(a.Y2, b.Y2)
	interW := max64(0, x2-x1)
	interH := max64(0, y2-y1)
	inter := interW * interH
	areaA := max64(0, a.X2-a.X1) * max64(0, a.Y2-a.Y1)
	areaB := max64(0, b.X2-b.X1) * max64(0, b.Y2-b.Y1)
	union := areaA + areaB - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func max64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// letterbox640 resizes img so its longer side is exactly scrfdInputSize
// while preserving aspect ratio, pastes it into the top-left corner of a
// black scrfdInputSize x scrfdInputSize canvas (matching insightface's
// reference preprocessing — padding is bottom/right only, not centered),
// and returns the canvas as a normalized NCHW float32 slice ready for the
// model's "input.1" tensor, along with the scale factor
// (newHeight/origHeight, equivalently newWidth/origWidth since aspect is
// preserved) needed to map detections back to img's own coordinate space.
//
// Normalization is (pixel - 127.5) / 128.0 in R,G,B channel order — the
// reference implementation reads BGR via OpenCV and swaps to RGB
// (swapRB=true) before this same formula; Go's image package already
// decodes to RGB, so no channel swap is needed here.
func letterbox640(img image.Image) (chw []float32, scale float64) {
	b := img.Bounds()
	origW, origH := b.Dx(), b.Dy()

	imRatio := float64(origH) / float64(origW)
	var newW, newH int
	if imRatio > 1.0 {
		newH = scrfdInputSize
		newW = int(float64(newH) / imRatio)
	} else {
		newW = scrfdInputSize
		newH = int(float64(newW) * imRatio)
	}
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	scale = float64(newH) / float64(origH)

	chw = make([]float32, 3*scrfdInputSize*scrfdInputSize)
	const rPlane = 0
	const gPlane = scrfdInputSize * scrfdInputSize
	const bPlane = 2 * scrfdInputSize * scrfdInputSize

	// Bilinear resize directly into the padded canvas (matches cv2.resize's
	// default interpolation, which the reference preprocessing relies on).
	// Source coordinates use pixel centers spaced by the scale ratio, the
	// standard resize sampling formula.
	scaleX := float64(origW) / float64(newW)
	scaleY := float64(origH) / float64(newH)
	for y := 0; y < newH; y++ {
		srcY := (float64(y)+0.5)*scaleY - 0.5
		for x := 0; x < newW; x++ {
			srcX := (float64(x)+0.5)*scaleX - 0.5
			r8, g8, b8, _ := bilinearSample(img, float64(b.Min.X)+srcX, float64(b.Min.Y)+srcY)

			pixelIdx := y*scrfdInputSize + x
			chw[rPlane+pixelIdx] = (float32(r8) - 127.5) / 128.0
			chw[gPlane+pixelIdx] = (float32(g8) - 127.5) / 128.0
			chw[bPlane+pixelIdx] = (float32(b8) - 127.5) / 128.0
		}
	}
	// Padding area (x>=newW or y>=newH) is left at its zero value, which
	// after normalization should represent pixel value 0 exactly like the
	// reference's zero-filled canvas does — apply the same normalization to
	// the implicit zero pixels the make() zero-value skipped.
	padValue := float32((0 - 127.5) / 128.0)
	for y := 0; y < scrfdInputSize; y++ {
		for x := 0; x < scrfdInputSize; x++ {
			if x < newW && y < newH {
				continue
			}
			pixelIdx := y*scrfdInputSize + x
			chw[rPlane+pixelIdx] = padValue
			chw[gPlane+pixelIdx] = padValue
			chw[bPlane+pixelIdx] = padValue
		}
	}

	return chw, scale
}
