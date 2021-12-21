package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"io"
	"os"
	"os/exec"

	"github.com/nfnt/resize"
)

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {

	fi, err := os.Open("card.mp4")
	checkError(err)
	defer fi.Close()

	jp, err := os.Create("card.jpg")
	checkError(err)
	defer jp.Close()

	buf := &bytes.Buffer{}

	inBuf := &bytes.Buffer{}

	out := io.MultiWriter(jp, buf)

	c := exec.Command("ffmpeg", "-i", "pipe:0", "-ss", "00:00:01.000", "-vframes", "1", "-f", "singlejpeg", "pipe:1")
	c.Stdin = inBuf
	c.Stdout = out
	c.Stderr = os.Stderr

	fmt.Println(buf.String())

	jpg, err := jpeg.Decode(buf)
	checkError(err)
	jpg = resize.Thumbnail(1024, 1024, jpg, resize.NearestNeighbor)
	buf = &bytes.Buffer{}
	err = jpeg.Encode(buf, jpg, nil)
	checkError(err)

	newJPG, err := os.Create("card.new.jpg")
	checkError(err)
	defer newJPG.Close()

	_, err = newJPG.Write(buf.Bytes())
	checkError(err)

}
