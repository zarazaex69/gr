package tile_test

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/zarazaex69/gr/tile"
)

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func TestRoundTripDefault(t *testing.T) {
	c, err := tile.New(tile.DefaultConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", c.Info())
	for _, size := range []int{1, 100, 1000, c.MaxPayload()} {
		payload := randBytes(size)
		frame, err := c.Encode(payload, 42, 100)
		if err != nil {
			t.Fatalf("Encode(%d): %v", size, err)
		}
		got, err := c.Decode(frame)
		if err != nil {
			t.Fatalf("Decode(%d): %v", size, err)
		}
		if !bytes.Equal(got.Payload, payload) {
			t.Fatalf("mismatch size=%d", size)
		}
	}
}

func TestRoundTripVariousModules(t *testing.T) {
	for _, mod := range []int{1, 2, 4, 8, 16, 32, 64, 128, 270} {
		for _, rsPct := range []int{0, 20, 50} {
			c, err := tile.New(tile.Config{Module: mod, RSPercent: rsPct})
			if err != nil {
				t.Logf("module=%d rs=%d%%: skip (%v)", mod, rsPct, err)
				continue
			}
			payload := randBytes(min(c.MaxPayload(), 512))
			frame, err := c.Encode(payload, 0, 1)
			if err != nil {
				t.Fatalf("module=%d rs=%d%% Encode: %v", mod, rsPct, err)
			}
			got, err := c.Decode(frame)
			if err != nil {
				t.Fatalf("module=%d rs=%d%% Decode: %v", mod, rsPct, err)
			}
			if !bytes.Equal(got.Payload, payload) {
				t.Fatalf("module=%d rs=%d%% mismatch", mod, rsPct)
			}
			t.Logf("module=%3dpx  rs=%3d%%  %s", mod, rsPct, c.Info())
		}
	}
}

func TestThroughput10MB(t *testing.T) {
	configs := []tile.Config{
		{Module: 1, RSPercent: 0},
		{Module: 2, RSPercent: 0},
		{Module: 4, RSPercent: 0},
		{Module: 4, RSPercent: 20},
	}
	for _, cfg := range configs {
		c, _ := tile.New(cfg)
		bench10MB(t, c)
	}
}

func bench10MB(t *testing.T, c *tile.Codec) {
	const totalData = 10 * 1024 * 1024
	data := randBytes(totalData)

	var chunks [][]byte
	for off := 0; off < len(data); off += c.MaxPayload() {
		end := off + c.MaxPayload()
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[off:end])
	}

	totalFrames := uint32(len(chunks))
	var encTotal, decTotal time.Duration

	for i, chunk := range chunks {
		t0 := time.Now()
		frame, _ := c.Encode(chunk, uint32(i), totalFrames)
		encTotal += time.Since(t0)

		t1 := time.Now()
		got, err := c.Decode(frame)
		decTotal += time.Since(t1)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if !bytes.Equal(got.Payload, chunk) {
			t.Fatalf("frame %d mismatch", i)
		}
	}

	n := len(chunks)
	avgRT := (encTotal + decTotal) / time.Duration(n)
	fps := float64(time.Second) / float64(avgRT)
	mbps := float64(c.MaxPayload()) * fps / 1024 / 1024

	fmt.Printf("%-45s  frames=%-5d  rt=%-8s  fps=%-7.1f  %.2f MB/s  10MB=%.1fs\n",
		c.Info()[:min(45, len(c.Info()))],
		n,
		avgRT.Round(time.Microsecond),
		fps,
		mbps,
		float64(totalData)/float64(c.MaxPayload())/fps,
	)
}

func BenchmarkEncode4x4(b *testing.B) {
	c, _ := tile.New(tile.DefaultConfig)
	payload := randBytes(c.MaxPayload())
	b.ResetTimer()
	for b.Loop() {
		c.Encode(payload, 0, 1)
	}
}

func BenchmarkDecode4x4(b *testing.B) {
	c, _ := tile.New(tile.DefaultConfig)
	payload := randBytes(c.MaxPayload())
	frame, _ := c.Encode(payload, 0, 1)
	b.ResetTimer()
	for b.Loop() {
		c.Decode(frame)
	}
}
