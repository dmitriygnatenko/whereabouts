// Package imageprocessor implements port.ImageProcessor.
package imageprocessor

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
)

// Compression thresholds. The client already shrinks photos before sending (see resizeImage in the
// frontend), but the backend can't rely on that — requests may come from another client, or with an
// uncropped photo.
const (
	maxImageDimension  = 1600       // max side length in pixels
	maxImageBytes      = 700 * 1024 // target file size after compression (~700 KB)
	skipIfUnderBytes   = 350 * 1024 // already-compact JPEGs are left untouched
	initialJPEGQuality = 85
	minJPEGQuality     = 40
	jpegQualityStep    = 15
)

// MaxRequestBytes caps the HTTP body of endpoints that accept photos — protects the server against
// oversized payloads.
const MaxRequestBytes = 20 * 1024 * 1024

// Compressor implements port.ImageProcessor.
type Compressor struct{}

// New builds a Compressor.
func New() *Compressor {
	return &Compressor{}
}

// Compress brings a photo down to a reasonable size: if it's already compact it's returned as-is,
// otherwise it's shrunk along its longer side to maxImageDimension and re-encoded as JPEG at a
// quality chosen to land around maxImageBytes. Returns file-ready bytes and the extension to save
// them under.
func (c *Compressor) Compress(data []byte, mimeType string) (out []byte, ext string, err error) {
	if len(data) <= skipIfUnderBytes && mimeType == "image/jpeg" {
		return data, ".jpg", nil
	}

	img, _, decErr := image.Decode(bytes.NewReader(data))
	if decErr != nil {
		// A format the standard library can't decode (e.g. HEIC/WebP) — better to keep the photo as-is
		// than lose it to a failed compression attempt.
		return data, extForMime(mimeType), nil //nolint:nilerr // deliberate fallback, see comment above
	}

	img = maybeResize(img, maxImageDimension)

	encoded, encErr := encodeJPEGWithBudget(img, maxImageBytes)
	if encErr != nil {
		return nil, "", fmt.Errorf("failed to compress image: %w", encErr)
	}

	return encoded, ".jpg", nil
}

// extForMime picks a file extension for formats we don't re-encode to JPEG (see Compress above).
func extForMime(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/heic", "image/heif":
		return ".heic"
	default:
		return ".jpg"
	}
}

// encodeJPEGWithBudget encodes an image as JPEG, stepping quality down until the result fits within
// budget bytes or the minimum quality is hit.
func encodeJPEGWithBudget(img image.Image, budget int) ([]byte, error) {
	quality := initialJPEGQuality

	var buf bytes.Buffer

	for {
		buf.Reset()

		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, err
		}

		if buf.Len() <= budget || quality <= minJPEGQuality {
			break
		}

		quality -= jpegQualityStep
		if quality < minJPEGQuality {
			quality = minJPEGQuality
		}
	}

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())

	return out, nil
}

// maybeResize shrinks an image, preserving aspect ratio, if either side exceeds maxDim. Returns it
// unchanged if it already fits.
func maybeResize(img image.Image, maxDim int) image.Image {
	b := img.Bounds()

	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return img
	}

	var newW, newH int
	if w >= h {
		newW = maxDim
		newH = int(float64(h) * float64(maxDim) / float64(w))
	} else {
		newH = maxDim
		newW = int(float64(w) * float64(maxDim) / float64(h))
	}

	if newW < 1 {
		newW = 1
	}

	if newH < 1 {
		newH = 1
	}

	return resizeBilinear(img, newW, newH)
}

// resizeBilinear is a dependency-free bilinear scale-down. Good enough for item photos without
// pulling in a package like x/image/draw.
func resizeBilinear(src image.Image, newW, newH int) image.Image {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	if srcW < 2 || srcH < 2 {
		// Source too small for bilinear interpolation — nearest-neighbor stretch instead, to avoid
		// dividing by zero.
		for y := 0; y < newH; y++ {
			for x := 0; x < newW; x++ {
				sx := x * srcW / newW
				sy := y * srcH / newH
				dst.Set(x, y, src.At(bounds.Min.X+sx, bounds.Min.Y+sy))
			}
		}

		return dst
	}

	xRatio := float64(srcW-1) / float64(newW)
	yRatio := float64(srcH-1) / float64(newH)

	for y := 0; y < newH; y++ {
		srcY := float64(y) * yRatio
		y0 := int(srcY)
		y1 := y0 + 1
		fy := srcY - float64(y0)

		for x := 0; x < newW; x++ {
			srcX := float64(x) * xRatio
			x0 := int(srcX)
			x1 := x0 + 1
			fx := srcX - float64(x0)

			c00 := src.At(bounds.Min.X+x0, bounds.Min.Y+y0)
			c10 := src.At(bounds.Min.X+x1, bounds.Min.Y+y0)
			c01 := src.At(bounds.Min.X+x0, bounds.Min.Y+y1)
			c11 := src.At(bounds.Min.X+x1, bounds.Min.Y+y1)

			dst.Set(x, y, bilerp(c00, c10, c01, c11, fx, fy))
		}
	}

	return dst
}

// bilerp linearly interpolates a color across four neighboring pixels.
func bilerp(c00, c10, c01, c11 color.Color, fx, fy float64) color.RGBA {
	r00, g00, b00, a00 := c00.RGBA()
	r10, g10, b10, a10 := c10.RGBA()
	r01, g01, b01, a01 := c01.RGBA()
	r11, g11, b11, a11 := c11.RGBA()

	lerp := func(a, b uint32, f float64) float64 {
		return float64(a) + (float64(b)-float64(a))*f
	}

	// Interpolate along X in the top and bottom rows first, then along Y between them.
	rTop, rBot := lerp(r00, r10, fx), lerp(r01, r11, fx)
	gTop, gBot := lerp(g00, g10, fx), lerp(g01, g11, fx)
	bTop, bBot := lerp(b00, b10, fx), lerp(b01, b11, fx)
	aTop, aBot := lerp(a00, a10, fx), lerp(a01, a11, fx)

	r := lerpF(rTop, rBot, fy)
	g := lerpF(gTop, gBot, fy)
	b := lerpF(bTop, bBot, fy)
	a := lerpF(aTop, aBot, fy)

	// RGBA() returns components in [0, 65535] — bring them down to [0, 255].
	return color.RGBA{
		R: uint8(clamp255(r / 257)),
		G: uint8(clamp255(g / 257)),
		B: uint8(clamp255(b / 257)),
		A: uint8(clamp255(a / 257)),
	}
}

// lerpF linearly interpolates between a and b at fraction f.
func lerpF(a, b, f float64) float64 {
	return a + (b-a)*f
}

// clamp255 clamps v into the [0, 255] range.
func clamp255(v float64) float64 {
	if v < 0 {
		return 0
	}

	if v > 255 {
		return 255
	}

	return v
}
