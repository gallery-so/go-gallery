package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	// log "github.com/sirupsen/logrus"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/mikeydub/go-gallery/runtime"
	log "github.com/sirupsen/logrus"
	// "github.com/davecgh/go-spew/spew"
)

// INPUT - USER_LOGIN
type authUserLoginInput struct {
	Signature string `json:"signature" binding:"required,medium_string"`
	Address   string `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// OUTPUT - USER_LOGIN
type authUserLoginOutput struct {
	SignatureValid bool         `json:"signature_valid"`
	JWTtoken       string       `json:"jwt_token"`
	UserID         persist.DbID `json:"user_id"`
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

//-------------------------------------------------------------
// HANDLERS

func getAuthPreflight(pRuntime *runtime.Runtime) gin.HandlerFunc {
	return func(c *gin.Context) {

		input := &authUserGetPreflightInput{}

		if err := c.ShouldBindQuery(input); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		// GET_PUBLIC_INFO
		output, err := authUserGetPreflightDb(c, input, pRuntime)
		if err != nil {
			// TODO: log specific error and return user friendly error message instead
			c.JSON(http.StatusInternalServerError, errorResponse{Error: err.Error()})

			return
		}

		//------------------
		// OUTPUT
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

		//------------------
		// OUTPUT
		c.JSON(http.StatusOK, output)
	}
}

//-------------------------------------------------------------
// NONCE
//-------------------------------------------------------------

//-------------------------------------------------------------
// NONCE_GENERATE
func generateNonce() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt := seededRand.Int()
	nonceStr := fmt.Sprintf("%d", nonceInt)
	return nonceStr
}

//-------------------------------------------------------------
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

//-------------------------------------------------------------
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

	dataStr := nonceValueStr
	sigValidBool, err := authVerifySignatureAllMethods(pInput.Signature,
		dataStr,
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

//-------------------------------------------------------------
// VERIFY_SIGNATURE_ALL_METHODS

func authVerifySignatureAllMethods(pSignatureStr string,
	pDataStr string,
	pAddressStr string,
	pRuntime *runtime.Runtime) (bool, error) {

	// DATA_HEADER - TRUE
	validBool, gErr := authVerifySignature(pSignatureStr,
		pDataStr,
		pAddressStr,
		true, // pUseDataHeaderBool
		pRuntime)
	if gErr != nil {
		return false, gErr
	}

	// DATA_HEADER - FALSE
	if !validBool {
		validBool, gErr = authVerifySignature(pSignatureStr,
			pDataStr,
			pAddressStr,
			false, // pUseDataHeaderBool
			pRuntime)
		if gErr != nil {
			return false, gErr
		}
	}

	return validBool, nil
}

//-------------------------------------------------------------
// VERIFY_SIGNATURE

// FINISH!! - also return the reason why the signature verification failed.
//            for persistance in the LoginAttempt.

func authVerifySignature(pSignatureStr string,
	pDataStr string,
	pAddress string,
	pUseDataHeaderBool bool,
	pRuntime *runtime.Runtime) (bool, error) {

	// eth_sign:
	// - https://goethereumbook.org/signature-verify/
	// - http://man.hubwiz.com/docset/Ethereum.docset/Contents/Resources/Documents/eth_sign.html
	// - sign(keccak256("\x19Ethereum Signed Message:\n" + len(message) + message)))

	// DATA

	var dataStr string
	if pUseDataHeaderBool {
		dataStr = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(fmt.Sprintf(pDataStr)), pDataStr)
	} else {
		dataStr = pDataStr
	}

	dataBytesLst := []byte(dataStr)
	dataHash := crypto.Keccak256Hash(dataBytesLst)

	// SIGNATURE
	log.WithFields(log.Fields{"signature": pSignatureStr}).Debug("signature to verify")
	log.WithFields(log.Fields{"header": pUseDataHeaderBool}).Debug("use 'Ethereum Signed Message' header in verifying")
	log.WithFields(log.Fields{"data": pDataStr}).Debug("data that was signed")

	signature, err := hexutil.Decode(pSignatureStr)
	if err != nil {
		return false, err
	}

	//------------------
	// SIGNATURE_V_NORMALIZE

	// IMPORTANT!! - 27 - "v" is the last byte of the signature, and is either 27 or 28.
	//                    its important because with eliptic curves multiple points on the curve
	//                    can be calculated from "r" and "s" alone. this would result in 2 different pubkeys.
	//                    "v" indicates which of those 2 points to use.
	//
	// https://github.com/ethereum/go-ethereum/issues/19751
	// @karalabe commented on Jun 24, 2019:
	// Originally Ethereum used 27 / 28 (which internally is just 0 / 1, just some weird bitcoin legacy to add 27).
	// Later when we needed to support chain IDs in the signatures, the V as changed to ID*2 + 35 / ID*2 + 35.
	// However, both V's are still supported on mainnet (Homestead vs. EIP155).
	// The code was messy to pass V's around from low level crypto primitives in 27/28 notation,
	// and then later for EIP155 to subtract 27, then do the whole x2+35 magic.
	// The current logic is that the low level crypto operations returns 0/1 (because that is the canonical V value),
	// and the higher level signers (Frontier, Homestead, EIP155) convert that V to whatever Ethereum specs on top of secp256k1.
	// Use the high level signers, don't use the secp256k1 library directly. If you use the low level crypto library directly,
	// you need to be aware of how generic ECC relates to Ethereum signatures.

	log.WithFields(log.Fields{"id": signature[len(signature)-1]}).Debug("signature last byte (recovery ID)")
	if signature[64] == 27 || signature[64] == 28 {
		signature[64] -= 27
	}

	signatureNoRecoverIDbytesLst := []byte(signature[:len(signature)-1]) // remove recovery id

	//------------------
	// PUBLIC_KEY

	// EC_RECOVER - returns the address for the account that was used to create the signature.
	//              compatible with eth_sign and personal_sign.
	//
	// It is important to know that the ECDSA signature scheme allows the public key to be
	// recovered from the signed message together with the signature.
	// The recovery process is based on some mathematical computations
	// (described in the SECG: SEC 1 standard).
	// The public key recovery from the ECDSA signature is very useful in bandwidth
	// constrained or storage constrained environments (such as blockchain systems),
	// when transmission or storage of the public keys cannot be afforded.
	//

	publicKey, err := crypto.SigToPub(dataHash.Bytes(), signature)

	// publicKey, err := crypto.Ecrecover(dataHash.Bytes(), signature)
	if err != nil {
		return false, err
	}

	publicKeyBytesLst := crypto.CompressPubkey(publicKey) // []byte(publicKey)

	//------------------

	//------------------
	// ADDRESSES_COMPARE - compare the address derived from the pubkey (which was derived from signature/data)
	//                     with the address supplied to this function as the one thats expected to be the address
	//                     sending the signature.
	//                     malicious actor could send a different address from the address derived from the pubkey
	//                     correlating to the private key used to generate the signature.
	pubkeyAddressHexStr := crypto.PubkeyToAddress(*publicKey).Hex()

	log.WithFields(log.Fields{"address": pubkeyAddressHexStr}).Debug("derived address from sig pubkey")
	log.WithFields(log.Fields{"address": pAddress}).Debug("registered address with the msg/nonce")

	var validBool bool
	if pubkeyAddressHexStr == string(pAddress) {
		validBool = true
	}
	if !validBool {
		return false, errors.New("address does not match signature")
	}
	//------------------
	// VERIFY
	validBool = crypto.VerifySignature(publicKeyBytesLst, dataHash.Bytes(), signatureNoRecoverIDbytesLst)

	//------------------

	return validBool, nil
}

//-------------------------------------------------------------
// USER_GET_PREFLIGHT__PIPELINE
func authUserGetPreflightDb(pCtx context.Context, pInput *authUserGetPreflightInput,
	pRuntime *runtime.Runtime) (*authUserGetPreflightOutput, error) {

	//------------------

	// DB_GET_USER_BY_ADDRESS
	user, err := persist.UserGetByAddress(pCtx, pInput.Address, pRuntime)

	userExistsBool := user != nil

	output := &authUserGetPreflightOutput{
		UserExists: userExistsBool,
	}
	var nonce *persist.UserNonce
	if err != nil || !userExistsBool {

		nonce = &persist.UserNonce{
			Address: pInput.Address,
			Value:   generateNonce(),
		}

		// NONCE_CREATE
		_, err = persist.AuthNonceCreate(pCtx, nonce, pRuntime)
		if err != nil {
			return nil, err
		}

	} else {
		nonce, err = persist.AuthNonceGet(pCtx, pInput.Address, pRuntime)
		if err != nil {
			return nil, err
		}
	}
	output.Nonce = nonce.Value

	return output, nil
}

func authNonceRotateDb(pCtx context.Context, pAddress string, pUserID persist.DbID, pRuntime *runtime.Runtime) error {

	newNonce := &persist.UserNonce{
		Value:   generateNonce(),
		Address: pAddress,
		UserID:  pUserID,
	}

	_, err := persist.AuthNonceCreate(pCtx, newNonce, pRuntime)
	if err != nil {
		return err
	}
	return nil
}
