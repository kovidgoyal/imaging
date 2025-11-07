package main

import (
	"fmt"
	"os"

	"github.com/kovidgoyal/imaging"
)

var _ = fmt.Print

func main() {
	var err error
	defer func() {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}()
	if len(os.Args) == 1 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/demo input-file [output-file]")
		os.Exit(1)
	}
	img, err := imaging.OpenAll(os.Args[1])
	if err != nil {
		return
	}
	ext := ".png"
	if len(img.Frames) > 1 {
		ext = ".apng"
	}
	output_file := os.Args[1] + ext
	if len(os.Args) == 3 {
		output_file = os.Args[2]
	}
	out, err := os.OpenFile(output_file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return
	}
	err = img.EncodeAsPNG(out)
	if err == nil {
		fmt.Println("PNG saved to:", output_file)
	}
}
