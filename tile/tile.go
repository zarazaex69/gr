package tile

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"

	"github.com/klauspost/reedsolomon"
)

const (
	FrameW     = 1080
	FrameH     = 1080
	Black      = byte(64)
	White      = byte(192)
	headerSize = 20
	magic      = uint32(0x544C4532)
)

// Config configures a Codec instance.
type Config struct {
	// Module is the pixel side of each bit-cell (1..270).
	// 1 = max density (145KB/frame, лол), 4 = VP8-safe default.
	Module int

	// RSPercent is Reed-Solomon parity overhead in percent of data shards (0..200).
	// 0 = no ECC, 20 = 20% parity overhead (recovers up to 10% shard loss), 100 = 2× redundancy.
	RSPercent int
}

// DefaultConfig is the VP8/WebRTC-tuned default: 4×4 modules, 20% RS parity.
var DefaultConfig = Config{Module: 4, RSPercent: 20}

// Codec encodes/decodes tile frames.
type Codec struct {
	cfg        Config
	cols, rows int
	syncRows   int // rows reserved top+bottom for sync band
	dataRows   int
	frameBytes int // total bytes in data area
	dataShards int // RS data shards
	parShards  int // RS parity shards
	maxPayload int // bytes of user data per frame after header+ECC overhead
	enc        reedsolomon.Encoder
}

// New creates a Codec from config.
func New(cfg Config) (*Codec, error) {
	if cfg.Module < 1 || cfg.Module > 270 {
		return nil, fmt.Errorf("tile: module must be 1..270, got %d", cfg.Module)
	}
	if cfg.RSPercent < 0 || cfg.RSPercent > 200 {
		return nil, fmt.Errorf("tile: RSPercent must be 0..200, got %d", cfg.RSPercent)
	}

	cols := FrameW / cfg.Module
	rows := FrameH / cfg.Module

	// Sync band: 2 module-rows top + 2 bottom (or 0 if module is so big it doesn't fit)
	syncRows := 0
	if rows > 4 {
		syncRows = 2
	}
	dataRows := rows - syncRows*2
	frameBytes := (cols * dataRows) / 8

	if frameBytes <= headerSize {
		return nil, fmt.Errorf("tile: module %d too large — frame holds only %d bytes", cfg.Module, frameBytes)
	}

	// RS sharding: treat payload area as N shards of ~256 bytes each
	// dataShards * shardSize = frameBytes - headerSize (approx)
	dataShards, parShards := rsShards(frameBytes-headerSize, cfg.RSPercent)

	// Recalculate max payload given shard geometry
	shardSize := shardBytes(frameBytes-headerSize, dataShards)
	maxPayload := dataShards*shardSize - headerSize
	if maxPayload <= 0 {
		return nil, fmt.Errorf("tile: module too large for RS config")
	}

	var enc reedsolomon.Encoder
	if parShards > 0 {
		var err error
		enc, err = reedsolomon.New(dataShards, parShards)
		if err != nil {
			return nil, fmt.Errorf("tile: reedsolomon.New(%d,%d): %w", dataShards, parShards, err)
		}
	}

	return &Codec{
		cfg:        cfg,
		cols:       cols,
		rows:       rows,
		syncRows:   syncRows,
		dataRows:   dataRows,
		frameBytes: frameBytes,
		dataShards: dataShards,
		parShards:  parShards,
		maxPayload: maxPayload,
		enc:        enc,
	}, nil
}

// MaxPayload returns the max user bytes per frame.
func (c *Codec) MaxPayload() int { return c.maxPayload }

// Info prints codec parameters.
func (c *Codec) Info() string {
	fps60 := float64(c.maxPayload) * 60 / 1024 / 1024
	return fmt.Sprintf("tile: module=%dpx  grid=%dx%d  payload=%d B/frame  %.2f MB/s @60fps  RS=%d+%d",
		c.cfg.Module, c.cols, c.rows, c.maxPayload, fps60, c.dataShards, c.parShards)
}

