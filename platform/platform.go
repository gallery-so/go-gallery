package platform

import (
	"strings"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

var (
	ProhibitionContract    = persist.NewContractIdentifiers("0x47a91457a3a1f700097199fd63c039c4784384ab", persist.ChainArbitrum)
	EnsContract            = persist.NewContractIdentifiers("0x57f1887a8bf19b14fc0df6fd9b2acc9af147ea85", persist.ChainETH)
	FxHashGentkV1Contract  = persist.NewContractIdentifiers("KT1KEa8z6vWXDJrVqtMrAeDVzsvxat3kHaCE", persist.ChainTezos)
	FxHash2GentkV2Contract = persist.NewContractIdentifiers("KT1U6EHmNxJTkvaWJ4ThczG4FSDaHC21ssvi", persist.ChainTezos)
	FxHash3GentkV3Contract = persist.NewContractIdentifiers("KT1EfsNuqwLAWDd3o4pvfUx1CAh5GMdTrRvr", persist.ChainTezos)
	FxHashArticlesContract = persist.NewContractIdentifiers("KT1GtbuswcNMGhHF2TSuH1Yfaqn16do8Qtva", persist.ChainTezos)
	HicEtNuncContract      = persist.NewContractIdentifiers("KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton", persist.ChainTezos)
	ObjktContract          = persist.NewContractIdentifiers("KT19xbD2xn6A81an18S35oKtnkFNr9CVwY5m", persist.ChainTezos)
)

var FxHashContracts = []persist.ContractIdentifiers{
	FxHashGentkV1Contract,
	FxHash2GentkV2Contract,
	FxHash3GentkV3Contract,
	FxHashArticlesContract,
}

var HicEtNuncContracts = []persist.ContractIdentifiers{
	HicEtNuncContract,
	ObjktContract,
}

func IsEns(chain persist.Chain, address persist.Address) bool {
	return persist.NewContractIdentifiers(address, chain) == EnsContract
}

func IsProhibition(chain persist.Chain, address persist.Address) bool {
	return persist.NewContractIdentifiers(address, chain) == ProhibitionContract
}

func IsHicEtNunc(chain persist.Chain, address persist.Address) bool {
	return util.Contains(HicEtNuncContracts, persist.NewContractIdentifiers(address, chain))
}

func IsFxhash(c db.Contract) bool {
	if c.Chain == persist.ChainTezos {
		return IsFxhashTezos(c.Chain, c.Address)
	}
	if c.Chain == persist.ChainETH {
		return IsFxhashEth(c.Chain, c.Address, c.Symbol.String)
	}
	return false
}

func IsFxhashTezos(chain persist.Chain, address persist.Address) bool {
	return util.Contains(FxHashContracts, persist.NewContractIdentifiers(address, chain))
}

func IsFxhashEth(chain persist.Chain, address persist.Address, contractSymbol string) bool {
	return chain == persist.ChainETH && strings.ToLower(contractSymbol) == "fxgen"
}

func IsFxhashSignedTezos(chain persist.Chain, address persist.Address, tokenName string) bool {
	return !IsFxhashTezos(chain, address) || strings.ToLower(tokenName) != "[waiting to be signed]"
}

func IsFxhashSignedEth(chain persist.Chain, address persist.Address, contractSymbol string, tokenMetadata persist.TokenMetadata) bool {
	return !IsFxhashEth(chain, address, contractSymbol) || (tokenMetadata["authenticityHash"] != "" && tokenMetadata["authenticityHash"] != nil)
}

func IsFxhashSigned(td db.TokenDefinition, c db.Contract, m persist.TokenMetadata) bool {
	if td.IsFxhash {
		return true
	}
	if td.Chain == persist.ChainTezos {
		return IsFxhashSignedTezos(td.Chain, td.ContractAddress, td.Name.String)
	}
	if td.Chain == persist.ChainETH {
		return IsFxhashSignedEth(td.Chain, td.ContractAddress, c.Symbol.String, m)
	}
	return true
}

// KeywordsFor returns the fields in a token's metadata that should be used to download assets from
func KeywordsFor(td db.TokenDefinition) ([]string, []string) {
	imgK, animK := td.Chain.BaseKeywords()

	if IsHicEtNunc(td.Chain, td.ContractAddress) {
		imgK = append([]string{"artifactUri", "displayUri", "image"}, imgK...)
		return imgK, animK
	}

	if td.IsFxhash {
		imgK := append([]string{"displayUri", "artifactUri", "image", "uri"}, imgK...)
		animK := append([]string{"artifactUri", "displayUri"}, animK...)
		return imgK, animK
	}

	return imgK, animK
}
