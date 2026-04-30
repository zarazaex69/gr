// Package tile implements a raw 2D tile codec for screen-to-camera data transfer over WebRTC.
// Uses 4×4 pixel modules (VP8 min block boundary), soft gray levels 64/192 to survive
// DCT quantization, and CRC32 per frame for error detection.
//
// Throughput: ~8.6 KB/frame × 60fps = 0.50 MB/s → 10 MB in ~20s.
package tile

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	FrameW = 1080
	FrameH = 1080

	Module = 4 // pixels per module side — aligns with VP8 4×4 min block

	// Soft gray levels instead of 0/255 — gives ±64px headroom before codec quantization flips a bit.
	Black = 64
	White = 192

	cols       = FrameW / Module // 270
	rows       = FrameH / Module // 270
	syncRows   = 4               // top + bottom sync band (4 module rows each)
	dataRows   = rows - syncRows*2 // 262
	frameBytes = (cols * dataRows) / 8 // 8842

	headerSize = 20 // magic(4) + frameID(4) + totalFrames(4) + payloadLen(4) + crc32(4)
	MaxPayload = frameBytes - headerSize // 8822

	magic = uint32(0x544C4532) // "TLE2"
)

// Encode packs payload into a 1080×1080 grayscale frame.
// Rows 0..3 and 267..270 are sync bands; data occupies rows 4..266.
func Encode(payload []byte, frameID, totalFrames uint32) ([]byte, error) {
	if len(payload) > MaxPayload {
		return nil, fmt.Errorf("tile: payload %d > max %d", len(payload), MaxPayload)
	}

	// Build header + data block
	data := make([]byte, frameBytes)
	binary.BigEndian.PutUint32(data[0:], magic)
	binary.BigEndian.PutUint32(data[4:], frameID)
	binary.BigEndian.PutUint32(data[8:], totalFrames)
	binary.BigEndian.PutUint32(data[12:], uint32(len(payload)))
	copy(data[headerSize:], payload)
	// CRC covers: fields 0..15 (magic+frameID+totalFrames+payloadLen) + payload bytes
	crcBuf := make([]byte, 16+len(payload))
	copy(crcBuf, data[:16])
	copy(crcBuf[16:], payload)
	crc := crc32.ChecksumIEEE(crcBuf)
	binary.BigEndian.PutUint32(data[16:], crc)

	frame := make([]byte, FrameW*FrameH)
	// Fill white
	for i := range frame {
		frame[i] = White
	}

	// Render sync bands (top + bottom): alternating black/white columns, fixed pattern.
	// Used by decoder to verify it's reading the right frame type and find column alignment.
	renderSync(frame, 0, syncRows)
	renderSync(frame, rows-syncRows, rows)

	// Render data modules
	for byteIdx := 0; byteIdx < frameBytes; byteIdx++ {
		b := data[byteIdx]
		for bit := 0; bit < 8; bit++ {
			modIdx := byteIdx*8 + bit
			col := modIdx % cols
			row := syncRows + modIdx/cols
			if row >= rows-syncRows {
				break
			}
			var color byte
			if (b>>(7-bit))&1 == 1 {
				color = Black
			} else {
				color = White
			}
			fillModule(frame, col, row, color)
		}
	}
	return frame, nil
}

// DecodeResult holds decoded frame data.
type DecodeResult struct {
	FrameID     uint32
	TotalFrames uint32
	Payload     []byte
}

// Decode extracts payload from a 1080×1080 grayscale frame.
// Samples center pixel of each 4×4 module — the point least affected by VP8 DCT ringing.
func Decode(frame []byte) (*DecodeResult, error) {
	if len(frame) != FrameW*FrameH {
		return nil, fmt.Errorf("tile: expected %d bytes, got %d", FrameW*FrameH, len(frame))
	}

	data := make([]byte, frameBytes)
	for modIdx := 0; modIdx < cols*dataRows; modIdx++ {
		byteIdx := modIdx / 8
		bit := modIdx % 8
		col := modIdx % cols
		row := syncRows + modIdx/cols
		// Sample center pixel of module
		cx := col*Module + Module/2
		cy := row*Module + Module/2
		px := frame[cy*FrameW+cx]
		// Threshold at midpoint between Black and White
		if px < 128 { // midpoint between Black=64 and White=192
			data[byteIdx] |= 1 << (7 - bit)
		}
	}

	if binary.BigEndian.Uint32(data[0:]) != magic {
		return nil, fmt.Errorf("tile: bad magic 0x%08X", binary.BigEndian.Uint32(data[0:]))
	}
	frameID := binary.BigEndian.Uint32(data[4:])
	totalFrames := binary.BigEndian.Uint32(data[8:])
	payloadLen := binary.BigEndian.Uint32(data[12:])
	if int(payloadLen) > MaxPayload {
		return nil, fmt.Errorf("tile: bad payloadLen %d", payloadLen)
	}
	gotCRC := binary.BigEndian.Uint32(data[16:])
	crcBuf := make([]byte, 16+payloadLen)
	copy(crcBuf, data[:16])
	copy(crcBuf[16:], data[headerSize:headerSize+payloadLen])
	wantCRC := crc32.ChecksumIEEE(crcBuf)
	if gotCRC != wantCRC {
		return nil, fmt.Errorf("tile: CRC mismatch got=%08X want=%08X", gotCRC, wantCRC)
	}

	out := make([]byte, payloadLen)
	copy(out, data[headerSize:])
	return &DecodeResult{
		FrameID:     frameID,
		TotalFrames: totalFrames,
		Payload:     out,
	}, nil
}

func fillModule(frame []byte, col, row int, color byte) {
	x0 := col * Module
	y0 := row * Module
	for dy := 0; dy < Module; dy++ {
		base := (y0+dy)*FrameW + x0
		for dx := 0; dx < Module; dx++ {
			frame[base+dx] = color
		}
	}
}

// renderSync paints a striped sync band: alternating black/white modules per column.
// Column k is black if k is even, white if odd.
func renderSync(frame []byte, rowStart, rowEnd int) {
	for row := rowStart; row < rowEnd; row++ {
		for col := 0; col < cols; col++ {
			var color byte
			if col%2 == 0 {
				color = Black
			} else {
				color = White
			}
			fillModule(frame, col, row, color)
		}
	}
}
