package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/eth"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

type walletType int

const (
	walletTypeEOA walletType = iota
	walletTypeGnosis
)

const noncePrepend = "Gallery uses this cryptographic signature in place of a password, verifying that you are the owner of this Ethereum address: "

var errAddressSignatureMismatch = errors.New("address does not match signature")

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

type authUserLoginInput struct {
	Signature  string          `json:"signature" binding:"required,medium_string"`
	Address    persist.Address `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	WalletType walletType      `json:"wallet_type"`
}

type authUserLoginOutput struct {
	SignatureValid bool            `json:"signature_valid"`
	JWTtoken       string          `json:"jwt_token"`
	UserID         persist.DBID    `json:"user_id"`
	Address        persist.Address `json:"address"`
}

type authUserGetPreflightInput struct {
	Address persist.Address `json:"address" form:"address" binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}
type authHasNFTInput struct {
	UserID persist.DBID `json:"user_id" form:"user_id" binding:"required"`
}

type authHasNFTOutput struct {
	HasNFT bool `json:"has_nft"`
}

type authUserGetPreflightOutput struct {
	Nonce      string `json:"nonce"`
	UserExists bool   `json:"user_exists"`
}

type errAddressDoesNotOwnRequiredNFT struct {
	address persist.Address
}

func getAuthPreflight(userRepository persist.UserRepository, authNonceRepository persist.NonceRepository, ethClient *eth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &authUserGetPreflightInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		authed := c.GetBool(middleware.AuthContextKey)

		output, err := authUserGetPreflightDb(c, input, authed, userRepository, authNonceRepository, ethClient)
		if err != nil {
			status := http.StatusInternalServerError
			if _, ok := err.(persist.ErrNonceNotFoundForAddress); ok {
				status = http.StatusNotFound
			}
			util.ErrResponse(c, status, err)
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func login(userRepository persist.UserRepository, authNonceRepository persist.NonceRepository, authLoginRepository persist.LoginAttemptRepository, ethClient *ethclient.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &authUserLoginInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		output, err := authUserLoginAndMemorizeAttemptDb(
			c,
			input,
			c.Request,
			userRepository,
			authNonceRepository,
			authLoginRepository,
			ethClient,
		)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func hasNFTs(userRepository persist.UserRepository, ethClient *eth.Client, tokenIDs []persist.TokenID) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &authHasNFTInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		user, err := userRepository.GetByID(c, input.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}
		has := false
		for _, addr := range user.Addresses {
			if res, _ := ethClient.HasNFTs(c, tokenIDs, addr); res {
				has = true
				break
			}
		}
		c.JSON(http.StatusOK, authHasNFTOutput{HasNFT: has})
	}
}

func generateNonce() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt := seededRand.Int()
	nonceStr := fmt.Sprintf("%d", nonceInt)
	return nonceStr
}

func authUserLoginAndMemorizeAttemptDb(pCtx context.Context, pInput *authUserLoginInput,
	pReq *http.Request, userRepo persist.UserRepository, nonceRepo persist.NonceRepository,
	loginRepo persist.LoginAttemptRepository, ec *ethclient.Client) (*authUserLoginOutput, error) {

	output, err := authUserLoginPipeline(pCtx, pInput, userRepo, nonceRepo, ec)
	if err != nil {
		return nil, err
	}

	loginAttempt := &persist.UserLoginAttempt{

		Address:        pInput.Address,
		Signature:      pInput.Signature,
		SignatureValid: output.SignatureValid,

		ReqHostAddr: pReq.RemoteAddr,
		ReqHeaders:  map[string][]string(pReq.Header),
	}

	_, err = loginRepo.Create(pCtx, loginAttempt)
	if err != nil {
		return nil, err
	}

	return output, err
}

