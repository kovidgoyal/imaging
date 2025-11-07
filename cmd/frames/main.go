package main

import (
	"encoding/json"
	"fmt"
	"image/png"
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
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/frames input-file [output-prefix]")
		os.Exit(1)
	}
	img, err := imaging.OpenAll(os.Args[1])
	if err != nil {
		return
	}
	output_prefix := os.Args[1]
	if len(os.Args) == 3 {
		output_prefix = os.Args[2]
	}
	b, err := json.MarshalIndent(img, "", "  ")
	if err != nil {
		return
	}
	output_file := fmt.Sprintf("%s-metadata.json", output_prefix)
	if err = os.WriteFile(output_file, b, 0o666); err != nil {
		return
	}
	cimg := img.Clone()
	cimg.Coalesce()
	for i, f := range img.Frames {
		output_file := fmt.Sprintf("%s-%05d.png", output_prefix, f.Number)
		out, err := os.OpenFile(output_file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
		if err != nil {
			return
		}
		func() {
			defer out.Close()
			if err = png.Encode(out, f.Image); err != nil {
				return
			}
		}()
		f = cimg.Frames[i]
		output_file = fmt.Sprintf("%s-coalesced-%05d.png", output_prefix, f.Number)
		out, err = os.OpenFile(output_file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
		if err != nil {
			return
		}
		func() {
			defer out.Close()
			if err = png.Encode(out, f.Image); err != nil {
				return
			}
		}()
	}
	if err == nil {
		fmt.Printf("Frames decoded to %s-[coalesced]*.[png|json]\n", output_prefix)
	}
}
