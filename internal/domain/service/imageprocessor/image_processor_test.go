package imageprocessor

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/require"
)

// TestCompressAlreadyCompactJPEGUnchanged checks that a JPEG already under
// the skip threshold is returned byte-for-byte, without re-encoding.
func TestCompressAlreadyCompactJPEGUnchanged(t *testing.T) {
	t.Parallel()

	c := New()

	data := solidJPEG(t, 40, 40, 80)

	out, ext, err := c.Compress(data, "image/jpeg")
	require.NoError(t, err)
	require.Equal(t, ".jpg", ext)
	require.True(t, bytes.Equal(out, data), "Compress() modified an already-compact JPEG, want it returned unchanged")
}

// TestCompressUndecodableDataPassedThrough checks that data the standard
// library can't decode (e.g. HEIC/WebP) is passed through unchanged, with the extension picked from
// mimeType alone.
func TestCompressUndecodableDataPassedThrough(t *testing.T) {
	t.Parallel()

	c := New()

	tests := []struct {
		mimeType string
		wantExt  string
	}{
		{
			"image/png",
			".png",
		},
		{
			"image/gif",
			".gif",
		},
		{
			"image/webp",
			".webp",
		},
		{
			"image/heic",
			".heic",
		},
		{
			"image/heif",
			".heic",
		},
		{
			"application/octet-stream",
			".jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			t.Parallel()

			data := []byte(gofakeit.LetterN(64)) // not valid image data

			out, ext, err := c.Compress(data, tt.mimeType)
			require.NoError(t, err)
			require.Equal(t, tt.wantExt, ext)
			require.True(t, bytes.Equal(out, data), "Compress() modified undecodable data, want it returned unchanged")
		})
	}
}

// TestCompressResizesOversizedImage checks that an image bigger than the max
// dimension is shrunk, preserving aspect ratio, and re-encoded as JPEG.
func TestCompressResizesOversizedImage(t *testing.T) {
	t.Parallel()

	c := New()

	data := gradientPNG(t, 2000, 1200) // longer side exceeds the 1600px cap

	out, ext, err := c.Compress(data, "image/png")
	require.NoError(t, err)
	require.Equal(t, ".jpg", ext)

	img, err := jpeg.Decode(bytes.NewReader(out))
	require.NoError(t, err, "Compress() output isn't a valid JPEG")

	b := img.Bounds()
	require.LessOrEqual(t, b.Dx(), 1600)
	require.LessOrEqual(t, b.Dy(), 1600)

	wantRatio := 2000.0 / 1200.0
	gotRatio := float64(b.Dx()) / float64(b.Dy())
	require.InDelta(t, wantRatio, gotRatio, 0.05)
}

// TestCompressJPEGOverThresholdIsRecompressed checks that a JPEG over the
// skip threshold, but already within the max dimension, is re-encoded rather than left untouched —
// isolating the re-encode path from resizing.
func TestCompressJPEGOverThresholdIsRecompressed(t *testing.T) {
	t.Parallel()

	c := New()

	data := noisyJPEG(t, 1200, 1200, 100)
	require.Greater(t, len(data), 350*1024, "test fixture too small, want > 350KB to skip the fast path")

	out, ext, err := c.Compress(data, "image/jpeg")
	require.NoError(t, err)
	require.Equal(t, ".jpg", ext)
	require.False(t, bytes.Equal(out, data), "Compress() left an oversized JPEG unchanged, want it recompressed")

	img, err := jpeg.Decode(bytes.NewReader(out))
	require.NoError(t, err, "Compress() output isn't a valid JPEG")

	b := img.Bounds()
	require.Equal(t, 1200, b.Dx())
	require.Equal(t, 1200, b.Dy())
}

// TestExtForMime checks the mimeType -> extension mapping used for formats
// Compress doesn't re-encode.
func TestExtForMime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mimeType string
		want     string
	}{
		{
			"image/png",
			".png",
		},
		{
			"image/gif",
			".gif",
		},
		{
			"image/webp",
			".webp",
		},
		{
			"image/heic",
			".heic",
		},
		{
			"image/heif",
			".heic",
		},
		{
			"image/jpeg",
			".jpg",
		},
		{
			"",
			".jpg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, extForMime(tt.mimeType))
		})
	}
}