// Encode packs payload into a 1080×1080 grayscale frame.
func (c *Codec) Encode(payload []byte, frameID, totalFrames uint32) ([]byte, error) {
	if len(payload) > c.maxPayload {
		return nil, fmt.Errorf("tile: payload %d > maxPayload %d", len(payload), c.maxPayload)
	}

	shardSz := shardBytes(c.frameBytes-headerSize, c.dataShards)
	totalData := c.dataShards * shardSz

	// Build raw data block: header + payload
	raw := make([]byte, totalData+headerSize)
	binary.BigEndian.PutUint32(raw[0:], magic)
	binary.BigEndian.PutUint32(raw[4:], frameID)
	binary.BigEndian.PutUint32(raw[8:], totalFrames)
	binary.BigEndian.PutUint32(raw[12:], uint32(len(payload)))
	copy(raw[headerSize:], payload)
	crcBuf := make([]byte, 16+len(payload))
	copy(crcBuf, raw[:16])
	copy(crcBuf[16:], payload)
	binary.BigEndian.PutUint32(raw[16:], crc32.ChecksumIEEE(crcBuf))

	// RS encode
	var wire []byte
	if c.parShards > 0 {
		shards := splitShards(raw, c.dataShards, shardSz)
		parity := make([][]byte, c.parShards)
		for i := range parity {
			parity[i] = make([]byte, shardSz)
		}
		all := append(shards, parity...)
		if err := c.enc.Encode(all); err != nil {
			return nil, fmt.Errorf("tile: rs encode: %w", err)
		}
		wire = joinShards(all)
	} else {
		wire = raw
	}

	// Pad/trim to frameBytes
	frame := c.renderFrame(wire)
	return frame, nil
}

// DecodeResult holds decoded frame data.
type DecodeResult struct {
	FrameID     uint32
	TotalFrames uint32
	Payload     []byte
}

// Decode extracts payload from a 1080×1080 grayscale frame.
func (c *Codec) Decode(frame []byte) (*DecodeResult, error) {
	if len(frame) != FrameW*FrameH {
		return nil, fmt.Errorf("tile: expected %d bytes frame, got %d", FrameW*FrameH, len(frame))
	}

	wire := c.readFrame(frame)
	shardSz := shardBytes(c.frameBytes-headerSize, c.dataShards)

	var raw []byte
	if c.parShards > 0 {
		totalShards := c.dataShards + c.parShards
		all := splitShards(wire, totalShards, shardSz)
		ok, err := c.enc.Verify(all)
		if err != nil || !ok {
			if err := c.enc.Reconstruct(all); err != nil {
				return nil, fmt.Errorf("tile: rs reconstruct: %w", err)
			}
		}
		raw = joinShards(all[:c.dataShards])
	} else {
		raw = wire
	}

	if binary.BigEndian.Uint32(raw[0:]) != magic {
		return nil, fmt.Errorf("tile: bad magic 0x%08X", binary.BigEndian.Uint32(raw[0:]))
	}
	frameID := binary.BigEndian.Uint32(raw[4:])
	totalFrames := binary.BigEndian.Uint32(raw[8:])
	payloadLen := binary.BigEndian.Uint32(raw[12:])
	if int(payloadLen) > c.maxPayload {
		return nil, fmt.Errorf("tile: bad payloadLen %d", payloadLen)
	}
	gotCRC := binary.BigEndian.Uint32(raw[16:])
	crcBuf := make([]byte, 16+payloadLen)
	copy(crcBuf, raw[:16])
	copy(crcBuf[16:], raw[headerSize:headerSize+payloadLen])
	if wantCRC := crc32.ChecksumIEEE(crcBuf); gotCRC != wantCRC {
		return nil, fmt.Errorf("tile: CRC mismatch got=%08X want=%08X", gotCRC, wantCRC)
	}

	out := make([]byte, payloadLen)
	copy(out, raw[headerSize:])
	return &DecodeResult{
		FrameID:     frameID,
		TotalFrames: totalFrames,
		Payload:     out,
	}, nil
}

