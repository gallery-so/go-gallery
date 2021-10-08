package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/eth"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

const noncePrepend = "Gallery uses this cryptographic signature in place of a password, verifying that you are the owner of this Ethereum address: "

// INPUT - USER_LOGIN
type authUserLoginInput struct {
	Signature string `json:"signature" binding:"required,medium_string"`
	Address   string `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_LOGIN
type authUserLoginOutput struct {
	SignatureValid bool         `json:"signature_valid"`
	JWTtoken       string       `json:"jwt_token"`
	UserID         persist.DBID `json:"user_id"`
	Address        string       `json:"address"`
}

// INPUT - USER_GET_PREFLIGHT
type authUserGetPreflightInput struct {
	Address string `json:"address" form:"address" binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_GET_PREFLIGHT
type authUserGetPreflightOutput struct {
	Nonce      string `json:"nonce"`
	UserExists bool   `json:"user_exists"`
}

// HANDLERS

func getAuthPreflight(userRepository persist.UserRepository, authNonceRepository persist.NonceRepository, ethClient *eth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &authUserGetPreflightInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		authed := c.GetBool(authContextKey)

		output, err := authUserGetPreflightDb(c, input, authed, userRepository, authNonceRepository, ethClient)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func login(userRepository persist.UserRepository, authNonceRepository persist.NonceRepository, authLoginRepository persist.LoginAttemptRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &authUserLoginInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			c.JSON(http.StatusBadRequest, util.ErrorResponse{Error: err.Error()})
			return
		}

		output, err := authUserLoginAndMemorizeAttemptDb(
			c,
			input,
			c.Request,
			userRepository,
			authNonceRepository,
			authLoginRepository,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, util.ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, output)
	}
}

// NONCE

// NONCE_GENERATE
func generateNonce() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt := seededRand.Int()
	nonceStr := fmt.Sprintf("%d", nonceInt)
	return nonceStr
}

// LOGIN_AND_MEMORIZE_ATTEMPT__PIPELINE
func authUserLoginAndMemorizeAttemptDb(pCtx context.Context, pInput *authUserLoginInput,
	pReq *http.Request, userRepo persist.UserRepository, nonceRepo persist.NonceRepository,
	loginRepo persist.LoginAttemptRepository) (*authUserLoginOutput, error) {

	//------------------
	// LOGIN
	output, err := authUserLoginPipeline(pCtx, pInput, userRepo, nonceRepo)
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
	nonceRepo persist.NonceRepository) (*authUserLoginOutput, error) {

	output := &authUserLoginOutput{}

	nonceValueStr, userIDstr, err := getUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if err != nil {
		return nil, err
	}

	output.UserID = userIDstr

	sigValidBool, err := authVerifySignatureAllMethods(pInput.Signature,
		nonceValueStr,
		pInput.Address)
	if err != nil {
		return nil, err
	}

	output.SignatureValid = sigValidBool
	if !sigValidBool {
		return output, nil
	}

	jwtTokenStr, err := jwtGeneratePipeline(pCtx, userIDstr)
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
	pAddressStr string) (bool, error) {

	// personal_sign
	validBool, err := authVerifySignature(pSignatureStr,
		pNonce,
		pAddressStr,
		true)
	if err != nil {
		return false, err
	}

	if !validBool {
		// eth_sign
		validBool, err = authVerifySignature(pSignatureStr,
			pNonce,
			pAddressStr,
			false)
		if err != nil {
			return false, err
		}
	}

	return validBool, nil
}

// VERIFY_SIGNATURE

func authVerifySignature(pSignatureStr string,
	pDataStr string,
	pAddress string,
	pUseDataHeaderBool bool) (bool, error) {

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

	dataBytesLst := []byte(dataStr)
	dataHash := crypto.Keccak256Hash(dataBytesLst)

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
	if !strings.EqualFold(pubkeyAddressHexStr, pAddress) {
		return false, errors.New("address does not match signature")
	}

	publicKeyBytes := crypto.CompressPubkey(sigPublicKeyECDSA)

	signatureNoRecoverID := sig[:len(sig)-1]

	return crypto.VerifySignature(publicKeyBytes, dataHash.Bytes(), signatureNoRecoverID), nil

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
			// TODO magic number
			hasNFT, err := ethClient.HasNFT(pCtx, "0", pInput.Address)
			if err != nil {
				return nil, err
			}
			if !hasNFT {
				return nil, errors.New("user does not own required NFT to signup")
			}
		}

		nonce := &persist.UserNonce{
			Address: strings.ToLower(pInput.Address),
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

func authNonceRotateDb(pCtx context.Context, pAddress string, pUserID persist.DBID, nonceRepo persist.NonceRepository) error {

	newNonce := &persist.UserNonce{
		Value:   generateNonce(),
		Address: strings.ToLower(pAddress),
	}

	err := nonceRepo.Create(pCtx, newNonce)
	if err != nil {
		return err
	}
	return nil
}