// TestMaybeResizeWithinBounds checks that an image already within maxDim on
// both sides is left unchanged.
func TestMaybeResizeWithinBounds(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 800, 600))

	got := maybeResize(img)

	b := got.Bounds()
	require.Equal(t, 800, b.Dx())
	require.Equal(t, 600, b.Dy())
}

// TestMaybeResizeWideImage checks that a wide image is shrunk to maxDim on
// its longer (horizontal) side, preserving aspect ratio.
func TestMaybeResizeWideImage(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 3200, 1600))

	got := maybeResize(img)

	b := got.Bounds()
	require.Equal(t, 1600, b.Dx())
	require.Equal(t, 800, b.Dy(), "aspect ratio preserved")
}

// TestMaybeResizeTallImage checks that a tall image is shrunk to maxDim on
// its longer (vertical) side, preserving aspect ratio.
func TestMaybeResizeTallImage(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 1600, 3200))

	got := maybeResize(img)

	b := got.Bounds()
	require.Equal(t, 1600, b.Dy())
	require.Equal(t, 800, b.Dx(), "aspect ratio preserved")
}

// TestMaybeResizeExtremeAspectRatioClampsToOnePixel checks that a
// pathologically thin image doesn't collapse its short side to 0px, in either orientation: a wide
// image clamps its computed height, a tall one clamps its computed width.
func TestMaybeResizeExtremeAspectRatioClampsToOnePixel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		w, h   int
		checkW bool // which dimension the extreme ratio drives toward 0: width (tall source) or height (wide source)
	}{
		{
			name: "a pathologically wide image clamps its height",
			w:    20000,
			h:    1,
		},
		{
			name:   "a pathologically tall image clamps its width",
			w:      1,
			h:      20000,
			checkW: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			img := image.NewRGBA(image.Rect(0, 0, tt.w, tt.h))

			got := maybeResize(img)

			b := got.Bounds()
			if tt.checkW {
				require.GreaterOrEqual(t, b.Dx(), 1)

				return
			}

			require.GreaterOrEqual(t, b.Dy(), 1)
		})
	}
}

// TestResizeBilinearTinySource checks the nearest-neighbor fallback used when
// the source is too small (< 2px on either side) for interpolation.
func TestResizeBilinearTinySource(t *testing.T) {
	t.Parallel()

	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	src.Set(0, 0, color.RGBA{
		R: 10,
		G: 20,
		B: 30,
		A: 255,
	})

	dst := resizeBilinear(src, 4, 4)

	b := dst.Bounds()
	require.Equal(t, 4, b.Dx())
	require.Equal(t, 4, b.Dy())

	r, g, bl, a := dst.At(2, 2).RGBA()
	wantR, wantG, wantB, wantA := src.At(0, 0).RGBA()
	require.Equal(t, wantR, r)
	require.Equal(t, wantG, g)
	require.Equal(t, wantB, bl)
	require.Equal(t, wantA, a)
}

// TestResizeBilinearInterpolatesCorners checks that upscaling a 2x2 image
// keeps its top-left corner close to the source's top-left color.
func TestResizeBilinearInterpolatesCorners(t *testing.T) {
	t.Parallel()

	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.Set(0, 0, color.RGBA{
		R: 0,
		G: 0,
		B: 0,
		A: 255,
	})
	src.Set(1, 0, color.RGBA{
		R: 255,
		G: 0,
		B: 0,
		A: 255,
	})
	src.Set(0, 1, color.RGBA{
		R: 0,
		G: 255,
		B: 0,
		A: 255,
	})
	src.Set(1, 1, color.RGBA{
		R: 255,
		G: 255,
		B: 0,
		A: 255,
	})

	dst := resizeBilinear(src, 3, 3)

	b := dst.Bounds()
	require.Equal(t, 3, b.Dx())
	require.Equal(t, 3, b.Dy())

	c, ok := dst.At(0, 0).(color.RGBA)
	require.True(t, ok)
	require.LessOrEqual(t, c.R, uint8(10))
	require.LessOrEqual(t, c.G, uint8(10))
}

