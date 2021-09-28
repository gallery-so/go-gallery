package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	// log "github.com/sirupsen/logrus"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	// "github.com/davecgh/go-spew/spew"
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

func getAuthPreflight(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &authUserGetPreflightInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		authed := c.GetBool(authContextKey)

		// GET_PUBLIC_INFO
		output, err := authUserGetPreflightDb(c, input, authed, pRuntime)
		if err != nil {
			// TODO: log specific error and return user friendly error message instead
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})

			return
		}

		c.JSON(http.StatusOK, output)
	}
}

func login(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := &authUserLoginInput{}
		if err := c.ShouldBindJSON(input); err != nil {
			// TODO this should be Bad Request I think
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		//------------------

		// USER_LOGIN__PIPELINE
		output, err := authUserLoginAndMemorizeAttemptDb(
			c,
			input,
			c.Request,
			pRuntime,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}

		/*
			// ADD!! - going forward we should follow this approach, after v1
			// SET_JWT_COOKIE
			expirationTime := time.Now().Add(time.Duration(pRuntime.Config.JWTtokenTTLsecInt/60) * time.Minute)
			http.SetCookie(pResp, &http.Cookie{
				Name:    "glry_token",
				Value:   userJWTtokenStr,
				Expires: expirationTime,
			})*/

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
	pReq *http.Request,
	pRuntime *runtime.Runtime) (*authUserLoginOutput, error) {

	//------------------
	// LOGIN
	output, err := authUserLoginPipeline(pCtx, pInput, pRuntime)
	if err != nil {
		return nil, err
	}

	//------------------
	// LOGIN_ATTEMPT

	loginAttempt := &persist.UserLoginAttempt{

		Address:        pInput.Address,
		Signature:      pInput.Signature,
		SignatureValid: output.SignatureValid,

		ReqHostAddr: pReq.RemoteAddr,
		ReqHeaders:  map[string][]string(pReq.Header),
	}

	// DB
	_, err = persist.AuthUserLoginAttemptCreate(pCtx, loginAttempt, pRuntime)
	if err != nil {
		return nil, err
	}

	//------------------
	return output, err
}

// USER_LOGIN__PIPELINE
func authUserLoginPipeline(pCtx context.Context, pInput *authUserLoginInput,
	pRuntime *runtime.Runtime) (*authUserLoginOutput, error) {

	//------------------
	// OUTPUT
	output := &authUserLoginOutput{}

	//------------------
	// USER_CHECK
	nonceValueStr, userIDstr, err := getUserWithNonce(pCtx, pInput.Address, pRuntime)
	if err != nil {
		return nil, err
	}

	output.UserID = userIDstr

	//------------------
	// VERIFY_SIGNATURE

	sigValidBool, err := authVerifySignatureAllMethods(pInput.Signature,
		nonceValueStr,
		pInput.Address,
		pRuntime)
	if err != nil {
		return nil, err
	}

	output.SignatureValid = sigValidBool
	if !sigValidBool {
		return output, nil
	}

	//------------------
	// JWT_GENERATION - signature is valid, so generate JWT key
	jwtTokenStr, err := jwtGeneratePipeline(
		pCtx,
		userIDstr,
		pRuntime,
	)
	if err != nil {
		return nil, err
	}

	output.JWTtoken = jwtTokenStr

	//------------------
	// NONCE ROTATE

	err = authNonceRotateDb(pCtx, pInput.Address, userIDstr, pRuntime)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func authVerifySignatureAllMethods(pSignatureStr string,
	pNonce string,
	pAddressStr string,
	pRuntime *runtime.Runtime) (bool, error) {

	// personal_sign
	validBool, err := authVerifySignature(pSignatureStr,
		pNonce,
		pAddressStr,
		true,
		pRuntime)
	if err != nil {
		return false, err
	}

	if !validBool {
		// eth_sign
		validBool, err = authVerifySignature(pSignatureStr,
			pNonce,
			pAddressStr,
			false,
			pRuntime)
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
	pUseDataHeaderBool bool,
	pRuntime *runtime.Runtime) (bool, error) {

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
	pRuntime *runtime.Runtime) (*authUserGetPreflightOutput, error) {

	user, _ := persist.UserGetByAddress(pCtx, pInput.Address, pRuntime)

	userExistsBool := user != nil

	output := &authUserGetPreflightOutput{
		UserExists: userExistsBool,
	}
	if !userExistsBool {

		if !pPreAuthed {
			// TODO enable this when we are checking for nfts
			// hasNFT, err := hasAnyNFT(pCtx, "0x0", pInput.Address, pRuntime)
			// if err != nil {
			// 	return nil, err
			// }
			// if !hasNFT {
			// 	return nil, errors.New("user does not own required nft to signup")
			// }
		}

		nonce := &persist.UserNonce{
			Address: strings.ToLower(pInput.Address),
			Value:   generateNonce(),
		}

		_, err := persist.AuthNonceCreate(pCtx, nonce, pRuntime)
		if err != nil {
			return nil, err
		}
		output.Nonce = noncePrepend + nonce.Value

	} else {
		nonce, err := persist.AuthNonceGet(pCtx, pInput.Address, pRuntime)
		if err != nil {
			return nil, err
		}
		output.Nonce = noncePrepend + nonce.Value
	}

	return output, nil
}

func authNonceRotateDb(pCtx context.Context, pAddress string, pUserID persist.DBID, pRuntime *runtime.Runtime) error {

	newNonce := &persist.UserNonce{
		Value:   generateNonce(),
		Address: strings.ToLower(pAddress),
		UserID:  pUserID,
	}

	_, err := persist.AuthNonceCreate(pCtx, newNonce, pRuntime)
	if err != nil {
		return err
	}
	return nil
}
