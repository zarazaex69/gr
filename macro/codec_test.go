package macro

import "testing"

func TestRoundtrip(t *testing.T) {
	payload := make([]byte, MaxPayload)
	for i := range payload {
		payload[i] = byte(i)
	}
	frame, err := Encode(payload, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Payload) != len(payload) {
		t.Fatalf("len %d != %d", len(res.Payload), len(payload))
	}
	for i := range payload {
		if res.Payload[i] != payload[i] {
			t.Fatalf("byte %d: %d != %d", i, res.Payload[i], payload[i])
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	payload := make([]byte, MaxPayload)
	for i := range payload {
		payload[i] = byte(i)
	}
	b.SetBytes(int64(MaxPayload))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Encode(payload, uint32(i), 100)
	}
}

func BenchmarkDecode(b *testing.B) {
	payload := make([]byte, MaxPayload)
	for i := range payload {
		payload[i] = byte(i)
	}
	frame, _ := Encode(payload, 1, 100)
	b.SetBytes(int64(MaxPayload))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(frame)
	}
}
