package main

import (
	"bytes"
	"io"

	"github.com/mikeydub/go-gallery/util"
)

func main() {

	bs := []byte("hello world")
	buf := bytes.NewBuffer(bs)

	printMatches(util.NewFileHeaderReader(buf, 4096))
}

func printMatches(r io.Reader) {
	if _, ok := r.(io.WriterTo); ok {
		panic("ASDAsd")
	}
}
