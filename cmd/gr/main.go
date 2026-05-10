package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zarazaex69/gr/qr"
	"golang.org/x/term"
)

const (
	whiteBG = "\033[107m"
	blackBG = "\033[40m"
	blackFG = "\033[30m"
	reset   = "\033[0m"
)

func main() {
	ecc := flag.String("ecc", "L", "error correction level: L, M, Q, H")
	small := flag.Bool("s", false, "force small (compact) rendering")
	octant := flag.Bool("o", false, "force octant rendering (densest, requires Unicode 16 font)")
	diag := flag.Bool("d", false, "print octant glyph diagnostic grid and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: gr [flags] <text>\n       echo <text> | gr [flags]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *diag {
		printOctantDiagnostic()
		return
	}

	var payload []byte
	if flag.NArg() > 0 {
		payload = []byte(strings.Join(flag.Args(), " "))
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatalf("read stdin: %v", err)
		}
		payload = data
	}
	payload = trimNewline(payload)
	if len(payload) == 0 {
		fatalf("empty input")
	}

	level := parseECC(*ecc)
	codec, err := qr.New(qr.Config{ECC: level})
	if err != nil {
		fatalf("init codec: %v", err)
	}

	bmp, err := codec.EncodeBitmap(payload)
	if err != nil {
		fatalf("encode: %v", err)
	}

	termW, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if termW <= 0 {
		termW = 80
	}

	switch {
	case *octant:
		printQROctant(bmp)
	case *small:
		printQRSextant(bmp)
	case len(bmp[0])*2 > termW:
		printQRCompact(bmp)
	default:
		printQRLarge(bmp)
	}
}

// printQROctant renders with Unicode 16 octants (2×4 modules per cell).
// Densest possible terminal rendering; requires a font with full
// U+1CD00..U+1CDE5 coverage or columns will misalign.
//
// The QR's 4-module quiet zone is trimmed asymmetrically so the live data
// lands on octant boundaries. Without this, the first or last data row
// gets squeezed into 1/4 of an octant cell and reads as a thin stray line.
// Trimming quiet zone is safe — QR scanners only require ≥1 module of
// margin, and we keep at least 2 on every side.
func printQROctant(bmp [][]bool) {
	h, w := len(bmp), len(bmp[0])
	yTrim := h % 4
	xTrim := w % 2
	yLo, yHi := yTrim/2, h-(yTrim-yTrim/2)
	xLo, xHi := xTrim/2, w-(xTrim-xTrim/2)
	for y := yLo; y < yHi; y += 4 {
		fmt.Print(whiteBG, blackFG)
		for x := xLo; x < xHi; x += 2 {
			var mask uint8
			for dy := range 4 {
				yy := y + dy
				if bmp[yy][x] {
					mask |= 1 << (dy + 4)
				}
				if x+1 < w && bmp[yy][x+1] {
					mask |= 1 << dy
				}
			}
			fmt.Print(octantGlyphs[mask])
		}
		fmt.Println(reset)
	}
}

// printOctantDiagnostic prints all 256 octant glyphs in a 16×16 grid with
// `|` separators. If your terminal renders any glyph as wide (or as tofu),
// the right column will misalign starting at that index — note the hex
// indices of the offending glyphs.
func printOctantDiagnostic() {
	fmt.Println("    " + strings.Repeat(" 0|", 16))
	for hi := range 16 {
		fmt.Printf("%X0: |", hi)
		for lo := range 16 {
			fmt.Print(octantGlyphs[hi<<4|lo], "|")
		}
		fmt.Println()
	}
	fmt.Println("\nIf any column boundary doesn't align, that glyph is broken in your font.")
}

func printQRLarge(bmp [][]bool) {
	fmt.Println()
	for _, row := range bmp {
		fmt.Print(whiteBG)
		for _, dark := range row {
			if dark {
				fmt.Print(blackBG, "  ")
			} else {
				fmt.Print(whiteBG, "  ")
			}
		}
		fmt.Println(reset)
	}
	fmt.Println()
}

func printQRCompact(bmp [][]bool) {
	for y := 0; y < len(bmp); y += 2 {
		fmt.Print(whiteBG, blackFG)
		for x := 0; x < len(bmp[y]); x++ {
			top := bmp[y][x]
			bottom := false
			if y+1 < len(bmp) {
				bottom = bmp[y+1][x]
			}

			if top && bottom {
				fmt.Print("█")
			} else if top {
				fmt.Print("▀")
			} else if bottom {
				fmt.Print("▄")
			} else {
				fmt.Print(" ")
			}
		}
		fmt.Println(reset)
	}
}

// sextantGlyphs maps a 6-bit pattern to the Unicode 13 sextant character.
// Bit layout: 0=top-left, 1=top-right, 2=middle-left, 3=middle-right,
// 4=bottom-left, 5=bottom-right. Patterns 0/21/42/63 reuse legacy block
// elements; the rest are encoded sequentially in U+1FB00..U+1FB3B.
var sextantGlyphs = func() [64]string {
	var t [64]string
	t[0] = " "
	t[21] = "▌"
	t[42] = "▐"
	t[63] = "█"
	skipped := map[int]bool{0: true, 21: true, 42: true, 63: true}
	cp := rune(0x1FB00)
	for n := 1; n < 63; n++ {
		if skipped[n] {
			continue
		}
		t[n] = string(cp)
		cp++
	}
	return t
}()

// printQRSextant renders the bitmap using Unicode 13 sextant glyphs (2×3
// modules per cell). Sextant fonts are far more widely supported than
// octants, so columns stay aligned even on older terminal fonts.
func printQRSextant(bmp [][]bool) {
	h, w := len(bmp), len(bmp[0])
	for y := 0; y < h; y += 3 {
		fmt.Print(whiteBG, blackFG)
		for x := 0; x < w; x += 2 {
			var mask uint8
			for dy := range 3 {
				yy := y + dy
				if yy >= h {
					break
				}
				if bmp[yy][x] {
					mask |= 1 << (dy * 2)
				}
				if x+1 < w && bmp[yy][x+1] {
					mask |= 1 << (dy*2 + 1)
				}
			}
			fmt.Print(sextantGlyphs[mask])
		}
		fmt.Println(reset)
	}
}

func parseECC(s string) qr.ECCLevel {
	switch strings.ToUpper(s) {
	case "M":
		return qr.ECCMedium
	case "Q":
		return qr.ECCQuartile
	case "H":
		return qr.ECCHigh
	default:
		return qr.ECCLow
	}
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "gr: "+format+"\n", a...)
	os.Exit(1)
}
