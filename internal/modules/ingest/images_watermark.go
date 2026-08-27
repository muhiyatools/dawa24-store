package ingest

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"strings"
)

// Simple 5x7 bitmap font for "DAWA 24"
var glyphs5x7 = map[rune][]string{
	'D': {
		"11110",
		"10001",
		"10001",
		"10001",
		"10001",
		"10001",
		"11110",
	},
	'A': {
		"01110",
		"10001",
		"10001",
		"11111",
		"10001",
		"10001",
		"10001",
	},
	'W': {
		"10001",
		"10001",
		"10001",
		"10101",
		"10101",
		"11011",
		"10001",
	},
	'2': {
		"01110",
		"10001",
		"00001",
		"00110",
		"01000",
		"10000",
		"11111",
	},
	'4': {
		"00010",
		"00110",
		"01010",
		"10010",
		"11111",
		"00010",
		"00010",
	},
	' ': {
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
	},
	'•': {
		"00000",
		"00000",
		"01110",
		"01110",
		"01110",
		"00000",
		"00000",
	},
}

// ApplyWatermark adds a clean, professional semi-transparent "DAWA24" watermark badge
// to the bottom-right corner of the image before saving.
func ApplyWatermark(imgData []byte, ext string) ([]byte, error) {
	if len(imgData) == 0 {
		return imgData, nil
	}

	src, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		// If decoding fails, fallback gracefully to original data
		return imgData, nil
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// If image is too small, do not watermark
	if width < 80 || height < 60 {
		return imgData, nil
	}

	// Create RGBA canvas
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)

	// Determine badge scale and position
	scale := 1
	if width >= 800 || height >= 800 {
		scale = 2
	}
	if width >= 1600 || height >= 1600 {
		scale = 3
	}

	text := "DAWA 24"
	charW := 6 * scale
	charH := 7 * scale
	textWidth := len(text) * charW

	padX := 10 * scale
	padY := 6 * scale
	badgeW := textWidth + (padX * 2)
	badgeH := charH + (padY * 2)

	margin := 12 * scale
	startX := width - badgeW - margin
	startY := height - badgeH - margin

	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	// Draw badge backdrop: semi-transparent slate (RGBA 15, 23, 42, 160)
	bgColor := color.NRGBA{R: 15, G: 23, B: 42, A: 170}
	for y := startY; y < startY+badgeH && y < height; y++ {
		for x := startX; x < startX+badgeW && x < width; x++ {
			// Subtle rounded corners
			if (x == startX || x == startX+badgeW-1) && (y == startY || y == startY+badgeH-1) {
				continue
			}
			dst.Set(x, y, blend(dst.At(x, y), bgColor))
		}
	}

	// Draw text: Crisp white/gold text (RGBA 255, 255, 255, 230)
	textColor := color.NRGBA{R: 255, G: 255, B: 255, A: 235}
	accentColor := color.NRGBA{R: 56, G: 189, B: 248, A: 240} // Cyan accent for 24

	curX := startX + padX
	curY := startY + padY

	for _, ch := range text {
		glyph, ok := glyphs5x7[ch]
		if !ok {
			glyph = glyphs5x7[' ']
		}
		c := textColor
		if ch == '2' || ch == '4' {
			c = accentColor
		}

		for r, row := range glyph {
			for colIdx, pixel := range row {
				if pixel == '1' {
					for dy := 0; dy < scale; dy++ {
						for dx := 0; dx < scale; dx++ {
							px := curX + (colIdx * scale) + dx
							py := curY + (r * scale) + dy
							if px < width && py < height {
								dst.Set(px, py, blend(dst.At(px, py), c))
							}
						}
					}
				}
			}
		}
		curX += charW
	}

	// Encode to output buffer
	var outBuf bytes.Buffer
	cleanExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	if cleanExt == "png" {
		if err := png.Encode(&outBuf, dst); err != nil {
			return imgData, nil
		}
	} else {
		// Default to JPEG with high quality 90
		if err := jpeg.Encode(&outBuf, dst, &jpeg.Options{Quality: 90}); err != nil {
			return imgData, nil
		}
	}

	return outBuf.Bytes(), nil
}

func blend(base color.Color, top color.NRGBA) color.Color {
	br, bg, bb, ba := base.RGBA()
	// Convert from 0..65535 to 0..255
	r1, g1, b1, a1 := float64(br>>8), float64(bg>>8), float64(bb>>8), float64(ba>>8)
	r2, g2, b2, a2 := float64(top.R), float64(top.G), float64(top.B), float64(top.A)

	alpha := a2 / 255.0
	outR := uint8(r2*alpha + r1*(1.0-alpha))
	outG := uint8(g2*alpha + g1*(1.0-alpha))
	outB := uint8(b2*alpha + b1*(1.0-alpha))
	outA := uint8(a1)
	if outA < 255 {
		outA = 255
	}

	return color.RGBA{R: outR, G: outG, B: outB, A: outA}
}
