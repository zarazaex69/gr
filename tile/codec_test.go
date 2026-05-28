package tile

import "testing"

func BenchmarkEncode(b *testing.B) {
	c, _ := New(DefaultConfig)
	payload := make([]byte, c.MaxPayload())
	for i := range payload {
		payload[i] = byte(i)
	}
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Encode(payload, uint32(i), 100)
	}
}

func BenchmarkDecode(b *testing.B) {
	c, _ := New(DefaultConfig)
	payload := make([]byte, c.MaxPayload())
	for i := range payload {
		payload[i] = byte(i)
	}
	frame, _ := c.Encode(payload, 1, 100)
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Decode(frame)
	}
}
