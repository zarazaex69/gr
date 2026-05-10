package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zarazaex69/gr/qr"
)

const (
	blockFull  = "██"
	blockEmpty = "  "
)

func main() {
	ecc := flag.String("ecc", "L", "error correction level: L, M, Q, H")
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

	printQR(bmp)
}

func printQR(bmp [][]bool) {
	border := strings.Repeat(blockEmpty, len(bmp[0])+2)
	fmt.Println(border)
	for _, row := range bmp {
		fmt.Print(blockEmpty)
		for _, dark := range row {
			if dark {
				fmt.Print(blockFull)
			} else {
				fmt.Print(blockEmpty)
			}
		}
		fmt.Println(blockEmpty)
	}
	fmt.Println(border)
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
