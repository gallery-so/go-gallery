package infra

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/stretchr/testify/assert"
)

func TestGetERC721sForWallet_Success(t *testing.T) {
	setupTest(t)
	assert := assert.New(t)

	resp, err := http.Get(fmt.Sprintf("%s/tokens/get?address=%s&skip_db=true", tc.serverURL, "0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5"))
	assert.Nil(err)
	assertValidJSONResponse(assert, resp)

	type response struct {
		Tokens []persist.ERC721 `json:"tokens"`
		Error  string           `json:"error"`
	}

	var r response
	util.UnmarshallBody(&r, resp.Body)
	assert.Empty(r.Error)
	assert.Greater(len(r.Tokens), 0)

}
