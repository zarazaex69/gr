// Package gr provides high-speed QR encode/decode targeting <8ms each at ~1500 bytes.
// Encode: payload → 1080×1080 grayscale frame (or [][]bool bitmap).
// Decode: 1080×1080 grayscale frame → payload.
// Error correction: Low (max capacity ~2953 bytes binary mode).
package gr

import (
	"fmt"

	"github.com/makiuchi-d/gozxing"
	gzqr "github.com/makiuchi-d/gozxing/qrcode"
	rsciiqr "rsc.io/qr"
)

const (
	FrameWidth  = 1080
	FrameHeight = 1080
)

// Encode encodes payload into a 1080×1080 grayscale frame (1 byte/pixel, 0=black 255=white).
// The QR is centered with quiet zone. Uses Low ECC for maximum capacity (~2953 bytes).
func Encode(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("gr: empty payload")
	}
	code, err := rsciiqr.Encode(string(payload), rsciiqr.L)
	if err != nil {
		return nil, fmt.Errorf("gr: encode: %w", err)
	}
	// code.Size is the module count (excluding quiet zone already embedded).
	// rsc.io/qr includes a 4-module quiet zone in its bitmap coordinates.
	modules := code.Size + 8 // includes 4-module quiet zone on each side

	scale := (FrameWidth - 40) / modules
	if scale < 1 {
		scale = 1
	}
	qrSize := modules * scale
	offsetX := (FrameWidth - qrSize) / 2
	offsetY := (FrameHeight - qrSize) / 2

	frame := make([]byte, FrameWidth*FrameHeight)
	for i := range frame {
		frame[i] = 255
	}

	// rsc.io/qr renders with 4-module quiet zone offset: pixel (x,y) maps to module (x-4, y-4).
	// Black() accepts coordinates in [0, Size+8) where 0..3 and Size+4..Size+7 are quiet zone.
	for row := 0; row < modules; row++ {
		y0 := offsetY + row*scale
		for col := 0; col < modules; col++ {
			if !code.Black(col-4, row-4) {
				continue
			}
			x0 := offsetX + col*scale
			for dy := 0; dy < scale; dy++ {
				base := (y0+dy)*FrameWidth + x0
				for dx := 0; dx < scale; dx++ {
					frame[base+dx] = 0
				}
			}
		}
	}
	return frame, nil
}

// EncodeBitmap returns a raw [][]bool module grid (including quiet zone) for callers
// that render themselves.
func EncodeBitmap(payload []byte) ([][]bool, error) {
	code, err := rsciiqr.Encode(string(payload), rsciiqr.L)
	if err != nil {
		return nil, err
	}
	size := code.Size + 8
	bmp := make([][]bool, size)
	for row := range bmp {
		bmp[row] = make([]bool, size)
		for col := range bmp[row] {
			bmp[row][col] = code.Black(col-4, row-4)
		}
	}
	return bmp, nil
}

// Decode decodes a QR code from a 1080×1080 grayscale frame.
// Uses PURE_BARCODE hint — assumes QR is cleanly rendered (no adaptive threshold).
func Decode(frame []byte) ([]byte, error) {
	if len(frame) != FrameWidth*FrameHeight {
		return nil, fmt.Errorf("gr: decode: expected %d bytes, got %d", FrameWidth*FrameHeight, len(frame))
	}

	src := &grayLuminance{pix: frame, w: FrameWidth, h: FrameHeight}

	// Fixed-threshold binarizer: QR is cleanly rendered with pure black/white pixels,
	// so histogram analysis is unnecessary — threshold at 128 is optimal and ~3× faster.
	binarizer := newFixedThresholdBinarizer(src, 128)
	bmp, err := gozxing.NewBinaryBitmap(binarizer)
	if err != nil {
		return nil, fmt.Errorf("gr: decode: bitmap: %w", err)
	}

	hints := map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_PURE_BARCODE: true,
	}
	reader := gzqr.NewQRCodeReader()
	result, err := reader.Decode(bmp, hints)
	if err != nil {
		// Fallback: GlobalHistogram for slightly distorted frames.
		bmp2, _ := gozxing.NewBinaryBitmap(gozxing.NewGlobalHistgramBinarizer(src))
		result, err = reader.Decode(bmp2, nil)
		if err != nil {
			return nil, fmt.Errorf("gr: decode: %w", err)
		}
	}
	return extractBytes(result), nil
}

