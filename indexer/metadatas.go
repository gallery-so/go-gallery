package indexer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"net/http"
	"sort"
	"strings"

	svg "github.com/ajstarks/svgo"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/persist"
)

var white = color.RGBA{255, 255, 255, 255}
var black = color.RGBA{2, 4, 8, 0}

const cryptoPunksImageContractAddress = "0x16f5a35647d6f03d5d3da7b35409d65ba03af3b2"

type uniqueMetadataHandler func(context.Context, persist.TokenURI, persist.EthereumAddress, persist.TokenID, *ethclient.Client, *shell.Shell, *goar.Client) (persist.TokenURI, persist.TokenMetadata, error)

type uniqueMetadatas map[persist.EthereumAddress]uniqueMetadataHandler

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
func autoglyphs(ctx context.Context, turi persist.TokenURI, addr persist.EthereumAddress, tid persist.TokenID, ethCl *ethclient.Client, ipfs *shell.Shell, arweave *goar.Client) (persist.TokenURI, persist.TokenMetadata, error) {

	start := strings.Index(turi.String(), ",") + 1
	if start == -1 {
		return turi, nil, fmt.Errorf("invalid colorglyphs tokenURI")
	}
	glyph := turi.String()[start:]

	glyph = strings.ReplaceAll(glyph, "\n", "")
	glyph = strings.ReplaceAll(glyph, "%0A", "")

	width := 240
	height := 240
	buf := &bytes.Buffer{}
	canvas := svg.New(buf)
	canvas.Start(width, height)
	canvas.Square(0, 0, width, canvas.RGB(255, 255, 255))
	for i, c := range glyph {

		y := int(math.Floor(float64(i)/float64(64))*3) + 21
		x := ((i % 64) * 3) + 21
		switch c {
		case 'O':
			canvas.Circle(x+1, y+1, 1, canvas.RGB(0, 0, 0))
		case '+':
			canvas.Line(x-1, y, x+1, y, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
			canvas.Line(x, y-1, x, (y + 1), `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case 'X':
			canvas.Line(x-1, y-1, x+1, y+1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
			canvas.Line(x-1, y+1, x+1, y-1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '|':
			canvas.Line(x, y-1, x, y+1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '-':
			canvas.Line(x-1, y, x+1, y, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '\\':
			canvas.Line(x-1, y+1, x+1, y-1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '/':
			canvas.Line(x-1, y-1, x+1, y+1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '#':
			canvas.Rect(x, y, 1, 1, `stroke="black"`, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		}
	}
	canvas.End()

	// cut off everything before the svg tag in the buffer
	svgStart := bytes.Index(buf.Bytes(), []byte("<svg"))
	if svgStart == -1 {
		return turi, nil, fmt.Errorf("no svg tag found in response")
	}
	return turi, persist.TokenMetadata{
		"name":        fmt.Sprintf("Autoglyph #%s", tid.Base10String()),
		"description": "Autoglyphs are the first “on-chain” generative art on the Ethereum blockchain. A completely self-contained mechanism for the creation and ownership of an artwork.",
		"image":       fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes()[svgStart:])),
	}, nil
}

/**
*  The drawing instructions for the nine different symbols are as follows:
*
*    .  Draw nothing in the cell.
*    O  Draw a circle bounded by the cell.
*    +  Draw centered lines vertically and horizontally the length of the cell.
*    X  Draw diagonal lines connecting opposite corners of the cell.
*    |  Draw a centered vertical line the length of the cell.
*    -  Draw a centered horizontal line the length of the cell.
*    \  Draw a line connecting the top left corner of the cell to the bottom right corner.
*    /  Draw a line connecting the bottom left corner of teh cell to the top right corner.
*    #  Fill in the cell completely.
*
* The 'tokenURI' function of colorglyphs adds two pieces of information to the response provided by autoglyphs:
*  1) The color scheme to apply to the Colorglyph.
*  2) The address of the Colorglyph's creator, from which colors are derived.
*
* The address of the Colorglyph's creator is split up into 35 6 digit chunks.
* For example, the first three chunks of 0xb189f76323678E094D4996d182A792E52369c005 are: b189f7, 189f76, and 89f763.
* The last chunk is 69c005.
* Each Colorglyph is an Autoglyph with a color scheme applied to it.
* Each Colorglyph takes the same shape as the Autoglyph of the corresponding ID.
* If the Colorglyph's ID is higher than 512, it takes the shape of the Autoglyph with its Colorglyphs ID - 512.
* Each black element in the Autoglyph is assigned a new color.
* The background color of the Autoglyph is changed to either black or one of the address colors.
* Visual implementations of Colorglyphs may exercise a substantial degree of flexibility.
* Color schemes that use multiple colors may apply any permitted color to any element,
* but no color should appear more than 16 times as often as the color with the lowest number of incidences.
* In the event that a color meets two conditions (reddest and orangest, for example),
* it may be used for both purposes.  The previous guideline establishing a threshold ratio of occurances
* treats the reddest color and the orangest color as two different colors, even if they have the same actual value.

* lightest address color = chunk with the lowest value resulting from red value + green value + blue value
* second lightest address color = second lightest chunk in relevant address
* third lightest address color = third lightest chunk in relevant address
* fourth lightest address color = fourth lightest chunk in relevant address
* fifth lightest address color = fifth lightest chunk in relevant address
* reddest address color = chunk with the lowest value resulting from red value - green value - blue value
* orangest address color = chunk with the highest value resulting from red value - blue value
* yellowest address color = chunk with higest value resulting from red value + green value - blue value
* greenest address color = chunk with higest value resulting from green value - red value - blue value
* bluest address color = chunk with higest value resulting from blue value - green value - red value
* darkest address color = darkest chunk in relevant address
* white = ffffff
* black = 020408

* scheme 1 = lightest address color, third lightest address color, and fifth lightest address color on black
* scheme 2 = lighest 4 address colors on black
* scheme 3 = reddest address color, orangest address color, and yellowest address color on black
* scheme 4 = reddest address color, yellowest address color, greenest address color, and white on black
* scheme 5 = lightest address color, reddest address color, yellowest address color, greenest address color, and bluest address color on black
* scheme 6 = reddest address color and white on black
* scheme 7 = greenest address color on black
* scheme 8 = lightest address color on darkest address color
* scheme 9 = greenest address color on reddest address color
* scheme 10 = reddest address color, yellowest address color, bluest address color, lightest address color, and black on white
 */
func colorglyphs(ctx context.Context, turi persist.TokenURI, addr persist.EthereumAddress, tid persist.TokenID, ethCl *ethclient.Client, ipfs *shell.Shell, arweave *goar.Client) (persist.TokenURI, persist.TokenMetadata, error) {
	spl := strings.Split(string(turi), " ")
	if len(spl) != 3 {
		panic("invalid colorglyphs tokenURI")
	}

	// find the index of the first character after data:text/plain;charset=utf-8, in spl[0]
	start := strings.Index(spl[0], ",") + 1
	if start == -1 {
		return turi, nil, fmt.Errorf("invalid colorglyphs tokenURI")
	}
	spl[0] = strings.ReplaceAll(spl[0], "\n", "")
	spl[0] = strings.ReplaceAll(spl[0], "%0A", "")
	spl[0] = spl[0][start:]

	allColors := make([]color.RGBA, 35)
	for i := 0; i < 35; i++ {
		col, err := parseHexColor(spl[2][i : i+6])
		if err != nil {
			panic(err)
		}
		allColors[i] = col
	}

	// sort colors by value
	sort.Slice(allColors, func(i, j int) bool {
		return allColors[i].R+allColors[i].G+allColors[i].B < allColors[j].R+allColors[j].G+allColors[j].B
	})
	lightestColor := allColors[0]
	secondLightestColor := allColors[1]
	thirdLightestColor := allColors[2]
	fourthLightestColor := allColors[3]
	fifthLightestColor := allColors[4]
	darkestColor := allColors[34]

	sort.Slice(allColors, func(i, j int) bool {
		return allColors[i].R-allColors[i].G-allColors[i].B < allColors[j].R-allColors[j].G-allColors[j].B
	})
	reddestColor := allColors[0]
	sort.Slice(allColors, func(i, j int) bool {
		return allColors[i].R-allColors[i].B > allColors[j].R-allColors[j].B
	})
	orangestColor := allColors[0]
	sort.Slice(allColors, func(i, j int) bool {
		return allColors[i].R+allColors[i].G-allColors[i].B > allColors[j].R+allColors[j].G-allColors[j].B
	})
	yellowestColor := allColors[0]
	sort.Slice(allColors, func(i, j int) bool {
		return allColors[i].G-allColors[i].R-allColors[i].B > allColors[j].G-allColors[j].R-allColors[j].B
	})
	greenestColor := allColors[0]
	sort.Slice(allColors, func(i, j int) bool {
		return allColors[i].B-allColors[i].G-allColors[i].R > allColors[j].B-allColors[j].G-allColors[j].R
	})
	bluestColor := allColors[0]

	var schemeColors []color.RGBA
	var backgroundColor color.RGBA
	switch spl[1] {
	case "1":
		schemeColors = []color.RGBA{lightestColor, thirdLightestColor, fifthLightestColor}
		backgroundColor = black
	case "2":
		schemeColors = []color.RGBA{lightestColor, secondLightestColor, thirdLightestColor, fourthLightestColor}
		backgroundColor = black
	case "3":
		schemeColors = []color.RGBA{reddestColor, orangestColor, yellowestColor}
		backgroundColor = black
	case "4":
		schemeColors = []color.RGBA{reddestColor, yellowestColor, greenestColor, white}
		backgroundColor = black
	case "5":
		schemeColors = []color.RGBA{lightestColor, reddestColor, yellowestColor, greenestColor, bluestColor}
		backgroundColor = black
	case "6":
		schemeColors = []color.RGBA{reddestColor, white}
		backgroundColor = black
	case "7":
		schemeColors = []color.RGBA{greenestColor}
		backgroundColor = black
	case "8":
		schemeColors = []color.RGBA{lightestColor}
		backgroundColor = darkestColor
	case "9":
		schemeColors = []color.RGBA{greenestColor}
		backgroundColor = reddestColor
	case "10":
		schemeColors = []color.RGBA{reddestColor, yellowestColor, bluestColor, lightestColor, black}
		backgroundColor = white
	}

	width := 240
	height := 240
	buf := &bytes.Buffer{}
	canvas := svg.New(buf)
	canvas.Start(width, height)
	canvas.Square(0, 0, width, canvas.RGB(int(backgroundColor.R), int(backgroundColor.G), int(backgroundColor.B)))
	for i, c := range spl[0] {
		y := int(math.Floor(float64(i)/float64(64))*3) + 21
		x := ((i % 64) * 3) + 21
		col := schemeColors[int(math.Floor(float64(int(c))/float64(len(schemeColors))))%len(schemeColors)]
		stroke := fmt.Sprintf(`stroke="rgb(%d,%d,%d)"`, col.R, col.G, col.B)
		switch c {
		case 'O':
			canvas.Circle(x+1, y+1, 1, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`, `fill="none"`)
		case '+':
			canvas.Line(x-1, y, x+1, y, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
			canvas.Line(x, y-1, x, y+1, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case 'X':
			canvas.Line(x-1, y-1, x+1, y+1, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
			canvas.Line(x+1, y-1, x-1, y+1, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '|':
			canvas.Line(x, y-1, x, y+1, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '-':
			canvas.Line(x-1, y, x+1, y, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '\\':
			canvas.Line(x-1, y+1, x+1, y-1, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '/':
			canvas.Line(x-1, y-1, x+1, y+1, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		case '#':
			canvas.Rect(x, y, 1, 1, stroke, `stroke-width="0.2"`, `stroke-linecap="butt"`)
		}
	}
	canvas.End()

	// cut off everything before the svg tag in the buffer
	svgStart := bytes.Index(buf.Bytes(), []byte("<svg"))
	if svgStart == -1 {
		return turi, nil, fmt.Errorf("no svg tag found in response")
	}
	return turi, persist.TokenMetadata{
		"name":        fmt.Sprintf("Colorglyph #%s", tid.Base10String()),
		"description": fmt.Sprintf("A Colorglyph with color scheme %s. Created by %s.", spl[1], spl[2]),
		"image":       fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes()[svgStart:])),
	}, nil
}

func parseHexColor(s string) (c color.RGBA, err error) {

	asBytes, err := hex.DecodeString(s)
	if err != nil {
		return
	}

	fmt.Printf("Hex: %s Bytes: %+v\n", s, asBytes)

	c.R = asBytes[0]
	c.G = asBytes[1]
	c.B = asBytes[2]

	return
}

const ensGraph = "https://api.thegraph.com/subgraphs/name/ensdomains/ens"

type ensDomain struct {
	LabelName string `json:"labelName"`
}
type ensDomains struct {
	Domains []ensDomain `json:"domains"`
}

type graphResponse struct {
	Data ensDomains `json:"data"`
}

func ens(ctx context.Context, turi persist.TokenURI, addr persist.EthereumAddress, tid persist.TokenID, ethCl *ethclient.Client, ipfs *shell.Shell, arweave *goar.Client) (persist.TokenURI, persist.TokenMetadata, error) {

	gql := fmt.Sprintf(`
	{
	  domains(first:1, where:{labelhash:"%s"}){
		labelName
	  }
	}`, tid)

	jsonData := map[string]interface{}{
		"query": gql,
	}

	marshaled, err := json.Marshal(jsonData)
	if err != nil {
		return turi, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", ensGraph, bytes.NewBuffer(marshaled))
	if err != nil {
		return turi, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return turi, nil, err
	}
	defer resp.Body.Close()

	var gr graphResponse
	err = json.NewDecoder(resp.Body).Decode(&gr)
	if err != nil {
		return turi, nil, err
	}

	if len(gr.Data.Domains) == 0 {
		return turi, nil, fmt.Errorf("no ENS domain found for %s", tid)
	}
	if len(gr.Data.Domains) > 1 {
		return turi, nil, fmt.Errorf("multiple ENS domains found for %s", tid)
	}

	domain := gr.Data.Domains[0]

	width := 240
	height := 240
	buf := &bytes.Buffer{}
	canvas := svg.New(buf)
	canvas.Start(width, height)
	canvas.Square(0, 0, width, canvas.RGB(255, 255, 255))
	result := domain.LabelName + ".eth"

	canvas.Text(width/2, height/2, result, `font-size="16px"`, `text-anchor="middle"`, `alignment-baseline="middle"`)

	canvas.End()

	// cut off everything before the svg tag in the buffer
	svgStart := bytes.Index(buf.Bytes(), []byte("<svg"))
	if svgStart == -1 {
		return turi, nil, fmt.Errorf("no svg tag found in response")
	}

	return persist.TokenURI(result), persist.TokenMetadata{
		"name":        fmt.Sprintf("ENS: %s", result),
		"description": "ENS names are used to resolve domain names to Ethereum addresses.",
		"image":       fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes()[svgStart:])),
	}, nil

}

func cryptopunks(ctx context.Context, turi persist.TokenURI, addr persist.EthereumAddress, tid persist.TokenID, ethCl *ethclient.Client, ipfs *shell.Shell, arweave *goar.Client) (persist.TokenURI, persist.TokenMetadata, error) {
	dataContract, err := contracts.NewCryptopunksDataCaller(common.HexToAddress(cryptoPunksImageContractAddress), ethCl)
	if err != nil {
		return turi, nil, err
	}

	punkSVG, err := dataContract.PunkImageSvg(&bind.CallOpts{Context: ctx}, uint16(tid.ToInt()))
	if err != nil {
		return turi, nil, err
	}

	removedPrefix := strings.TrimPrefix(punkSVG, "data:image/svg+xml;utf8,")
	asBase64 := base64.RawStdEncoding.EncodeToString([]byte(removedPrefix))
	withBase64Prefix := fmt.Sprintf("data:image/svg+xml;base64,%s", asBase64)
	return persist.TokenURI(withBase64Prefix), persist.TokenMetadata{
		"name":        fmt.Sprintf("Cryptopunks: %s", tid.Base10String()),
		"description": "CryptoPunks launched as a fixed set of 10,000 items in mid-2017 and became one of the inspirations for the ERC-721 standard. They have been featured in places like The New York Times, Christie’s of London, Art|Basel Miami, and The PBS NewsHour.",
		"image":       withBase64Prefix,
	}, nil

}
