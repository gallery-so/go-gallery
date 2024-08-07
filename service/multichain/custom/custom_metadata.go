package custom

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"

	svg "github.com/ajstarks/svgo"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"
	colorful "github.com/lucasb-eyer/go-colorful"

	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/multichain/common"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
)

var (
	AutoglyphContract               = persist.ContractIdentifiers{ContractAddress: "0xd4e4078ca3495de5b1d4db434bebc5a986197782", Chain: persist.ChainETH}
	ColorglyphContract              = persist.ContractIdentifiers{ContractAddress: "0x60f3680350f65beb2752788cb48abfce84a4759e", Chain: persist.ChainETH}
	EnsContract                     = persist.ContractIdentifiers{ContractAddress: eth.EnsAddress, Chain: persist.ChainETH}
	CryptopunkContract              = persist.ContractIdentifiers{ContractAddress: "0xb47e3cd837ddf8e4c57f05d70ab865de6e193bbb", Chain: persist.ChainETH}
	ZoraContract                    = persist.ContractIdentifiers{ContractAddress: "0xabefbc9fd2f806065b4f3c237d4b59d9a97bcac7", Chain: persist.ChainETH}
	OpenseaSharedStorefrontContract = persist.ContractIdentifiers{ContractAddress: "0x495f947276749ce646f68ac8c248420045cb7b5e", Chain: persist.ChainETH}
)

type metadataHandler func(context.Context, persist.TokenIdentifiers, ...persist.TokenMetadata) (persist.TokenMetadata, error)

type CustomMetadataHandlers struct {
	AutoglyphHandler               metadataHandler
	ColorglyphHandler              metadataHandler
	EnsHandler                     metadataHandler
	CryptopunkHandler              metadataHandler
	ZoraHandler                    metadataHandler
	OpenseaSharedStorefrontHandler metadataHandler
}

func NewCustomMetadataHandlers(ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) *CustomMetadataHandlers {
	return &CustomMetadataHandlers{
		AutoglyphHandler:               newAutoglyphHandler(ethClient),
		ColorglyphHandler:              newColorglyphHandler(ethClient),
		EnsHandler:                     newEnsHandler(),
		CryptopunkHandler:              newCryptopunkHandler(ethClient),
		ZoraHandler:                    newZoraHandler(ethClient, ipfsClient, arweaveClient),
		OpenseaSharedStorefrontHandler: newOpenseaSharedStorefrontHandler(),
	}
}

func (c *CustomMetadataHandlers) HandlerFor(t persist.TokenIdentifiers) metadataHandler {
	cID := persist.ContractIdentifiers{ContractAddress: t.ContractAddress, Chain: t.Chain}
	switch cID {
	case AutoglyphContract:
		return c.AutoglyphHandler
	case ColorglyphContract:
		return c.ColorglyphHandler
	case EnsContract:
		return c.EnsHandler
	case CryptopunkContract:
		return c.CryptopunkHandler
	case ZoraContract:
		return c.ZoraHandler
	case OpenseaSharedStorefrontContract:
		return c.OpenseaSharedStorefrontHandler
	default:
		return nil
	}
}

func (c *CustomMetadataHandlers) AddToToken(ctx context.Context, chain persist.Chain, t common.ChainAgnosticToken) common.ChainAgnosticToken {
	tID := common.ChainAgnosticIdentifiers{ContractAddress: t.ContractAddress, TokenID: t.TokenID}
	m := c.Load(ctx, chain, tID, t.TokenMetadata)
	t.TokenMetadata = m
	return t
}

func (c *CustomMetadataHandlers) AddToPage(ctx context.Context, chain persist.Chain, recCh <-chan common.ChainAgnosticTokensAndContracts, errIn <-chan error) (<-chan common.ChainAgnosticTokensAndContracts, <-chan error) {
	outCh := make(chan common.ChainAgnosticTokensAndContracts, 2*10)
	errOut := make(chan error)
	go func() {
		defer close(outCh)
		defer close(errOut)
		for {
			select {
			case page, ok := <-recCh:
				if !ok {
					return
				}
				page.Tokens = c.LoadAll(ctx, chain, page.Tokens)
				outCh <- page
			case err, ok := <-errIn:
				if ok {
					errOut <- err
				}
			case <-ctx.Done():
				errOut <- ctx.Err()
				return
			}
		}
	}()
	logger.For(ctx).Info("finished applying custom metadata to page")
	return outCh, errOut
}

