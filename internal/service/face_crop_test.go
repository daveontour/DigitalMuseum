package service

import "testing"

func TestFaceCropGeometry_CenteredBox(t *testing.T) {
	// A face occupying the middle 50% of a 1000x1000 image (0.25..0.75 in
	// both axes) has plenty of room to pad on every side without clamping.
	x, y, w, h := FaceCropGeometry(0.25, 0.25, 0.5, 0.5, 1000, 1000)

	// Tight box is 500x500 centered at (500,500); padded by 1.6x -> 800x800,
	// so the crop should span (100,100) to (900,900).
	if x != 100 || y != 100 {
		t.Errorf("want origin (100,100), got (%d,%d)", x, y)
	}
	if w != 800 || h != 800 {
		t.Errorf("want size 800x800, got %dx%d", w, h)
	}
}

func TestFaceCropGeometry_ClampsAtImageEdges(t *testing.T) {
	// A face right at the top-left corner: padding would want to extend
	// past (0,0), so the crop must clamp there instead of going negative.
	x, y, w, h := FaceCropGeometry(0.0, 0.0, 0.2, 0.2, 1000, 1000)
	if x < 0 || y < 0 {
		t.Fatalf("crop origin must not be negative, got (%d,%d)", x, y)
	}
	if x != 0 || y != 0 {
		t.Errorf("want origin clamped to (0,0), got (%d,%d)", x, y)
	}
	if w <= 0 || h <= 0 {
		t.Fatalf("crop size must be positive, got %dx%d", w, h)
	}
}

func TestFaceCropGeometry_ClampsAtBottomRight(t *testing.T) {
	x, y, w, h := FaceCropGeometry(0.85, 0.85, 0.15, 0.15, 1000, 1000)
	if x+w > 1000 || y+h > 1000 {
		t.Errorf("crop must not extend past image bounds: x+w=%d y+h=%d (image 1000x1000)", x+w, y+h)
	}
}

func TestFaceCropGeometry_NeverProducesZeroOrNegativeSize(t *testing.T) {
	cases := []struct{ x, y, w, h float64 }{
		{0, 0, 0, 0},         // degenerate zero-size box
		{1, 1, 0.01, 0.01},   // box entirely outside bounds (x,y at the edge, tiny size)
		{0.5, 0.5, 0, 0},     // zero-size box in the middle
	}
	for _, c := range cases {
		_, _, w, h := FaceCropGeometry(c.x, c.y, c.w, c.h, 500, 500)
		if w < 1 || h < 1 {
			t.Errorf("FaceCropGeometry(%v) produced non-positive size %dx%d", c, w, h)
		}
	}
}

func TestFaceCropGeometry_NonSquareImage(t *testing.T) {
	// A wide image (1600x900); a centered face should still produce a
	// symmetric padded box without accidentally using the wrong dimension.
	x, y, w, h := FaceCropGeometry(0.4, 0.3, 0.2, 0.3, 1600, 900)
	if x < 0 || y < 0 || x+w > 1600 || y+h > 900 {
		t.Errorf("crop out of bounds for 1600x900 image: x=%d y=%d w=%d h=%d", x, y, w, h)
	}
	// Sanity: tight box is 320x270 at (640,270); padded 1.6x -> 512x432,
	// centered on the same center point (800,405).
	wantW, wantH := 512, 432
	if w != wantW || h != wantH {
		t.Errorf("want padded size %dx%d, got %dx%d", wantW, wantH, w, h)
	}
}
