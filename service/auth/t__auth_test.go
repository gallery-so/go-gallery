package auth

import (
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestAuthVerifySignature_Success(t *testing.T) {
	assert := assert.New(t)

	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}

	testNonce := "TestNonce"
	sig := "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b"
	addr := persist.Address("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")

	success, err := VerifySignatureAllMethods(sig, testNonce, addr, WalletTypeEOA, client)
	assert.Nil(err)
	assert.True(success)
}

func TestAuthVerifySignature_WrongNonce_Failure(t *testing.T) {
	assert := assert.New(t)

	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}
	testNonce := "Wrong Nonce despite address signing sig"
	sig := "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b"
	addr := persist.Address("0x9a3f9764B21adAF3C6fDf6f947e6D3340a3F8AC5")

	success, err := VerifySignatureAllMethods(sig, testNonce, addr, WalletTypeEOA, client)
	assert.NotNil(err)
	assert.False(success)
}

func TestAuthVerifySignature_WrongAddress_Failure(t *testing.T) {
	assert := assert.New(t)

	client, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))
	if err != nil {
		panic(err)
	}

	testNonce := "TestNonce"
	sig := "0x7d3b810c5ae6efa6e5457f5ed85fe048f623b0f1127a7825f119a86714b72fec444d3fa301c05887ba1b94b77e5d68c8567171404cff43b7790e8f4d928b752a1b"
	addr := persist.Address("0x456d569592f15Af845D0dbe984C12BAB8F430e32")

	success, err := VerifySignatureAllMethods(sig, testNonce, addr, WalletTypeEOA, client)
	assert.NotNil(err)
	assert.False(success)
}