// extractBytes recovers the original binary payload from a decoded QR result.
// gozxing re-encodes byte-mode segments through charset, so we use BYTE_SEGMENTS metadata.
func extractBytes(result interface {
	GetResultMetadata() map[gozxing.ResultMetadataType]interface{}
	GetText() string
}) []byte {
	meta := result.GetResultMetadata()
	if segs, ok := meta[gozxing.ResultMetadataType_BYTE_SEGMENTS]; ok {
		if parts, ok := segs.([][]byte); ok && len(parts) > 0 {
			if len(parts) == 1 {
				return parts[0]
			}
			total := 0
			for _, p := range parts {
				total += len(p)
			}
			out := make([]byte, 0, total)
			for _, p := range parts {
				out = append(out, p...)
			}
			return out
		}
	}
	return []byte(result.GetText())
}

// --- gozxing LuminanceSource backed by flat grayscale frame (zero-copy) ---

type grayLuminance struct {
	pix  []byte
	w, h int
}

func (g *grayLuminance) GetWidth() int  { return g.w }
func (g *grayLuminance) GetHeight() int { return g.h }

func (g *grayLuminance) GetRow(y int, row []byte) ([]byte, error) {
	if y < 0 || y >= g.h {
		return nil, fmt.Errorf("gr: row %d out of bounds [0, %d)", y, g.h)
	}
	start := y * g.w
	if len(row) < g.w {
		row = make([]byte, g.w)
	}
	copy(row, g.pix[start:start+g.w])
	return row[:g.w], nil
}

// GetMatrix returns the backing slice directly — zero allocation.
func (g *grayLuminance) GetMatrix() []byte { return g.pix }

func (g *grayLuminance) IsCropSupported() bool                           { return false }
func (g *grayLuminance) Crop(_, _, _, _ int) (gozxing.LuminanceSource, error) {
	return nil, fmt.Errorf("gr: crop not supported")
}
func (g *grayLuminance) IsRotateSupported() bool { return false }
func (g *grayLuminance) RotateCounterClockwise() (gozxing.LuminanceSource, error) {
	return nil, fmt.Errorf("gr: rotate not supported")
}
func (g *grayLuminance) RotateCounterClockwise45() (gozxing.LuminanceSource, error) {
	return nil, fmt.Errorf("gr: rotate not supported")
}
func (g *grayLuminance) Invert() gozxing.LuminanceSource {
	return gozxing.NewInvertedLuminanceSource(g)
}
func (g *grayLuminance) String() string {
	return fmt.Sprintf("GrayLuminance(%dx%d)", g.w, g.h)
}

// --- Fixed-threshold binarizer for perfectly rendered QR frames ---
// Skips histogram analysis — 128 is the optimal threshold for black=0 / white=255 frames.

type fixedThresholdBinarizer struct {
	src       gozxing.LuminanceSource
	threshold int
}

func newFixedThresholdBinarizer(src gozxing.LuminanceSource, threshold int) gozxing.Binarizer {
	return &fixedThresholdBinarizer{src: src, threshold: threshold}
}

func (b *fixedThresholdBinarizer) GetLuminanceSource() gozxing.LuminanceSource { return b.src }
func (b *fixedThresholdBinarizer) GetWidth() int                                { return b.src.GetWidth() }
func (b *fixedThresholdBinarizer) GetHeight() int                               { return b.src.GetHeight() }

func (b *fixedThresholdBinarizer) CreateBinarizer(src gozxing.LuminanceSource) gozxing.Binarizer {
	return newFixedThresholdBinarizer(src, b.threshold)
}

func (b *fixedThresholdBinarizer) GetBlackRow(y int, row *gozxing.BitArray) (*gozxing.BitArray, error) {
	w := b.src.GetWidth()
	if row == nil || row.GetSize() < w {
		row = gozxing.NewBitArray(w)
	} else {
		row.Clear()
	}
	lum, err := b.src.GetRow(y, nil)
	if err != nil {
		return nil, err
	}
	for x := 0; x < w; x++ {
		if int(lum[x]&0xff) < b.threshold {
			row.Set(x)
		}
	}
	return row, nil
}

func (b *fixedThresholdBinarizer) GetBlackMatrix() (*gozxing.BitMatrix, error) {
	w := b.src.GetWidth()
	h := b.src.GetHeight()
	matrix, err := gozxing.NewBitMatrix(w, h)
	if err != nil {
		return nil, err
	}
	lum := b.src.GetMatrix()
	for y := 0; y < h; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			if int(lum[row+x]&0xff) < b.threshold {
				matrix.Set(x, y)
			}
		}
	}
	return matrix, nil
}
