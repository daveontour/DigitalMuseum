package service

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os/exec"
)

// faceCropPaddingRatio expands a face's tight detection box by this fraction
// on each side when producing a representative crop, so the image shows a
// bit of surrounding context (hair, shoulders) rather than just the tight
// SCRFD bounding box. 0.6 means the crop is 1.6x the tight box's width/height.
const faceCropPaddingRatio = 0.6

// FaceCropGeometry computes the pixel crop rectangle for a face's fractional
// bounding box (bboxX/Y/W/H, each 0..1, as stored in media_item_faces) within
// an image of the given pixel dimensions, padded by faceCropPaddingRatio and
// clamped to the image bounds. A pure function, kept separate from the
// ImageMagick invocation in CropFaceJPEG so the geometry math is independently
// unit-testable.
func FaceCropGeometry(bboxX, bboxY, bboxW, bboxH float64, imgWidth, imgHeight int) (x, y, w, h int) {
	fw, fh := float64(imgWidth), float64(imgHeight)
	cx := bboxX*fw + bboxW*fw/2
	cy := bboxY*fh + bboxH*fh/2
	halfW := bboxW * fw / 2 * (1 + faceCropPaddingRatio)
	halfH := bboxH * fh / 2 * (1 + faceCropPaddingRatio)

	x0, y0 := cx-halfW, cy-halfH
	x1, y1 := cx+halfW, cy+halfH

	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > fw {
		x1 = fw
	}
	if y1 > fh {
		y1 = fh
	}
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}

	x, y = int(x0), int(y0)
	w, h = int(x1-x0), int(y1-y0)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return x, y, w, h
}

// CropFaceJPEG crops imgBytes to the padded region around a face's
// fractional bounding box and returns a resized JPEG (longest side capped at
// outSize, aspect preserved), via the bundled ImageMagick binary — the same
// "-" stdin/stdout subprocess pattern used by internal/import/thumbnails.
func CropFaceJPEG(imgBytes []byte, bboxX, bboxY, bboxW, bboxH float64, outSize int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image config: %w", err)
	}
	x, y, w, h := FaceCropGeometry(bboxX, bboxY, bboxW, bboxH, cfg.Width, cfg.Height)

	// Deliberately no -auto-orient here (unlike internal/import/thumbnails,
	// which processes a whole image with no separately-computed coordinates
	// to keep in sync). x/y/w/h above were computed from cfg.Width/Height —
	// image.DecodeConfig's raw pixel dimensions, before any EXIF-orientation
	// correction — the same raw dimensions cmd/facerecognizer's image.Decode
	// used when it computed the face's bbox fractions in the first place (Go's
	// image package never auto-applies EXIF orientation). Auto-orienting here
	// would rotate/resize the canvas (e.g. swap width/height for a portrait
	// phone photo) before the crop is applied, so a crop rectangle computed in
	// the pre-rotation coordinate space would land in the wrong place on a
	// now-different-shaped image — this was confirmed to produce exactly the
	// thin, misplaced slivers seen in testing, for any photo carrying EXIF
	// orientation metadata (i.e. most phone photos).
	args := []string{
		"-",
		"-crop", fmt.Sprintf("%dx%d+%d+%d", w, h, x, y),
		"+repage",
		"-resize", fmt.Sprintf("%dx%d>", outSize, outSize),
		"-quality", "90",
		"jpg:-",
	}
	cmd := exec.Command("bin/ImageMagick/magick", args...)
	hideConsole(cmd)
	cmd.Stdin = bytes.NewReader(imgBytes)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("imagemagick crop: %w (%s)", err, stderr.String())
	}
	return out.Bytes(), nil
}
