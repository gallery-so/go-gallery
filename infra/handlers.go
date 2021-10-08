package infra

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"

	svg "github.com/ajstarks/svgo"
	"github.com/mikeydub/go-gallery/util"
)

// autoglyphs is a unique metadata handler for the autoglyphs contract
/**
 * The drawing instructions for the nine different symbols are as follows:
 *
 *   .  Draw nothing in the cell.
 *   O  Draw a circle bounded by the cell.
 *   +  Draw centered lines vertically and horizontally the length of the cell.
 *   X  Draw diagonal lines connecting opposite corners of the cell.
 *   |  Draw a centered vertical line the length of the cell.
 *   -  Draw a centered horizontal line the length of the cell.
 *   \  Draw a line connecting the top left corner of the cell to the bottom right corner.
 *   /  Draw a line connecting the bottom left corner of teh cell to the top right corner.
 *   #  Fill in the cell completely.
 *
 */
func autoglyphs(i *Indexer, tokenURI uri, addr address, tid tokenID) (metadata, error) {
	width := 80
	height := 80
	buf := &bytes.Buffer{}
	canvas := svg.New(buf)
	canvas.Start(width, height)
	canvas.Square(0, 0, width, canvas.RGB(255, 255, 255))
	for i, c := range tokenURI {
		y := int(math.Floor(float64(i)/float64(64))) + 8
		x := (i % 64) + 8
		switch c {
		case 'O':
			canvas.Circle(x, y, 1, canvas.RGB(0, 0, 0))
		case '+':
			canvas.Line(x, y, x+1, y, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
			canvas.Line(x, y, x, (y + 1), `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case 'X':
			canvas.Line(x, y, x+1, y+1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
			canvas.Line(x, y, x+1, y-1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '|':
			canvas.Line(x, y, x, y+1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '-':
			canvas.Line(x, y, x+1, y, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '\\':
			canvas.Line(x, y, x+1, y+1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '/':
			canvas.Line(x, y, x+1, y-1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '#':
			canvas.Rect(x, y, 1, 1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		}
	}
	canvas.End()
	it, err := util.HexToBigInt(string(tid))
	if err != nil {
		return nil, err
	}
	return metadata{
		"name":        fmt.Sprintf("Autoglyphs #%d", it.Uint64()),
		"description": "Autoglyphs are the first “on-chain” generative art on the Ethereum blockchain. A completely self-contained mechanism for the creation and ownership of an artwork.",
		"image":       fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes())),
	}, nil
}
