package tile_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/zarazaex69/gr/tile"
)

func TestRoundTrip(t *testing.T) {
	for _, size := range []int{1, 100, 1000, tile.MaxPayload} {
		payload := make([]byte, size)
		rand.Read(payload)

		frame, err := tile.Encode(payload, 0, 1)
		if err != nil {
			t.Fatalf("Encode(%d): %v", size, err)
		}
		got, err := tile.Decode(frame)
		if err != nil {
			t.Fatalf("Decode(%d): %v", size, err)
		}
		if !bytes.Equal(got.Payload, payload) {
			t.Fatalf("mismatch at size %d", size)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	payload := make([]byte, tile.MaxPayload)
	rand.Read(payload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tile.Encode(payload, uint32(i), 1000)
	}
}

func BenchmarkDecode(b *testing.B) {
	payload := make([]byte, tile.MaxPayload)
	rand.Read(payload)
	frame, _ := tile.Encode(payload, 0, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tile.Decode(frame)
	}
}

func TestThroughput10MB(t *testing.T) {
	const totalData = 10 * 1024 * 1024
	data := make([]byte, totalData)
	rand.Read(data)

	var chunks [][]byte
	for off := 0; off < len(data); off += tile.MaxPayload {
		end := off + tile.MaxPayload
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[off:end])
	}

	totalFrames := uint32(len(chunks))
	var encTotal, decTotal time.Duration

	for i, chunk := range chunks {
		t0 := time.Now()
		frame, err := tile.Encode(chunk, uint32(i), totalFrames)
		encTotal += time.Since(t0)
		if err != nil {
			t.Fatalf("frame %d encode: %v", i, err)
		}

		t1 := time.Now()
		got, err := tile.Decode(frame)
		decTotal += time.Since(t1)
		if err != nil {
			t.Fatalf("frame %d decode: %v", i, err)
		}
		if !bytes.Equal(got.Payload, chunk) {
			t.Fatalf("frame %d mismatch", i)
		}
	}

	n := len(chunks)
	avgEnc := encTotal / time.Duration(n)
	avgDec := decTotal / time.Duration(n)
	avgRT := avgEnc + avgDec
	fps := float64(time.Second) / float64(avgRT)
	mbps := float64(tile.MaxPayload) * fps / 1024 / 1024
	t10 := float64(totalData) / (mbps * 1024 * 1024)

	fmt.Printf("\n=== tile 4×4 throughput (%d frames, all ✓) ===\n", n)
	fmt.Printf("  Payload/frame:  %d bytes (%.1f KB)\n", tile.MaxPayload, float64(tile.MaxPayload)/1024)
	fmt.Printf("  Avg encode:     %.3fms\n", float64(avgEnc)/1e6)
	fmt.Printf("  Avg decode:     %.3fms\n", float64(avgDec)/1e6)
	fmt.Printf("  Avg round-trip: %.3fms\n", float64(avgRT)/1e6)
	fmt.Printf("  Achieved FPS:   %.1f\n", fps)
	fmt.Printf("  Throughput:     %.2f MB/s\n", mbps)
	fmt.Printf("  10 MB in:       %.1f seconds\n", t10)
}
