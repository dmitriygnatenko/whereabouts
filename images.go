package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/url"
	"strings"
)

// Пороговые значения сжатия. Клиент уже ужимает фото перед отправкой
// (см. resizeImage во фронтенде), но бэкенд не должен полагаться на это —
// сюда может прийти запрос и от другого клиента, и с необрезанным фото.
const (
	maxImageDimension  = 1600       // максимальная сторона изображения в пикселях
	maxImageBytes      = 700 * 1024 // целевой размер файла после сжатия (~700 КБ)
	skipIfUnderBytes   = 350 * 1024 // если файл и так компактный и уже JPEG — не трогаем
	initialJPEGQuality = 85
	minJPEGQuality     = 40
	jpegQualityStep    = 15
	maxRequestBytes    = 20 * 1024 * 1024 // общий лимит тела запроса (защита от чрезмерных payload'ов)
)

// parseDataURL разбирает "data:<mime>;base64,<данные>" на MIME-тип и сырые байты.
func parseDataURL(s string) (mimeType string, data []byte, err error) {
	if !strings.HasPrefix(s, "data:") {
		return "", nil, fmt.Errorf("does not look like a data URL")
	}
	comma := strings.IndexByte(s, ',')
	if comma == -1 {
		return "", nil, fmt.Errorf("invalid data URL: missing comma separator")
	}
	header := s[len("data:"):comma]
	body := s[comma+1:]

	isBase64 := strings.HasSuffix(header, ";base64")
	mimeType = strings.TrimSuffix(header, ";base64")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	if !isBase64 {
		decodedStr, uerr := url.QueryUnescape(body)
		if uerr != nil {
			return "", nil, fmt.Errorf("invalid data URL encoding: %w", uerr)
		}
		return mimeType, []byte(decodedStr), nil
	}

	raw, derr := base64.StdEncoding.DecodeString(body)
	if derr != nil {
		return "", nil, fmt.Errorf("invalid base64 in data URL: %w", derr)
	}
	return mimeType, raw, nil
}

// decodeAndCompressImage приводит фотографию к разумному размеру: если она
// уже компактная — возвращает как есть, иначе уменьшает по большей стороне
// до maxImageDimension и подбирает качество JPEG так, чтобы уложиться
// примерно в maxImageBytes. Возвращает готовые к записи на диск байты файла
// и расширение, с которым его следует сохранить.
func decodeAndCompressImage(dataURL string) (data []byte, ext string, err error) {
	mimeType, raw, err := parseDataURL(dataURL)
	if err != nil {
		return nil, "", err
	}

	if len(raw) <= skipIfUnderBytes && mimeType == "image/jpeg" {
		return raw, ".jpg", nil
	}

	img, _, decErr := image.Decode(bytes.NewReader(raw))
	if decErr != nil {
		// Формат, который стандартная библиотека не умеет декодировать
		// (например, HEIC/WebP) — лучше сохранить фото как есть, чем
		// потерять его из-за неудавшегося сжатия.
		return raw, extForMime(mimeType), nil
	}

	img = maybeResize(img, maxImageDimension)

	encoded, encErr := encodeJPEGWithBudget(img, maxImageBytes)
	if encErr != nil {
		return nil, "", fmt.Errorf("failed to compress image: %w", encErr)
	}

	return encoded, ".jpg", nil
}

// extForMime подбирает расширение файла для форматов, которые мы не умеем
// перекодировать в JPEG (см. decodeAndCompressImage выше).
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

// encodeJPEGWithBudget кодирует изображение в JPEG, постепенно снижая
// качество, пока результат не уложится в budget байт или пока не будет
// достигнут минимально допустимый уровень качества.
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

// maybeResize уменьшает изображение, если хотя бы одна сторона превышает
// maxDim, сохраняя пропорции. Если изображение уже укладывается — возвращает
// его без изменений.
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

// resizeBilinear — простое билинейное масштабирование без внешних
// зависимостей. Для уменьшения фотографий вещей этого достаточно по качеству
// и не требует тянуть в проект дополнительные пакеты вроде x/image/draw.
func resizeBilinear(src image.Image, newW, newH int) image.Image {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	if srcW < 2 || srcH < 2 {
		// Слишком маленький источник для билинейной интерполяции — просто
		// растягиваем ближайшим соседом, чтобы не делить на ноль.
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

// bilerp линейно интерполирует цвет по четырём соседним пикселям.
func bilerp(c00, c10, c01, c11 color.Color, fx, fy float64) color.RGBA {
	r00, g00, b00, a00 := c00.RGBA()
	r10, g10, b10, a10 := c10.RGBA()
	r01, g01, b01, a01 := c01.RGBA()
	r11, g11, b11, a11 := c11.RGBA()

	lerp := func(a, b uint32, f float64) float64 {
		return float64(a) + (float64(b)-float64(a))*f
	}

	// Сначала интерполируем по X в верхней и нижней строке, затем по Y между ними.
	rTop, rBot := lerp(r00, r10, fx), lerp(r01, r11, fx)
	gTop, gBot := lerp(g00, g10, fx), lerp(g01, g11, fx)
	bTop, bBot := lerp(b00, b10, fx), lerp(b01, b11, fx)
	aTop, aBot := lerp(a00, a10, fx), lerp(a01, a11, fx)

	r := lerpF(rTop, rBot, fy)
	g := lerpF(gTop, gBot, fy)
	b := lerpF(bTop, bBot, fy)
	a := lerpF(aTop, aBot, fy)

	// RGBA() возвращает компоненты в диапазоне [0, 65535] — приводим к [0, 255].
	return color.RGBA{
		R: uint8(clamp255(r / 257)),
		G: uint8(clamp255(g / 257)),
		B: uint8(clamp255(b / 257)),
		A: uint8(clamp255(a / 257)),
	}
}

func lerpF(a, b, f float64) float64 {
	return a + (b-a)*f
}

func clamp255(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