func (c *CustomMetadataHandlers) Load(ctx context.Context, chain persist.Chain, t common.ChainAgnosticIdentifiers, oldMetadata ...persist.TokenMetadata) persist.TokenMetadata {
	tID := persist.NewTokenIdentifiers(t.ContractAddress, t.TokenID, chain)
	h := c.HandlerFor(tID)
	if h == nil {
		return persist.TokenMetadata{}
	}
	m, err := h(ctx, tID, oldMetadata...)
	if err != nil {
		logger.For(ctx).Errorf("failed to get custom metadata for token(chain=%s, contract=%s, tokenID=%s): %s", chain, t.ContractAddress, t.TokenID, err)
		return nil
	}
	return m
}

func (c *CustomMetadataHandlers) LoadMetadataAll(ctx context.Context, chain persist.Chain, tokens []common.ChainAgnosticToken) []persist.TokenMetadata {
	tokens = c.LoadAll(ctx, chain, tokens)
	return util.MapWithoutError(tokens, func(t common.ChainAgnosticToken) persist.TokenMetadata { return t.TokenMetadata })
}

func (c *CustomMetadataHandlers) LoadAll(ctx context.Context, chain persist.Chain, tokens []common.ChainAgnosticToken) []common.ChainAgnosticToken {
	for i, t := range tokens {
		tokens[i] = c.AddToToken(ctx, chain, t)
	}
	return tokens
}

