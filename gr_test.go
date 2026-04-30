package gr_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/zarazaex69/gr"
)

func randomPayload(n int) []byte {
	p := make([]byte, n)
	_, _ = rand.Read(p)
	return p
}

func TestRoundTrip(t *testing.T) {
	for _, size := range []int{64, 512, 1024, 1500} {
		payload := randomPayload(size)
		frame, err := gr.Encode(payload)
		if err != nil {
			t.Fatalf("Encode(%d): %v", size, err)
		}
		got, err := gr.Decode(frame)
		if err != nil {
			t.Fatalf("Decode(%d): %v", size, err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("Decode(%d): roundtrip mismatch", size)
		}
	}
}

func TestEncodeBitmap(t *testing.T) {
	payload := randomPayload(100)
	bmp, err := gr.EncodeBitmap(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(bmp) == 0 {
		t.Fatal("empty bitmap")
	}
}

// BenchmarkEncode1500 — target: < 8ms
func BenchmarkEncode1500(b *testing.B) {
	payload := randomPayload(1500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := gr.Encode(payload)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecode1500 — target: < 8ms
func BenchmarkDecode1500(b *testing.B) {
	payload := randomPayload(1500)
	frame, err := gr.Encode(payload)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := gr.Decode(frame)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRoundTrip1500 — target: < 16ms (60fps)
func BenchmarkRoundTrip1500(b *testing.B) {
	payload := randomPayload(1500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frame, err := gr.Encode(payload)
		if err != nil {
			b.Fatal(err)
		}
		_, err = gr.Decode(frame)
		if err != nil {
			b.Fatal(err)
		}
	}
}
