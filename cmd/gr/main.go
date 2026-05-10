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
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: gr [flags] <text>\n       echo <text> | gr [flags]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

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

	// Double-space mode width is len(bmp)*2
	if *small || len(bmp[0])*2 > termW {
		printQRCompact(bmp)
	} else {
		printQRLarge(bmp)
	}
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
	fmt.Println()
	for y := 0; y < len(bmp); y += 2 {
		fmt.Print(whiteBG, blackFG)
		for x := 0; x < len(bmp[y]); x++ {
			top := bmp[y][x]
			bottom := false
			if y+1 < len(bmp) {
				bottom = bmp[y+1][x]
			}

			if top && bottom {
				fmt.Print("█") // Both black
			} else if top {
				fmt.Print("▀") // Top black
			} else if bottom {
				fmt.Print("▄") // Bottom black
			} else {
				fmt.Print(" ") // Both white
			}
		}
		fmt.Println(reset)
	}
	fmt.Println()
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
