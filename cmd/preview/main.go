package main

import (
	"crypto/rand"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"

	"github.com/zarazaex69/gr"
	"github.com/zarazaex69/gr/tile"
)

func grayFrameToImage(frame []byte, w, h int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, w, h))
	copy(img.Pix, frame)
	return img
}

func savePNG(img image.Image, path string) {
	f, _ := os.Create(path)
	defer f.Close()
	png.Encode(f, img)
	fmt.Println("saved:", path)
}

func main() {
	payload := make([]byte, 1500)
	rand.Read(payload)

	// --- QR 1500 bytes ---
	qrFrame, _ := gr.Encode(payload)
	savePNG(grayFrameToImage(qrFrame, 1080, 1080), "preview_qr_1500b.png")

	// --- QR small (64 bytes) ---
	small := make([]byte, 64)
	rand.Read(small)
	qrSmall, _ := gr.Encode(small)
	savePNG(grayFrameToImage(qrSmall, 1080, 1080), "preview_qr_64b.png")

	// --- Tile configs ---
	configs := []struct {
		name string
		cfg  tile.Config
	}{
		{"tile_1px_noECC", tile.Config{Module: 1, RSPercent: 0}},
		{"tile_2px_noECC", tile.Config{Module: 2, RSPercent: 0}},
		{"tile_4px_noECC", tile.Config{Module: 4, RSPercent: 0}},
		{"tile_4px_RS20", tile.Config{Module: 4, RSPercent: 20}},
		{"tile_8px_RS50", tile.Config{Module: 8, RSPercent: 50}},
		{"tile_16px_RS50", tile.Config{Module: 16, RSPercent: 50}},
	}

	var gifFrames []*image.Paletted
	var delays []int

	pal := make(color.Palette, 256)
	for i := range pal {
		pal[i] = color.Gray{Y: uint8(i)}
	}

	for _, cfg := range configs {
		c, err := tile.New(cfg.cfg)
		if err != nil {
			fmt.Printf("skip %s: %v\n", cfg.name, err)
			continue
		}
		p := make([]byte, min(c.MaxPayload(), 8000))
		rand.Read(p)
		frame, _ := c.Encode(p, 0, 1)
		img := grayFrameToImage(frame, 1080, 1080)
		savePNG(img, "preview_"+cfg.name+".png")
		fmt.Printf("  %s: %s\n", cfg.name, c.Info())

		// add to GIF (scale down to 270x270 for file size)
		scaled := scaleDown(img, 4)
		palImg := toPaletted(scaled, pal)
		gifFrames = append(gifFrames, palImg)
		delays = append(delays, 80) // 0.8s per frame
	}

	// --- GIF animation ---
	f, _ := os.Create("preview_all.gif")
	defer f.Close()
	gif.EncodeAll(f, &gif.GIF{
		Image: gifFrames,
		Delay: delays,
	})
	fmt.Println("saved: preview_all.gif")
}

func scaleDown(src *image.Gray, factor int) *image.Gray {
	w := src.Bounds().Dx() / factor
	h := src.Bounds().Dy() / factor
	dst := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.SetGray(x, y, src.GrayAt(x*factor, y*factor))
		}
	}
	return dst
}

func toPaletted(src *image.Gray, pal color.Palette) *image.Paletted {
	b := src.Bounds()
	dst := image.NewPaletted(b, pal)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, y, src.GrayAt(x, y))
		}
	}
	return dst
}
