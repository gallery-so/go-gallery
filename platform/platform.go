package platform

import (
	"net/url"
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

func IsFxhash(td db.TokenDefinition, c db.Contract) bool {
	if td.Chain == persist.ChainTezos {
		return IsFxhashTezos(td.Chain, td.ContractAddress)
	}
	if td.Chain == persist.ChainETH {
		return IsFxhashEth(td.Chain, td.ContractAddress, c.Symbol.String, td.Metadata)
	}
	return false
}

func IsFxhashTezos(chain persist.Chain, address persist.Address) bool {
	return util.Contains(FxHashContracts, persist.NewContractIdentifiers(address, chain))
}

func IsFxhashEth(chain persist.Chain, address persist.Address, contractSymbol string, tokenMetadata persist.TokenMetadata) bool {
	if chain == persist.ChainETH {
		// fxhash contracts on eth are deployed with "FXGEN" as the contract symbol
		if strings.ToLower(contractSymbol) == "fxgen" {
			return true
		}
		// check if the external_url has fxhash as the domain
		if u, ok := tokenMetadata["external_url"].(string); ok {
			parsed, _ := url.Parse(u)
			if chain == persist.ChainETH && strings.HasPrefix(parsed.Hostname(), "fxhash") {
				return true
			}
		}
	}
	return false
}

func IsFxhashSignedTezos(chain persist.Chain, address persist.Address, tokenName string) bool {
	return !IsFxhashTezos(chain, address) || tokenName != "[WAITING TO BE SIGNED]"
}

func IsFxhashSignedEth(chain persist.Chain, address persist.Address, contractSymbol string, tokenMetadata persist.TokenMetadata) bool {
	return !IsFxhashEth(chain, address, contractSymbol, tokenMetadata) || (tokenMetadata["authenticityHash"] != "" && tokenMetadata["authenticityHash"] != nil)
}

func IsFxhashSigned(td db.TokenDefinition, c db.Contract, m persist.TokenMetadata) bool {
	if !IsFxhash(td, c) {
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
func KeywordsFor(td db.TokenDefinition, c db.Contract) ([]string, []string) {
	imgK, animK := td.Chain.BaseKeywords()

	if IsHicEtNunc(td.Chain, td.ContractAddress) {
		imgK = append([]string{"artifactUri", "displayUri", "image"}, imgK...)
		return imgK, animK
	}

	if IsFxhash(td, c) {
		imgK := append([]string{"displayUri", "artifactUri", "image", "uri"}, imgK...)
		animK := append([]string{"artifactUri", "displayUri"}, animK...)
		return imgK, animK
	}

	return imgK, animK
}
