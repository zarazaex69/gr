package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/zarazaex69/gr"
)

const (
	chunkSize    = 2953          // max QR payload at Low ECC
	targetFPS    = 60
	frameBudget  = time.Second / targetFPS // 16.67ms
	totalData    = 10 * 1024 * 1024        // 10 MB
)

func main() {
	// Generate 10 MB of random data
	data := make([]byte, totalData)
	rand.Read(data)

	// Split into QR-sized chunks
	var chunks [][]byte
	for off := 0; off < len(data); off += chunkSize {
		end := off + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[off:end])
	}

	fmt.Printf("=== gr QR throughput bench ===\n")
	fmt.Printf("Payload:       %.2f MB (%d bytes)\n", float64(totalData)/1024/1024, totalData)
	fmt.Printf("Chunk size:    %d bytes (QR v40, ECC Low)\n", chunkSize)
	fmt.Printf("Total chunks:  %d\n", len(chunks))
	fmt.Printf("Frame budget:  %.2fms @ %d fps\n\n", float64(frameBudget)/float64(time.Millisecond), targetFPS)

	var (
		totalEncode  time.Duration
		totalDecode  time.Duration
		slowFrames   int
		maxRoundTrip time.Duration
	)

	for i, chunk := range chunks {
		// --- Encode ---
		t0 := time.Now()
		frame, err := gr.Encode(chunk)
		encodeTime := time.Since(t0)
		if err != nil {
			fmt.Printf("ENCODE ERROR chunk %d: %v\n", i, err)
			return
		}

		// --- Decode ---
		t1 := time.Now()
		got, err := gr.Decode(frame)
		decodeTime := time.Since(t1)
		if err != nil {
			fmt.Printf("DECODE ERROR chunk %d: %v\n", i, err)
			return
		}

		// --- Verify ---
		if !bytes.Equal(got, chunk) {
			fmt.Printf("MISMATCH chunk %d (len got=%d want=%d)\n", i, len(got), len(chunk))
			return
		}

		roundTrip := encodeTime + decodeTime
		totalEncode += encodeTime
		totalDecode += decodeTime
		if roundTrip > maxRoundTrip {
			maxRoundTrip = roundTrip
		}
		if roundTrip > frameBudget {
			slowFrames++
		}
	}

	n := len(chunks)
	avgEncode := totalEncode / time.Duration(n)
	avgDecode := totalDecode / time.Duration(n)
	avgRoundTrip := (totalEncode + totalDecode) / time.Duration(n)

	// Actual achievable fps given average round-trip
	achievedFPS := float64(time.Second) / float64(avgRoundTrip)
	throughputMBps := float64(chunkSize) * achievedFPS / 1024 / 1024
	transferTime10MB := float64(totalData) / (throughputMBps * 1024 * 1024)

	fmt.Printf("Results (%d chunks, all verified ✓)\n", n)
	fmt.Printf("  Avg encode:      %6.2fms\n", float64(avgEncode)/float64(time.Millisecond))
	fmt.Printf("  Avg decode:      %6.2fms\n", float64(avgDecode)/float64(time.Millisecond))
	fmt.Printf("  Avg round-trip:  %6.2fms\n", float64(avgRoundTrip)/float64(time.Millisecond))
	fmt.Printf("  Max round-trip:  %6.2fms\n", float64(maxRoundTrip)/float64(time.Millisecond))
	fmt.Printf("  Slow frames:     %d / %d (>16.67ms)\n", slowFrames, n)
	fmt.Println()
	fmt.Printf("  Achieved FPS:    %.1f\n", achievedFPS)
	fmt.Printf("  Throughput:      %.2f MB/s\n", throughputMBps)
	fmt.Printf("  10 MB transfer:  %.1f seconds\n", transferTime10MB)

	if avgRoundTrip <= frameBudget {
		fmt.Println("\n  ✓ fits in 60fps budget")
	} else {
		fmt.Printf("\n  ✗ over budget by %.2fms\n",
			float64(avgRoundTrip-frameBudget)/float64(time.Millisecond))
	}
}
