package vp8test

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zarazaex69/gr/qr"
	"github.com/zarazaex69/gr/tile"
)

const (
	W = 1080
	H = 1080
	N = 10 // frames per test
)

var bitrates = []int{4000, 10000, 20000, 40000}

func TestQR_VP8(t *testing.T) {
	levels := []qr.ECCLevel{qr.ECCLow, qr.ECCMedium, qr.ECCQuartile, qr.ECCHigh}
	names := []string{"Low", "Medium", "Quartile", "High"}

	for i, ecc := range levels {
		codec, _ := qr.New(qr.Config{ECC: ecc})
		maxP := codec.MaxPayload()

		for _, br := range bitrates {
			name := fmt.Sprintf("ECC_%s/%dkbps", names[i], br)
			t.Run(name, func(t *testing.T) {
				tmp := t.TempDir()

				// Generate frames
				payloads := make([][]byte, N)
				raw := make([]byte, 0, W*H*N)
				for f := range N {
					p := make([]byte, maxP)
					rand.Read(p)
					payloads[f] = p
					frame, err := codec.Encode(p)
					if err != nil {
						t.Fatalf("encode frame %d: %v", f, err)
					}
					raw = append(raw, frame...)
				}

				// VP8 roundtrip
				decoded := vp8Roundtrip(t, tmp, raw, br)
				if decoded == nil {
					t.Fatal("vp8 roundtrip failed")
				}

				// Decode and compare
				ok, fail := 0, 0
				for f := range N {
					if (f+1)*W*H > len(decoded) {
						break
					}
					frame := decoded[f*W*H : (f+1)*W*H]
					got, err := codec.Decode(frame)
					if err != nil {
						fail++
						continue
					}
					if string(got) == string(payloads[f]) {
						ok++
					} else {
						fail++
					}
				}
				rate := float64(ok) / float64(ok+fail) * 100
				mbps := float64(maxP) * 60 / 1024 / 1024
				t.Logf("success=%.0f%% (%d/%d) payload=%dB throughput=%.2fMB/s@60fps",
					rate, ok, ok+fail, maxP, mbps)
				if rate < 50 {
					t.Errorf("too many failures: %.0f%%", rate)
				}
			})
		}
	}
}

func TestTile_VP8(t *testing.T) {
	modules := []int{2, 3, 4, 5}
	rsPercents := []int{0, 20, 50}

	for _, mod := range modules {
		for _, rs := range rsPercents {
			codec, err := tile.New(tile.Config{Module: mod, RSPercent: rs})
			if err != nil {
				continue
			}
			maxP := codec.MaxPayload()

			for _, br := range bitrates {
				name := fmt.Sprintf("mod%d_rs%d/%dkbps", mod, rs, br)
				t.Run(name, func(t *testing.T) {
					tmp := t.TempDir()

					payloads := make([][]byte, N)
					raw := make([]byte, 0, W*H*N)
					for f := range N {
						p := make([]byte, maxP)
						rand.Read(p)
						payloads[f] = p
						frame, err := codec.Encode(p, uint32(f), uint32(N))
						if err != nil {
							t.Fatalf("encode frame %d: %v", f, err)
						}
						raw = append(raw, frame...)
					}

					decoded := vp8Roundtrip(t, tmp, raw, br)
					if decoded == nil {
						t.Fatal("vp8 roundtrip failed")
					}

					ok, fail := 0, 0
					for f := range N {
						if (f+1)*W*H > len(decoded) {
							break
						}
						frame := decoded[f*W*H : (f+1)*W*H]
						res, err := codec.Decode(frame)
						if err != nil {
							fail++
							continue
						}
						if string(res.Payload) == string(payloads[f]) {
							ok++
						} else {
							fail++
						}
					}
					rate := float64(ok) / float64(ok+fail) * 100
					mbps := float64(maxP) * 60 / 1024 / 1024
					t.Logf("success=%.0f%% (%d/%d) payload=%dB throughput=%.2fMB/s@60fps",
						rate, ok, ok+fail, maxP, mbps)
				})
			}
		}
	}
}

func vp8Roundtrip(t *testing.T, tmp string, raw []byte, kbps int) []byte {
	t.Helper()
	inPath := filepath.Join(tmp, "in.gray")
	webm := filepath.Join(tmp, "out.webm")
	outPath := filepath.Join(tmp, "out.gray")

	os.WriteFile(inPath, raw, 0644)

	// encode
	if err := ffmpeg("-f", "rawvideo", "-pix_fmt", "gray", "-s", "1080x1080", "-r", "60",
		"-i", inPath,
		"-c:v", "libvpx", "-b:v", fmt.Sprintf("%dk", kbps),
		"-quality", "realtime", "-cpu-used", "8",
		"-g", "9999", "-y", webm); err != nil {
		t.Fatalf("ffmpeg encode: %v", err)
	}

	// decode
	if err := ffmpeg("-i", webm, "-pix_fmt", "gray", "-f", "rawvideo", "-y", outPath); err != nil {
		t.Fatalf("ffmpeg decode: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		return nil
	}
	return data
}

func ffmpeg(args ...string) error {
	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Run()
}