// --- render / read ---

func (c *Codec) renderFrame(wire []byte) []byte {
	frame := make([]byte, FrameW*FrameH)
	for i := range frame {
		frame[i] = White
	}
	c.renderSync(frame, 0, c.syncRows)
	c.renderSync(frame, c.rows-c.syncRows, c.rows)

	m := c.cfg.Module
	maxMod := c.cols * c.dataRows

	for byteIdx := 0; byteIdx < len(wire) && byteIdx*8 < maxMod; byteIdx++ {
		b := wire[byteIdx]
		if b == 0 {
			continue // all white, already filled
		}
		for bit := 0; bit < 8; bit++ {
			if (b>>(7-bit))&1 == 0 {
				continue
			}
			modIdx := byteIdx*8 + bit
			if modIdx >= maxMod {
				break
			}
			col := modIdx % c.cols
			row := c.syncRows + modIdx/c.cols
			x0, y0 := col*m, row*m
			for dy := 0; dy < m; dy++ {
				base := (y0+dy)*FrameW + x0
				// fill m pixels in one shot
				for dx := 0; dx < m; dx++ {
					frame[base+dx] = Black
				}
			}
		}
	}
	return frame
}

func (c *Codec) readFrame(frame []byte) []byte {
	m := c.cfg.Module
	half := m / 2
	if half == 0 {
		half = 0 // 1px module: sample at (0,0)
	}

	totalMods := c.cols * c.dataRows
	out := make([]byte, (totalMods+7)/8)
	for modIdx := 0; modIdx < totalMods; modIdx++ {
		col := modIdx % c.cols
		row := c.syncRows + modIdx/c.cols
		cx := col*m + half
		cy := row*m + half
		if cx >= FrameW {
			cx = FrameW - 1
		}
		if cy >= FrameH {
			cy = FrameH - 1
		}
		if frame[cy*FrameW+cx] < 128 {
			out[modIdx/8] |= 1 << (7 - modIdx%8)
		}
	}
	return out
}

func (c *Codec) renderSync(frame []byte, rowStart, rowEnd int) {
	m := c.cfg.Module
	for row := rowStart; row < rowEnd; row++ {
		for col := 0; col < c.cols; col++ {
			color := White
			if col%2 == 0 {
				color = Black
			}
			x0, y0 := col*m, row*m
			for dy := 0; dy < m; dy++ {
				base := (y0+dy)*FrameW + x0
				for dx := 0; dx < m; dx++ {
					frame[base+dx] = color
				}
			}
		}
	}
}

// --- RS helpers ---

func rsShards(dataBytes, pctParity int) (data, parity int) {
	if pctParity == 0 {
		return 1, 0
	}
	// target ~256-byte shards
	data = max(1, dataBytes/256)
	// cap at 256 (reedsolomon limit per GF(2^8))
	data = min(data, 256-int(math.Ceil(float64(data)*float64(pctParity)/100)))
	if data < 1 {
		data = 1
	}
	parity = int(math.Ceil(float64(data) * float64(pctParity) / 100))
	if parity < 1 {
		parity = 1
	}
	// reedsolomon requires data+parity <= 256
	for data+parity > 256 {
		data--
	}
	return data, parity
}

func shardBytes(totalBytes, dataShards int) int {
	return (totalBytes + dataShards - 1) / dataShards
}

func splitShards(data []byte, n, shardSz int) [][]byte {
	shards := make([][]byte, n)
	for i := range shards {
		start := i * shardSz
		end := start + shardSz
		shard := make([]byte, shardSz)
		if start < len(data) {
			if end > len(data) {
				end = len(data)
			}
			copy(shard, data[start:end])
		}
		shards[i] = shard
	}
	return shards
}

func joinShards(shards [][]byte) []byte {
	total := 0
	for _, s := range shards {
		total += len(s)
	}
	out := make([]byte, 0, total)
	for _, s := range shards {
		out = append(out, s...)
	}
	return out
}
