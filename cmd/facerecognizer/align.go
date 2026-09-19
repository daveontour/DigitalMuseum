package main

// Face alignment for the recognition model: warps a 112x112 crop out of the
// original image so the detected 5 landmarks line up with ArcFace's fixed
// reference template, matching insightface's face_align.norm_crop (mode
// "arcface", image_size=112 — so ratio=1.0, diff_x=0 and the template below
// is used unscaled/unshifted).
import (
	"image"
	"image/color"
	"math/cmplx"
)

// arcfaceDst is the fixed 5-point reference template ArcFace-family models
// are trained to expect (insightface face_align.py's arcface_dst constant).
var arcfaceDst = [5][2]float64{
	{38.2946, 51.6963},
	{73.5318, 51.5014},
	{56.0252, 71.7366},
	{41.5493, 92.3655},
	{70.7299, 92.2041},
}

const alignedSize = 112

// estimateSimilarity finds the complex numbers a, b such that a*src[i]+b
// approximates dst[i] as closely as possible in the least-squares sense
// (points represented as complex numbers x+iy). This is a similarity
// transform: a encodes uniform scale and rotation together (scale=|a|,
// rotation=arg(a)), b is translation.
//
// This is mathematically equivalent to skimage's SimilarityTransform.estimate
// (the Umeyama algorithm) for the non-reflective case, which always holds
// here since face landmarks are always presented in a fixed, non-mirrored
// order (left eye, right eye, nose, left mouth corner, right mouth corner) —
// there is no ambiguity between a valid rotation and a reflection to resolve,
// unlike Umeyama's general-purpose formulation which has to handle that case.
//
// Derivation: minimizing sum_i |a*src_i + b - dst_i|^2 over complex a,b is a
// linear least-squares problem. Setting the derivative w.r.t. conj(b) to
// zero gives b = mean(dst) - a*mean(src); substituting into the derivative
// w.r.t. conj(a) and simplifying (using that centered points sum to zero)
// gives the closed form below.
func estimateSimilarity(src, dst [5][2]float64) (a, b complex128) {
	var srcMean, dstMean complex128
	var srcPts, dstPts [5]complex128
	for i := 0; i < 5; i++ {
		srcPts[i] = complex(src[i][0], src[i][1])
		dstPts[i] = complex(dst[i][0], dst[i][1])
		srcMean += srcPts[i]
		dstMean += dstPts[i]
	}
	srcMean /= 5
	dstMean /= 5

	var num complex128
	var den float64
	for i := 0; i < 5; i++ {
		sc := srcPts[i] - srcMean
		dc := dstPts[i] - dstMean
		num += dc * cmplx.Conj(sc)
		den += real(sc)*real(sc) + imag(sc)*imag(sc)
	}
	if den == 0 {
		// Degenerate (all landmarks coincide) — identity transform rather
		// than dividing by zero.
		return complex(1, 0), dstMean - srcMean
	}
	a = num / complex(den, 0)
	b = dstMean - a*srcMean
	return a, b
}

// alignFace warps a 112x112 crop out of img so landmarks (in img's own pixel
// coordinate space) line up with arcfaceDst, using inverse-mapped bilinear
// sampling — the same approach cv2.warpAffine uses internally (compute the
// forward transform from the point correspondences, then sample each output
// pixel from its inverse-mapped source location, rather than forward-
// splatting source pixels).
func alignFace(img image.Image, landmarks [5][2]float64) *image.RGBA {
	a, b := estimateSimilarity(landmarks, arcfaceDst)

	// Inverse of z -> a*z + b is z -> (1/a)*z - (1/a)*b.
	invA := complex(1, 0) / a
	invB := -invA * b

	out := image.NewRGBA(image.Rect(0, 0, alignedSize, alignedSize))
	for v := 0; v < alignedSize; v++ {
		for u := 0; u < alignedSize; u++ {
			zDst := complex(float64(u), float64(v))
			zSrc := invA*zDst + invB
			sx, sy := real(zSrc), imag(zSrc)

			r, g, bl, al := bilinearSample(img, sx, sy)
			out.SetRGBA(u, v, color.RGBA{R: r, G: g, B: bl, A: al})
		}
	}
	return out
}
