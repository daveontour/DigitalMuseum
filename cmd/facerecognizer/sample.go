package main

import "image"

// bilinearSample samples img at floating-point coordinate (x, y) using
// OpenCV's pixel-center convention (pixel (i,j)'s value is located exactly
// at integer coordinate (i,j), no half-pixel offset) — this is the
// convention both the reference cv2.resize and cv2.warpAffine calls use, and
// matching it matters here since detection/alignment coordinates are
// produced against that same convention. Returns (0,0,0,255) — opaque black
// — for out-of-bounds coordinates, matching cv2.warpAffine's default
// borderValue=0 used for alignment; resize callers never sample out of
// bounds since they only interpolate within the source rectangle.
func bilinearSample(img image.Image, x, y float64) (r, g, b, a uint8) {
	b0 := img.Bounds()

	x0f, y0f := floor(x), floor(y)
	x0, y0 := int(x0f), int(y0f)
	fx, fy := x-x0f, y-y0f

	get := func(px, py int) (float64, float64, float64, float64) {
		if px < b0.Min.X || px >= b0.Max.X || py < b0.Min.Y || py >= b0.Max.Y {
			return 0, 0, 0, 0
		}
		rr, gg, bb, aa := img.At(px, py).RGBA()
		return float64(rr >> 8), float64(gg >> 8), float64(bb >> 8), float64(aa >> 8)
	}

	r00, g00, b00, a00 := get(x0, y0)
	r10, g10, b10, a10 := get(x0+1, y0)
	r01, g01, b01, a01 := get(x0, y0+1)
	r11, g11, b11, a11 := get(x0+1, y0+1)

	lerp2 := func(v00, v10, v01, v11 float64) float64 {
		top := v00*(1-fx) + v10*fx
		bottom := v01*(1-fx) + v11*fx
		return top*(1-fy) + bottom*fy
	}

	rf := lerp2(r00, r10, r01, r11)
	gf := lerp2(g00, g10, g01, g11)
	bf := lerp2(b00, b10, b01, b11)
	af := lerp2(a00, a10, a01, a11)

	return clampByte(rf), clampByte(gf), clampByte(bf), clampByte(af)
}

func floor(v float64) float64 {
	i := int64(v)
	if v < 0 && float64(i) != v {
		i--
	}
	return float64(i)
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}