// TestBilerp checks interpolation at the corners and the midpoint of a 2x2
// color patch.
func TestBilerp(t *testing.T) {
	t.Parallel()

	c00 := color.RGBA{
		R: 0,
		G: 0,
		B: 0,
		A: 255,
	}
	c10 := color.RGBA{
		R: 100,
		G: 100,
		B: 100,
		A: 255,
	}
	c01 := color.RGBA{
		R: 0,
		G: 0,
		B: 0,
		A: 255,
	}
	c11 := color.RGBA{
		R: 100,
		G: 100,
		B: 100,
		A: 255,
	}

	tests := []struct {
		name   string
		fx, fy float64
		wantR  uint8
	}{
		{
			"top-left corner",
			0,
			0,
			0,
		},
		{
			"top-right corner",
			1,
			0,
			100,
		},
		{
			"midpoint",
			0.5,
			0.5,
			50,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := bilerp(c00, c10, c01, c11, tt.fx, tt.fy)
			require.Equal(t, tt.wantR, got.R)
		})
	}
}

// TestClamp255 checks clamp255's bounds.
func TestClamp255(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{
			"below range",
			-10,
			0,
		},
		{
			"above range",
			300,
			255,
		},
		{
			"within range",
			128,
			128,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.InDelta(t, tt.want, clamp255(tt.in), 0)
		})
	}
}

// TestEncodeJPEGWithBudgetFitsWithinBudget checks that an easily compressible
// image is encoded within the given byte budget.
func TestEncodeJPEGWithBudgetFitsWithinBudget(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	for y := range 100 {
		for x := range 100 {
			img.Set(x, y, color.RGBA{
				R: 128,
				G: 128,
				B: 128,
				A: 255,
			})
		}
	}

	out, err := encodeJPEGWithBudget(img, maxImageBytes)
	require.NoError(t, err)
	require.LessOrEqual(t, len(out), maxImageBytes)
}

// TestEncodeJPEGWithBudgetStepsQualityDown checks that an impossible budget
// still terminates (quality bottoms out at minJPEGQuality) instead of looping forever.
func TestEncodeJPEGWithBudgetStepsQualityDown(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 300, 300))

	seed := uint32(12345)

	for y := range 300 {
		for x := range 300 {
			seed = seed*1664525 + 1013904223
			img.Set(x, y, color.RGBA{
				R: uint8(seed),
				G: uint8(seed >> 8),
				B: uint8(seed >> 16),
				A: 255,
			})
		}
	}

	out, err := encodeJPEGWithBudget(img, 1) // unreachable budget
	require.NoError(t, err)
	require.NotEmpty(t, out)
}

// TestEncodeJPEGWithBudgetEncodeError checks that a jpeg.Encode failure — the JPEG format caps each
// side at 65535px — is surfaced as an error rather than looping or returning a truncated result.
func TestEncodeJPEGWithBudgetEncodeError(t *testing.T) {
	t.Parallel()

	img := image.NewRGBA(image.Rect(0, 0, 70000, 1))

	_, err := encodeJPEGWithBudget(img, maxImageBytes)
	require.Error(t, err)
}

// solidJPEG encodes a w x h solid-color JPEG at the given quality.
func solidJPEG(t *testing.T, w, h, quality int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{
				R: 200,
				G: 100,
				B: 50,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	require.NoError(t, err, "failed to encode test JPEG")

	return buf.Bytes()
}

// noisyJPEG encodes a w x h JPEG filled with pseudo-random noise, which compresses poorly — useful
// for exercising the size-budget logic.
func noisyJPEG(t *testing.T, w, h, quality int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))

	seed := uint32(12345)

	for y := range h {
		for x := range w {
			seed = seed*1664525 + 1013904223
			img.Set(x, y, color.RGBA{
				R: uint8(seed),
				G: uint8(seed >> 8),
				B: uint8(seed >> 16),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	require.NoError(t, err, "failed to encode test JPEG")

	return buf.Bytes()
}

// gradientPNG encodes a w x h PNG with a smooth gradient, decodable and compressible via the
// resize+re-encode path.
func gradientPNG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{
				R: uint8(x % 256),
				G: uint8(y % 256),
				B: uint8((x + y) % 256),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	require.NoError(t, err, "failed to encode test PNG")

	return buf.Bytes()
}