func newAutoglyphHandler(ethClient *ethclient.Client) metadataHandler {
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
	return func(ctx context.Context, t persist.TokenIdentifiers, _ ...persist.TokenMetadata) (persist.TokenMetadata, error) {
		tURI, err := rpc.RetryGetTokenURI(ctx, "", persist.EthereumAddress(t.ContractAddress), t.TokenID, ethClient)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		start := strings.Index(tURI.String(), ",") + 1
		if start == -1 {
			return persist.TokenMetadata{}, fmt.Errorf("invalid autoglyphs tokenURI")
		}

		glyph := tURI.String()[start:]
		glyph = strings.ReplaceAll(glyph, "\n", "")
		glyph = strings.ReplaceAll(glyph, "%0A", "")

		width := 368
		height := 368
		add := 3
		buf := &bytes.Buffer{}
		canvas := svg.New(buf)
		canvas.Start(width, height)
		canvas.Square(0, 0, width, canvas.RGB(255, 255, 255))
		for i, c := range glyph {

			y := int(math.Floor(float64(i)/float64(64))*5) + 28
			x := ((i % 64) * 5) + 28
			switch c {
			case 'O':
				canvas.Circle(x, y, add-1, `stroke="black"`, `stroke-width="0.6"`, `stroke-linecap="butt"`, `fill="none"`)
			case '+':
				canvas.Line(x-add, y, x+add, y, `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
				canvas.Line(x, y-add, x, (y + add), `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
			case 'X':
				canvas.Line(x-add, y-add, x+add, y+add, `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
				canvas.Line(x-add, y+add, x+add, y-add, `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
			case '|':
				canvas.Line(x, y-add, x, y+add, `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
			case '-':
				canvas.Line(x-add, y, x+add, y, `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
			case '\\':
				canvas.Line(x-add, y+add, x+add, y-add, `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
			case '/':
				canvas.Line(x-add, y-add, x+add, y+add, `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
			case '#':
				canvas.Rect(x-int(math.Ceil(float64(add)/2.0)), y-add, add+1, add+1, `stroke="black"`, `stroke-width="0.8"`, `stroke-linecap="square"`)
			}
		}
		canvas.End()

		// cut off everything before the svg tag in the buffer
		svgStart := bytes.Index(buf.Bytes(), []byte("<svg"))
		if svgStart == -1 {
			return persist.TokenMetadata{}, fmt.Errorf("no svg tag found in response")
		}

		return persist.TokenMetadata{
			"name":        fmt.Sprintf("Autoglyph #%s", t.TokenID.Base10String()),
			"description": "Autoglyphs are the first “on-chain” generative art on the Ethereum blockchain. A completely self-contained mechanism for the creation and ownership of an artwork.",
			"image":       fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes()[svgStart:])),
		}, nil
	}
}

func newColorglyphHandler(ethClient *ethclient.Client) metadataHandler {
	/*
	 *
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
	return func(ctx context.Context, t persist.TokenIdentifiers, _ ...persist.TokenMetadata) (persist.TokenMetadata, error) {
		tURI, err := rpc.RetryGetTokenURI(ctx, "", persist.EthereumAddress(t.ContractAddress), t.TokenID, ethClient)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		spl := strings.Split(string(tURI), " ")
		if len(spl) != 3 {
			panic("invalid colorglyphs tokenURI")
		}

		// find the index of the first character after data:text/plain;charset=utf-8, in spl[0]
		start := strings.Index(spl[0], ",") + 1
		if start == -1 {
			return persist.TokenMetadata{}, fmt.Errorf("invalid colorglyphs tokenURI")
		}

		spl[0] = strings.ReplaceAll(spl[0], "\n", "")
		spl[0] = strings.ReplaceAll(spl[0], "%0A", "")
		spl[0] = spl[0][start:]

		allColorsArray := make([]color.RGBA, 35)
		for i := 0; i < 35; i++ {
			col, err := parseHexColor(spl[2][i : i+6])
			if err != nil {
				panic(err)
			}
			allColorsArray[i] = col
		}

		allColors := allColorsArray[:]

		// sort colors by value
		sort.SliceStable(allColors, func(i, j int) bool {
			return getLightness(allColors[i]) > getLightness(allColors[j])
		})

		lightestColor := allColors[0]
		secondLightestColor := allColors[1]
		thirdLightestColor := allColors[2]
		fourthLightestColor := allColors[3]
		fifthLightestColor := allColors[4]
		darkestColor := allColors[34]

		sort.SliceStable(allColors, func(i, j int) bool {
			initialR, initialG, initialB, _ := allColors[i].RGBA()
			secondR, secondG, secondB, _ := allColors[j].RGBA()

			return initialR-initialG-initialB < secondR-secondG-secondB
		})
		reddestColor := allColors[0]
		sort.SliceStable(allColors, func(i, j int) bool {
			initialR, _, initialB, _ := allColors[i].RGBA()
			secondR, _, secondB, _ := allColors[j].RGBA()
			return initialR-initialB > secondR-secondB
		})
		orangestColor := allColors[0]
		sort.SliceStable(allColors, func(i, j int) bool {
			initialR, initialG, initialB, _ := allColors[i].RGBA()
			secondR, secondG, secondB, _ := allColors[j].RGBA()
			return initialR+initialG-initialB > secondR+secondG-secondB
		})
		yellowestColor := allColors[0]
		sort.SliceStable(allColors, func(i, j int) bool {
			initialR, initialG, initialB, _ := allColors[i].RGBA()
			secondR, secondG, secondB, _ := allColors[j].RGBA()
			return initialG-initialR-initialB > secondG-secondR-secondB
		})
		greenestColor := allColors[0]
		sort.SliceStable(allColors, func(i, j int) bool {
			initialR, initialG, initialB, _ := allColors[i].RGBA()
			secondR, secondG, secondB, _ := allColors[j].RGBA()
			return initialB-initialG-initialR > secondB-secondG-secondR
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

		width := 368
		height := 368
		add := 3
		buf := &bytes.Buffer{}
		canvas := svg.New(buf)
		canvas.Start(width, height)
		canvas.Square(0, 0, width, canvas.RGB(int(backgroundColor.R), int(backgroundColor.G), int(backgroundColor.B)))
		for i, c := range spl[0] {
			y := int(math.Floor(float64(i)/float64(64))*5) + 28
			x := ((i % 64) * 5) + 28
			col := schemeColors[int(math.Floor(float64(int(c)+i)/float64(len(schemeColors))))%len(schemeColors)]
			stroke := fmt.Sprintf(`stroke="rgb(%d,%d,%d)"`, col.R, col.G, col.B)
			switch c {
			case 'O':
				canvas.Circle(x, y, add-1, stroke, `stroke-width="0.7"`, `stroke-linecap="butt"`, `fill="none"`, "stroke-opacity: 1.0")
			case '+':
				canvas.Line(x-add, y, x+add, y, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
				canvas.Line(x, y-add, x, y+add, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
			case 'X':
				canvas.Line(x-add, y-add, x+add, y+add, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
				canvas.Line(x+add, y-add, x-add, y+add, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
			case '|':
				canvas.Line(x, y-add, x, y+add, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
			case '-':
				canvas.Line(x-add, y, x+add, y, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
			case '\\':
				canvas.Line(x-add, y+add, x+add, y-add, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
			case '/':
				canvas.Line(x-add, y-add, x+add, y+add, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
			case '#':
				canvas.Rect(x-int(math.Ceil(float64(add)/2.0)), y-add, add+1, add+1, stroke, `stroke-width="0.8"`, `stroke-linecap="square"`, "stroke-opacity: 1.0")
			}
		}
		canvas.End()

		// cut off everything before the svg tag in the buffer
		svgStart := bytes.Index(buf.Bytes(), []byte("<svg"))
		if svgStart == -1 {
			return persist.TokenMetadata{}, fmt.Errorf("no svg tag found in response")
		}
		return persist.TokenMetadata{
			"name":        fmt.Sprintf("Colorglyph #%s", t.TokenID.Base10String()),
			"description": fmt.Sprintf("A Colorglyph with color scheme %s. Created by %s.", spl[1], spl[2]),
			"image":       fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes()[svgStart:])),
		}, nil
	}
}

var (
	white = color.RGBA{255, 255, 255, 255}
	black = color.RGBA{2, 4, 8, 0}
)

func getLightness(c color.RGBA) uint32 {
	r, g, b, _ := c.RGBA()
	return r + g + b
}

func parseHexColor(s string) (c color.RGBA, err error) {
	h, err := colorful.Hex(fmt.Sprintf("#%s", s))
	if err != nil {
		return c, err
	}
	r, g, b := h.RGB255()
	return color.RGBA{r, g, b, 255}, nil
}

func newEnsHandler() metadataHandler {
	return func(ctx context.Context, t persist.TokenIdentifiers, _ ...persist.TokenMetadata) (persist.TokenMetadata, error) {
		var resp *http.Response
		var m persist.TokenMetadata

		u := fmt.Sprintf("https://metadata.ens.domains/mainnet/%s/%s", t.ContractAddress, t.TokenID.Base10String())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		defer resp.Body.Close()

		err = json.NewDecoder(resp.Body).Decode(&m)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		if errMsg, ok := m["message"].(string); ok && errMsg != "" {
			return persist.TokenMetadata{}, fmt.Errorf(errMsg)
		}

		name, ok := m["name"].(string)
		if !ok {
			return persist.TokenMetadata{}, fmt.Errorf("no ENS name found in metadata")
		}

		width := 240
		height := 240
		buf := &bytes.Buffer{}
		canvas := svg.New(buf)
		canvas.Start(width, height)
		canvas.Square(0, 0, width, canvas.RGB(255, 255, 255))

		canvas.Text(width/2, height/2, name, `font-size="16px"`, `text-anchor="middle"`, `alignment-baseline="middle"`, `font-family="Helvetica Neue"`)

		canvas.End()

		// cut off everything before the svg tag in the buffer
		svgStart := bytes.Index(buf.Bytes(), []byte("<svg"))
		if svgStart == -1 {
			return persist.TokenMetadata{}, fmt.Errorf("no svg tag found in response")
		}

		return persist.TokenMetadata{
			"name":          fmt.Sprintf("ENS: %s", name),
			"description":   "ENS names are used to resolve domain names to Ethereum addresses.",
			"image":         fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes()[svgStart:])),
			"profile_image": fmt.Sprintf("https://metadata.ens.domains/mainnet/avatar/%s", name),
		}, nil
	}
}

func newCryptopunkHandler(ethClient *ethclient.Client) metadataHandler {
	return func(ctx context.Context, t persist.TokenIdentifiers, _ ...persist.TokenMetadata) (persist.TokenMetadata, error) {
		dataContract, err := contracts.NewCryptopunksDataCaller(ethcommon.HexToAddress("0x16f5a35647d6f03d5d3da7b35409d65ba03af3b2"), ethClient)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		punkSVG, err := dataContract.PunkImageSvg(&bind.CallOpts{Context: ctx}, uint16(t.TokenID.ToInt()))
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		removedPrefix := strings.TrimPrefix(punkSVG, "data:image/svg+xml;utf8,")
		asBase64 := base64.RawStdEncoding.EncodeToString([]byte(removedPrefix))
		withBase64Prefix := fmt.Sprintf("data:image/svg+xml;base64,%s", asBase64)

		return persist.TokenMetadata{
			"name":        fmt.Sprintf("Cryptopunks: %s", t.TokenID.Base10String()),
			"description": "CryptoPunks launched as a fixed set of 10,000 items in mid-2017 and became one of the inspirations for the ERC-721 standard. They have been featured in places like The New York Times, Christie’s of London, Art|Basel Miami, and The PBS NewsHour.",
			"image":       withBase64Prefix,
		}, nil

	}
}

func newZoraHandler(ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) metadataHandler {
	return func(ctx context.Context, t persist.TokenIdentifiers, _ ...persist.TokenMetadata) (persist.TokenMetadata, error) {
		metadataContract, err := contracts.NewZoraCaller(ethcommon.HexToAddress(t.ContractAddress.String()), ethClient)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		metadataURI, err := metadataContract.TokenMetadataURI(&bind.CallOpts{Context: ctx}, t.TokenID.BigInt())
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		tokenMetadata, err := rpc.GetMetadataFromURI(ctx, persist.TokenURI(metadataURI), ipfsClient, arweaveClient)
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		resultMetadata := persist.TokenMetadata{}
		resultMetadata["name"] = util.FindFirstFieldFromMap(tokenMetadata, "name", "title")
		resultMetadata["description"] = util.FindFirstFieldFromMap(tokenMetadata, "description", "desc", "notes")

		mediaURI, err := metadataContract.TokenURI(&bind.CallOpts{Context: ctx}, t.TokenID.BigInt())
		if err != nil {
			return persist.TokenMetadata{}, err
		}

		contentType, ok := util.FindFirstFieldFromMap(tokenMetadata, "mimeType", "contentType", "content-type", "type").(string)
		var mediaType persist.MediaType
		if ok {
			mediaType = media.MediaFromContentType(contentType)
		} else {
			mediaType, _, _, _ = media.PredictMediaType(ctx, mediaURI)
		}
		switch mediaType {
		case persist.MediaTypeImage, persist.MediaTypeGIF, persist.MediaTypeSVG:
			resultMetadata["image"] = mediaURI
		default:
			resultMetadata["animation_url"] = mediaURI
			someOtherURI, ok := util.FindFirstFieldFromMap(tokenMetadata, "image", "thumbnail", "uri").(string)
			if ok {
				resultMetadata["image"] = someOtherURI
			}
		}
		for k, v := range tokenMetadata {
			if k == "name" || k == "description" || k == "image" || k == "animation_url" {
				continue
			}
			resultMetadata[k] = v
		}

		return resultMetadata, nil
	}
}
func newOpenseaSharedStorefrontHandler() metadataHandler {
	return func(ctx context.Context, t persist.TokenIdentifiers, oldMetadata ...persist.TokenMetadata) (persist.TokenMetadata, error) {
		var m persist.TokenMetadata
		var err error

		// Determine whether to use existing metadata or fetch new metadata
		if len(oldMetadata) > 0 {
			m = oldMetadata[0]
		}

		imgKey, imgURL, _, _, err := media.FindMediaURLsKeysChain(m, t.Chain)
		// If there is no image or error occurred, return the metadata as is
		if err != nil {
			return m, nil
		}

		u, err := url.Parse(string(imgURL))
		if err != nil {
			return m, nil
		}

		query := u.Query()
		// Opensea uses imgix for image resizing. We add a width query parameter with the
		// maximum width to get the highest resolution image.
		if u.Hostname() == "i.seadn.io" {
			query.Set("w", "8120")
			u.RawQuery = query.Encode()
			m[imgKey] = u.String()
		}

		return m, nil
	}
}