func authUserLoginPipeline(pCtx context.Context, pInput *authUserLoginInput, userRepo persist.UserRepository,
	nonceRepo persist.NonceRepository, ec *ethclient.Client) (*authUserLoginOutput, error) {

	output := &authUserLoginOutput{}

	nonceValueStr, userIDstr, err := getUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if err != nil {
		return nil, err
	}

	sigValid, err := authVerifySignatureAllMethods(pInput.Signature,
		nonceValueStr,
		pInput.Address, pInput.WalletType, ec)
	if err != nil {
		return nil, err
	}

	output.SignatureValid = sigValid
	if !sigValid {
		return output, nil
	}

	output.UserID = userIDstr

	jwtTokenStr, err := middleware.JWTGeneratePipeline(pCtx, userIDstr)
	if err != nil {
		return nil, err
	}

	output.JWTtoken = jwtTokenStr

	err = authNonceRotateDb(pCtx, pInput.Address, userIDstr, nonceRepo)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func authVerifySignatureAllMethods(pSignatureStr string,
	pNonce string,
	pAddressStr persist.Address, pWalletType walletType, ec *ethclient.Client) (bool, error) {

	// personal_sign
	validBool, err := authVerifySignature(pSignatureStr,
		pNonce,
		pAddressStr, pWalletType,
		true, ec)

	if !validBool || err != nil {
		// eth_sign
		validBool, err = authVerifySignature(pSignatureStr,
			pNonce,
			pAddressStr, pWalletType,
			false, ec)
	}

	if err != nil {
		return false, err
	}

	return validBool, nil
}

func authVerifySignature(pSignatureStr string,
	pDataStr string,
	pAddress persist.Address, pWalletType walletType,
	pUseDataHeaderBool bool, ec *ethclient.Client) (bool, error) {

	// eth_sign:
	// - https://goethereumbook.org/signature-verify/
	// - http://man.hubwiz.com/docset/Ethereum.docset/Contents/Resources/Documents/eth_sign.html
	// - sign(keccak256("\x19Ethereum Signed Message:\n" + len(message) + message)))

	nonceWithPrepend := noncePrepend + pDataStr

	var dataStr string
	if pUseDataHeaderBool {
		dataStr = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(nonceWithPrepend), nonceWithPrepend)
	} else {
		dataStr = nonceWithPrepend
	}

	switch pWalletType {
	case walletTypeEOA:
		dataHash := crypto.Keccak256Hash([]byte(dataStr))

		sig, err := hexutil.Decode(pSignatureStr)
		if err != nil {
			return false, err
		}
		// Ledger-produced signatures have v = 0 or 1
		if sig[64] == 0 || sig[64] == 1 {
			sig[64] += 27
		}
		v := sig[64]
		if v != 27 && v != 28 {
			return false, errors.New("invalid signature (V is not 27 or 28)")
		}
		sig[64] -= 27

		sigPublicKeyECDSA, err := crypto.SigToPub(dataHash.Bytes(), sig)
		if err != nil {
			return false, err
		}

		pubkeyAddressHexStr := crypto.PubkeyToAddress(*sigPublicKeyECDSA).Hex()
		log.Println("pubkeyAddressHexStr:", pubkeyAddressHexStr)
		log.Println("pAddress:", pAddress)
		if !strings.EqualFold(pubkeyAddressHexStr, pAddress.String()) {
			return false, errAddressSignatureMismatch
		}

		publicKeyBytes := crypto.CompressPubkey(sigPublicKeyECDSA)

		signatureNoRecoverID := sig[:len(sig)-1]

		return crypto.VerifySignature(publicKeyBytes, dataHash.Bytes(), signatureNoRecoverID), nil
	case walletTypeGnosis:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sigValidator, err := contracts.NewISignatureValidator(pAddress.Address(), ec)
		if err != nil {
			return false, err
		}

		hashedData := crypto.Keccak256([]byte(dataStr))
		var input [32]byte
		copy(input[:], hashedData)

		result, err := sigValidator.IsValidSignature(&bind.CallOpts{Context: ctx}, input, []byte{})
		if err != nil {
			logrus.WithError(err).Error("IsValidSignature")
			return false, nil
		}

		return result == eip1271MagicValue, nil
	default:
		return false, errors.New("wallet type not supported")
	}

}

func authUserGetPreflightDb(pCtx context.Context, pInput *authUserGetPreflightInput, pPreAuthed bool,
	userRepo persist.UserRepository, nonceRepo persist.NonceRepository, ethClient *eth.Client) (*authUserGetPreflightOutput, error) {

	user, err := userRepo.GetByAddress(pCtx, pInput.Address)

	logrus.WithError(err).Error("error retrieving user by address for auth preflight")

	userExistsBool := user != nil

	output := &authUserGetPreflightOutput{
		UserExists: userExistsBool,
	}
	if !userExistsBool {

		if !pPreAuthed {

			hasNFT, err := ethClient.HasNFTs(pCtx, middleware.RequiredNFTs, pInput.Address)
			if err != nil {
				return nil, err
			}
			if !hasNFT {
				return nil, errAddressDoesNotOwnRequiredNFT{pInput.Address}
			}

		}

		nonce := &persist.UserNonce{
			Address: pInput.Address,
			Value:   generateNonce(),
		}

		err := nonceRepo.Create(pCtx, nonce)
		if err != nil {
			return nil, err
		}
		output.Nonce = noncePrepend + nonce.Value

	} else {
		nonce, err := nonceRepo.Get(pCtx, pInput.Address)
		if err != nil {
			return nil, err
		}
		output.Nonce = noncePrepend + nonce.Value
	}

	return output, nil
}

func authNonceRotateDb(pCtx context.Context, pAddress persist.Address, pUserID persist.DBID, nonceRepo persist.NonceRepository) error {

	newNonce := &persist.UserNonce{
		Value:   generateNonce(),
		Address: pAddress,
	}

	err := nonceRepo.Create(pCtx, newNonce)
	if err != nil {
		return err
	}
	return nil
}

func (e errAddressDoesNotOwnRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by address: %s", e.address)
}
