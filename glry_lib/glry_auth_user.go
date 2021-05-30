package glry_lib

import (
	"fmt"
	"net/http"
	"context"
	"time"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common/hexutil"
	log "github.com/sirupsen/logrus"
	"github.com/gloflow/gloflow/go/gf_core"
	"github.com/mikeydub/go-gallery/glry_core"
	"github.com/mikeydub/go-gallery/glry_db"
)

//-------------------------------------------------------------
func AuthUserUpdatePipeline(pInput *GLRYauthUserUpdateInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gf_core.Gf_error {





	return nil


	
}

//-------------------------------------------------------------
// LOGIN_AND_MEMORIZE_ATTEMPT__PIPELINE
func AuthUserLoginAndMemorizeAttemptPipeline(pInput *GLRYauthUserLoginInput,
	pReq     *http.Request,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (bool, string, *gf_core.Gf_error) {

	//------------------
	// LOGIN
	validBool, jwtStr, nonceValueStr, gErr := AuthUserLoginPipeline(pInput, pCtx, pRuntime)
	if gErr != nil {
		return false, "", gErr
	}

	//------------------
	// LOGIN_ATTEMPT
	creationTimeUNIXf := float64(time.Now().UnixNano())/1000000000.0
	IDstr             := glry_db.AuthUserLoginAttemptCreateID(pInput.UsernameStr, pInput.AddressStr, pInput.SignatureStr, creationTimeUNIXf)
	
	loginAttempt := &glry_db.GLRYuserLoginAttempt {
		VersionInt:    0,
		ID:            IDstr,
		CreationTimeF: creationTimeUNIXf,
		AddressStr:    pInput.AddressStr,
		SignatureStr:  pInput.SignatureStr,
		NonceValueStr: nonceValueStr,
		UsernameStr:   pInput.UsernameStr,
		ValidBool:     validBool,

		ReqHostAddrStr: pReq.RemoteAddr,
		ReqHeaders:     map[string][]string(pReq.Header),
	}

	// DB
	gErr = glry_db.AuthUserLoginAttemptCreate(loginAttempt, pCtx, pRuntime)
	if gErr != nil {
		return false, "", gErr
	}

	//------------------
	return validBool, jwtStr, gErr
}

//-------------------------------------------------------------
// USER_LOGIN__PIPELINE
func AuthUserLoginPipeline(pInput *GLRYauthUserLoginInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (bool, string, string, *gf_core.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return false, "", "", gErr
	}

	//------------------
	// GET_NONCE - get latest nonce for this user_address from the DB

	nonce, gErr := glry_db.AuthNonceGet(pInput.AddressStr,
		pCtx,
		pRuntime)
	if gErr != nil {
		return false, "", "", gErr
	}

	//------------------
	// VERIFY_SIGNATURE

	dataStr := nonce.ValueStr
	validBool, gErr := AuthVerifySignatureAllMethods(pInput.SignatureStr,
		dataStr,
		pInput.AddressStr,
		pRuntime) 
	if gErr != nil {
		return false, "", "", gErr
	}

	//------------------
	// SIGNATURE_VALID

	var jwtTokenStr string
	if validBool {

		//------------------
		// JWT_GENERATION - signature is valid, so generate JWT key

		jwtTokenStr, gErr = AuthJWTgeneratePipeline(pInput.AddressStr,
			pCtx,
			pRuntime)
		if gErr != nil {
			return false, jwtTokenStr, "", gErr
		}

		//------------------
	}

	return validBool, jwtTokenStr, nonce.ValueStr, nil
}

//-------------------------------------------------------------
// VERIFY_SIGNATURE_ALL_METHODS

func AuthVerifySignatureAllMethods(pSignatureStr string,
	pDataStr    string,
	pAddressStr glry_db.GLRYuserAddress,
	pRuntime    *glry_core.Runtime) (bool, *gf_core.Gf_error) {

	// DATA_HEADER - TRUE
	validBool, gErr := AuthVerifySignature(pSignatureStr,
		pDataStr,
		pAddressStr,
		true, // pUseDataHeaderBool
		pRuntime)
	if gErr != nil {
		return false, gErr
	}

	// DATA_HEADER - FALSE
	if !validBool {
		validBool, gErr = AuthVerifySignature(pSignatureStr,
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

func AuthVerifySignature(pSignatureStr string,
	pDataStr           string,
	pAddressStr        glry_db.GLRYuserAddress,
	pUseDataHeaderBool bool,
	pRuntime           *glry_core.Runtime) (bool, *gf_core.Gf_error) {

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
	dataHash     := crypto.Keccak256Hash(dataBytesLst)

	// SIGNATURE
	log.WithFields(log.Fields{"signature": pSignatureStr,}).Debug("signature to verify")
	log.WithFields(log.Fields{"header":    pUseDataHeaderBool,}).Debug("use 'Ethereum Signed Message' header in verifying")
	log.WithFields(log.Fields{"data":      pDataStr,}).Debug("data that was signed")

	signature, err := hexutil.Decode(pSignatureStr)
	if err != nil {
		gErr := gf_core.Error__create("failed to hex decode a ECDSA signature",
			"crypto_hex_decode",
			map[string]interface{}{
				"signature": pSignatureStr,
			},
			err, "glry_lib", pRuntime.RuntimeSys)

		return false, gErr
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

	log.WithFields(log.Fields{"id": signature[len(signature)-1],}).Debug("signature last byte (recovery ID)")
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
		gErr := gf_core.Error__create("failed to recover PublicKey from ECDSA public key",
			"crypto_ec_recover_pubkey",
			map[string]interface{}{},
			err, "glry_lib", pRuntime.RuntimeSys)
		return false, gErr
	}

	publicKeyBytesLst := crypto.CompressPubkey(publicKey) // []byte(publicKey)

	//------------------

	var validBool bool

	//------------------
	// ADDRESSES_COMPARE - compare the address derived from the pubkey (which was derived from signature/data)
	//                     with the address supplied to this function as the one thats expected to be the address
	//                     sending the signature.
	//                     malicious actor could send a different address from the address derived from the pubkey
	//                     correlating to the private key used to generate the signature.
	pubkeyAddressHexStr := crypto.PubkeyToAddress(*publicKey).Hex()

	log.WithFields(log.Fields{"address": pubkeyAddressHexStr,}).Debug("derived address from sig pubkey")
	log.WithFields(log.Fields{"address": pAddressStr,}).Debug("registered address with the msg/nonce")

	if pubkeyAddressHexStr != string(pAddressStr) {
		validBool = false
		return validBool, nil
	}

	//------------------
	// VERIFY
	validBool = crypto.VerifySignature(publicKeyBytesLst, dataHash.Bytes(), signatureNoRecoverIDbytesLst)

	//------------------

	return validBool, nil
}

//-------------------------------------------------------------
// USER_GET_PREFLIGHT__PIPELINE
func AuthUserGetPreflightPipeline(pInput *GLRYauthUserGetPreflightInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYauthUserGetPublicInfoOutput, *gf_core.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	//------------------

	var nonce *glry_db.GLRYuserNonce
	var userExistsBool bool
	//-------------------------------------------------------------
	dbTXfun := func() *gf_core.Gf_error {

		// DB_GET_USER_BY_ADDRESS
		user, gErr := glry_db.AuthUserGetByAddress(pInput.AddressStr, pCtx, pRuntime)
		if gErr != nil {
			return gErr
		}

		// NO_USER_FOUND - user doesnt exist in the system, and so return an empty response
		//                 to the front-end. subsequently the client has to create a new user.
		if user == nil {

			
			// NONCE_CREATE
			nonce, gErr = AuthNonceCreatePipeline(glry_db.GLRYuserID(""), pInput.AddressStr, pCtx, pRuntime)
			if gErr != nil {
				return gErr
			}

			userExistsBool = false
			return nil
		} else {
			userExistsBool = true
		}

		// NONCE_GET
		nonce, gErr = glry_db.AuthNonceGet(pInput.AddressStr, pCtx, pRuntime)
		if gErr != nil {
			return gErr
		}

		return nil
	}

	//-------------------------------------------------------------


	// TX_RUN
	txSession, gErr := gf_core.MongoTXrun(dbTXfun,
		map[string]interface{}{
			"address":        pInput.AddressStr,
			"caller_err_msg": "failed to run DB transaction for getting user public_info",
		},
		pRuntime.DB.MongoClient,
		pCtx,
		pRuntime.RuntimeSys)
	if gErr != nil {
		return nil, gErr
	}
	defer txSession.EndSession(pCtx)
	

	
	
	output := &GLRYauthUserGetPublicInfoOutput{
		NonceStr:       nonce.ValueStr,
		UserExistsBool: userExistsBool,
	}
	return output, nil
}

//-------------------------------------------------------------
func AuthUserGetPipeline(pInput *GLRYauthUserGetInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*GLRYauthUserGetOutput, *gf_core.Gf_error) {



	user, gErr := glry_db.AuthUserGetByAddress(pInput.AddressStr,
		pCtx,
		pRuntime)
	if gErr != nil {
		return nil, gErr
	}


	output := &GLRYauthUserGetOutput{
		UserNameStr:    user.UserNameStr,
		DescriptionStr: user.DescriptionStr, 
	}

	return output, nil
}

//-------------------------------------------------------------
// USER_CREATE__PIPELINE
func AuthUserCreatePipeline(pInput *GLRYauthUserCreateInput,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) (*glry_db.GLRYuser, *gf_core.Gf_error) {

	//------------------
	// VALIDATE
	gErr := glry_core.Validate(pInput, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	//------------------

	creationTimeUNIXf := float64(time.Now().UnixNano())/1000000000.0
	// nameStr           := pInput.NameStr
	addressStr        := pInput.AddressStr
	IDstr := glry_db.AuthUserCreateID(addressStr,
		creationTimeUNIXf)

	

	user := &glry_db.GLRYuser{
		VersionInt:    0,
		IDstr:         IDstr,
		CreationTimeF: creationTimeUNIXf,
		// NameStr:       nameStr,
		AddressesLst:  []glry_db.GLRYuserAddress{addressStr, },
		// NonceInt:      nonceInt,
	}

	// DB
	gErr = glry_db.AuthUserCreate(user, pCtx, pRuntime)
	if gErr != nil {
		return nil, gErr
	}

	return user, nil
}

//-------------------------------------------------------------
// USER_DELETE__PIPELINE
func AuthUserDeletePipeline(pUserIDstr glry_db.GLRYuserID,
	pCtx     context.Context,
	pRuntime *glry_core.Runtime) *gf_core.Gf_error {

	// DELETE
	gErr := glry_db.AuthUserDelete(pUserIDstr, pCtx, pRuntime)
	if gErr != nil {
		return gErr
	}

	return nil
}