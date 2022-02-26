package auth

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
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// WalletType is the type of wallet used to sign a message
type WalletType int

const (
	// WalletTypeEOA represents an externally owned account (regular wallet address)
	WalletTypeEOA WalletType = iota
	// WalletTypeGnosis represents a smart contract gnosis safe
	WalletTypeGnosis
)

const (
	// Context keys for auth data
	userIDContextKey     = "auth.user_id"
	userAuthedContextKey = "auth.authenticated"
	authErrorContextKey  = "auth.auth_error"
)

// We don't want our cookies to expire, so their max age is arbitrarily set to 10 years.
// Note that browsers can still cap this expiration time (e.g. Brave limits cookies to 6 months).
const cookieMaxAge int = 60 * 60 * 24 * 365 * 10

// NoncePrepend is what is prepended to every nonce
const NoncePrepend = "Gallery uses this cryptographic signature in place of a password, verifying that you are the owner of this Ethereum address: "

// NewNoncePrepend is what will now be prepended to every nonce
const NewNoncePrepend = "Gallery uses this cryptographic signature in place of a password: "

// JWTCookieKey is the key used to store the JWT token in the cookie
const JWTCookieKey = "GLRY_JWT"

var errAddressSignatureMismatch = errors.New("address does not match signature")

// ErrNonceMismatch is returned when the nonce does not match the expected nonce
var ErrNonceMismatch = errors.New("incorrect nonce input")

// ErrInvalidJWT is returned when the JWT is invalid
var ErrInvalidJWT = errors.New("invalid or expired auth token")

// ErrNoCookie is returned when there is no JWT in the request
var ErrNoCookie = errors.New("no jwt passed as cookie")

// ErrInvalidAuthHeader is returned when the auth header is invalid
var ErrInvalidAuthHeader = errors.New("invalid auth header format")

// ErrSignatureInvalid is returned when the signed nonce's signature is invalid
var ErrSignatureInvalid = errors.New("signature invalid")

var eip1271MagicValue = [4]byte{0x16, 0x26, 0xBA, 0x7E}

// LoginInput is the input to the login pipeline
type LoginInput struct {
	Signature  string          `json:"signature" binding:"signature"`
	Address    persist.Address `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	WalletType WalletType      `json:"wallet_type"`
	Nonce      string          `json:"nonce"`
}

// LoginOutput is the output of the login pipeline
type LoginOutput struct {
	SignatureValid bool         `json:"signature_valid"`
	UserID         persist.DBID `json:"user_id"`
}

// GetPreflightInput is the input to the preflight pipeline
type GetPreflightInput struct {
	Address persist.Address `json:"address" form:"address" binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
}

// GetPreflightOutput is the output of the preflight pipeline
type GetPreflightOutput struct {
	Nonce      string `json:"nonce"`
	UserExists bool   `json:"user_exists"`
}

type Authenticator interface {
	// GetDescription returns information about the authenticator for error and logging purposes.
	// NOTE: GetDescription should NOT include any sensitive data (passwords, auth tokens, etc)
	// that we wouldn't want showing up in logs!
	GetDescription() string

	Authenticate(context.Context) (*AuthResult, error)
}

type AuthResult struct {
	UserID    persist.DBID
	Addresses []persist.Address
}

type ErrAuthenticationFailed struct {
	WrappedErr error
}

func (e ErrAuthenticationFailed) Unwrap() error {
	return e.WrappedErr
}

func (e ErrAuthenticationFailed) Error() string {
	return fmt.Sprintf("authentication failed: %s", e.WrappedErr.Error())
}

type ErrSignatureVerificationFailed struct {
	WrappedErr error
}

func (e ErrSignatureVerificationFailed) Unwrap() error {
	return e.WrappedErr
}

func (e ErrSignatureVerificationFailed) Error() string {
	return fmt.Sprintf("signature verification failed: %s", e.WrappedErr.Error())
}

type ErrDoesNotOwnRequiredNFT struct {
	address persist.Address
}

func (e ErrDoesNotOwnRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by address: %s", e.address)
}

type ErrNonceNotFound struct {
	Address persist.Address
}

func (e ErrNonceNotFound) Error() string {
	return fmt.Sprintf("nonce not found for address: %s", e.Address)
}

// GenerateNonce generates a random nonce to be signed by a wallet
func GenerateNonce() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt := seededRand.Int()
	nonceStr := fmt.Sprintf("%d", nonceInt)
	return nonceStr
}

type EthereumNonceAuthenticator struct {
	Address    string
	Nonce      string
	Signature  string
	WalletType WalletType
	UserRepo   persist.UserRepository
	NonceRepo  persist.NonceRepository
	EthClient  *ethclient.Client
}

func (e EthereumNonceAuthenticator) GetDescription() string {
	return fmt.Sprintf("EthereumNonceAuthenticator(address: %s, nonce: %s, signature: %s, walletType: %v)", e.Address, e.Nonce, e.Signature, e.WalletType)
}

func (e EthereumNonceAuthenticator) Authenticate(pCtx context.Context) (*AuthResult, error) {

	address := persist.Address(e.Address)
	nonce, userID, _ := GetUserWithNonce(pCtx, address, e.UserRepo, e.NonceRepo)
	if nonce == "" {
		return nil, ErrNonceNotFound{Address: address}
	}

	if e.WalletType != WalletTypeEOA {
		if NewNoncePrepend+nonce != e.Nonce && NoncePrepend+nonce != e.Nonce {
			return nil, ErrNonceMismatch
		}
	}

	sigValid, err := VerifySignatureAllMethods(e.Signature, nonce, address, e.WalletType, e.EthClient)
	if err != nil {
		return nil, ErrSignatureVerificationFailed{err}
	}

	if !sigValid {
		return nil, ErrSignatureVerificationFailed{ErrSignatureInvalid}
	}

	err = NonceRotate(pCtx, address, userID, e.NonceRepo)
	if err != nil {
		return nil, err
	}

	// All authenticated addresses for the user, including the one they just authenticated with
	var addresses []persist.Address

	if userID != "" {
		user, err := e.UserRepo.GetByID(pCtx, userID)
		if err != nil {
			return nil, err
		}

		addresses = make([]persist.Address, len(user.Addresses), len(user.Addresses)+1)
		copy(addresses, user.Addresses)

		if !containsAddress(addresses, address) {
			addresses = addresses[:cap(addresses)]
			addresses[cap(addresses)-1] = address
		}
	} else {
		addresses = []persist.Address{address}
	}

	authResult := AuthResult{
		Addresses: addresses,
		UserID:    userID,
	}

	return &authResult, nil
}

// LoginREST will run the login pipeline and memorize the result
func LoginREST(pCtx context.Context, pInput LoginInput,
	pReq *http.Request, userRepo persist.UserRepository, nonceRepo persist.NonceRepository,
	loginRepo persist.LoginAttemptRepository, ec *ethclient.Client) (LoginOutput, error) {

	authenticator := EthereumNonceAuthenticator{
		Address:    pInput.Address.String(),
		Nonce:      pInput.Nonce,
		Signature:  pInput.Signature,
		WalletType: pInput.WalletType,
		UserRepo:   userRepo,
		NonceRepo:  nonceRepo,
		EthClient:  ec,
	}

	gqlOutput, err := Login(pCtx, authenticator)
	if err != nil {
		return LoginOutput{}, err
	}

	output := LoginOutput{
		SignatureValid: true,
		UserID:         persist.DBID(*gqlOutput.UserID),
	}

	return output, nil
}

// Login logs in a user with a given authentication scheme
func Login(pCtx context.Context, authenticator Authenticator) (*model.LoginPayload, error) {
	gc := util.GinContextFromContext(pCtx)

	authResult, err := authenticator.Authenticate(pCtx)
	if err != nil {
		return nil, ErrAuthenticationFailed{WrappedErr: err}
	}

	if authResult.UserID == "" {
		return nil, persist.ErrUserNotFound{Authenticator: authenticator.GetDescription()}
	}

	jwtTokenStr, err := JWTGeneratePipeline(pCtx, authResult.UserID)
	if err != nil {
		return nil, err
	}

	SetJWTCookie(gc, jwtTokenStr)

	output := model.LoginPayload{
		UserID: &authResult.UserID,
	}

	return &output, nil
}

// VerifySignatureAllMethods will verify a signature using all available methods (eth_sign and personal_sign)
func VerifySignatureAllMethods(pSignatureStr string,
	pNonce string,
	pAddressStr persist.Address, pWalletType WalletType, ec *ethclient.Client) (bool, error) {

	nonce := NewNoncePrepend + pNonce
	// personal_sign
	validBool, err := VerifySignature(pSignatureStr,
		nonce,
		pAddressStr, pWalletType,
		true, ec)

	if !validBool || err != nil {
		// eth_sign
		validBool, err = VerifySignature(pSignatureStr,
			nonce,
			pAddressStr, pWalletType,
			false, ec)
		if err != nil || !validBool {
			nonce = NoncePrepend + pNonce
			validBool, err = VerifySignature(pSignatureStr,
				nonce,
				pAddressStr, pWalletType,
				true, ec)
			if err != nil || !validBool {
				validBool, err = VerifySignature(pSignatureStr,
					nonce,
					pAddressStr, pWalletType,
					false, ec)
			}
		}
	}

	if err != nil {
		return false, err
	}

	return validBool, nil
}

// VerifySignature will verify a signature using either personal_sign or eth_sign
func VerifySignature(pSignatureStr string,
	pData string,
	pAddress persist.Address, pWalletType WalletType,
	pUseDataHeaderBool bool, ec *ethclient.Client) (bool, error) {

	// eth_sign:
	// - https://goethereumbook.org/signature-verify/
	// - http://man.hubwiz.com/docset/Ethereum.docset/Contents/Resources/Documents/eth_sign.html
	// - sign(keccak256("\x19Ethereum Signed Message:\n" + len(message) + message)))

	var data string
	if pUseDataHeaderBool {
		data = fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(pData), pData)
	} else {
		data = pData
	}

	switch pWalletType {
	case WalletTypeEOA:
		dataHash := crypto.Keccak256Hash([]byte(data))

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
	case WalletTypeGnosis:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sigValidator, err := contracts.NewISignatureValidator(pAddress.Address(), ec)
		if err != nil {
			return false, err
		}

		hashedData := crypto.Keccak256([]byte(data))
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

// GetAuthNonce will determine whether a user is permitted to log in, and if so, generate a nonce to be signed
func GetAuthNonce(pCtx context.Context, pAddress persist.Address, pPreAuthed bool,
	userRepo persist.UserRepository, nonceRepo persist.NonceRepository, ethClient *ethclient.Client) (*model.AuthNonce, error) {

	user, err := userRepo.GetByAddress(pCtx, pAddress)
	if err != nil {
		logrus.WithError(err).Error("error retrieving user by address to get login nonce")
	}

	userExistsBool := user.ID != ""

	output := model.AuthNonce{
		UserExists: &userExistsBool,
	}

	if !userExistsBool {

		if !pPreAuthed {

			req := GetAllowlistContracts()
			has := false
			for k, v := range req {

				hasNFT, err := eth.HasNFTs(pCtx, k, v, pAddress, ethClient)
				if err != nil {
					return nil, err
				}
				if hasNFT {
					has = true
					break
				}
			}
			if !has {
				return nil, ErrDoesNotOwnRequiredNFT{pAddress}
			}

		}

		nonce, err := nonceRepo.Get(pCtx, pAddress)
		if err != nil || nonce.ID == "" {
			nonce = persist.UserNonce{
				Address: pAddress,
				Value:   persist.NullString(GenerateNonce()),
			}

			err = nonceRepo.Create(pCtx, nonce)
			if err != nil {
				return nil, err
			}
		}

		output.Nonce = util.StringToPointer(NewNoncePrepend + nonce.Value.String())

	} else {
		nonce, err := nonceRepo.Get(pCtx, pAddress)
		if err != nil {
			return nil, err
		}
		output.Nonce = util.StringToPointer(NewNoncePrepend + nonce.Value.String())
	}

	return &output, nil
}

// GetAuthNonceREST will determine whether a user is permitted to log in, and if so, generate a nonce to be signed
func GetAuthNonceREST(pCtx context.Context, pInput GetPreflightInput, pPreAuthed bool,
	userRepo persist.UserRepository, nonceRepo persist.NonceRepository, ethClient *ethclient.Client) (*GetPreflightOutput, error) {

	gqlOutput, err := GetAuthNonce(pCtx, pInput.Address, pPreAuthed, userRepo, nonceRepo, ethClient)
	if err != nil {
		return nil, err
	}

	output := GetPreflightOutput{
		Nonce:      *gqlOutput.Nonce,
		UserExists: *gqlOutput.UserExists,
	}

	return &output, nil
}

// NonceRotate will rotate a nonce for a user
func NonceRotate(pCtx context.Context, pAddress persist.Address, pUserID persist.DBID, nonceRepo persist.NonceRepository) error {

	newNonce := persist.UserNonce{
		Value:   persist.NullString(GenerateNonce()),
		Address: pAddress,
	}

	err := nonceRepo.Create(pCtx, newNonce)
	if err != nil {
		return err
	}
	return nil
}

// GetUserWithNonce returns nonce value string, user id
// will return empty strings and error if no nonce found
// will return empty string if no user found
func GetUserWithNonce(pCtx context.Context, pAddress persist.Address, userRepo persist.UserRepository, nonceRepo persist.NonceRepository) (nonceValue string, userID persist.DBID, err error) {

	nonce, err := nonceRepo.Get(pCtx, pAddress)
	if err != nil {
		return nonceValue, userID, err
	}

	nonceValue = nonce.Value.String()

	user, err := userRepo.GetByAddress(pCtx, pAddress)
	if err != nil {
		return nonceValue, userID, err
	}
	if user.ID != "" {
		userID = user.ID
	} else {
		return nonceValue, userID, persist.ErrUserNotFound{Address: pAddress}
	}

	return nonceValue, userID, nil
}

// GetUserIDFromCtx returns the user ID from the context
func GetUserIDFromCtx(c *gin.Context) persist.DBID {
	return c.MustGet(userIDContextKey).(persist.DBID)
}

// GetUserAuthedFromCtx queries the context to determine whether the user is authenticated
func GetUserAuthedFromCtx(c *gin.Context) bool {
	return c.GetBool(userAuthedContextKey)
}

func GetAuthErrorFromCtx(c *gin.Context) error {
	err := c.MustGet(authErrorContextKey)

	if err == nil {
		return nil
	}

	return err.(error)
}

func SetAuthStateForCtx(c *gin.Context, userID persist.DBID, err error) {
	c.Set(userIDContextKey, userID)
	c.Set(authErrorContextKey, err)
	c.Set(userAuthedContextKey, userID != "" && err == nil)
}

// GetAllowlistContracts returns the list of addresses we allowlist against
func GetAllowlistContracts() map[persist.Address][]persist.TokenID {
	addrs := viper.GetString("CONTRACT_ADDRESSES")
	spl := strings.Split(addrs, "|")
	logrus.Info("contract addresses:", spl)
	res := make(map[persist.Address][]persist.TokenID)
	for _, addr := range spl {
		nextSpl := strings.Split(addr, "=")
		if len(nextSpl) != 2 {
			panic("invalid contract address")
		}
		addr := nextSpl[0]
		tokens := nextSpl[1]
		tokens = strings.TrimLeft(tokens, "[")
		tokens = strings.TrimRight(tokens, "]")
		logrus.Info("token_ids:", tokens)
		tokenIDs := strings.Split(tokens, ",")
		logrus.Infof("tids %v and length %d", tokenIDs, len(tokenIDs))
		res[persist.Address(addr)] = make([]persist.TokenID, len(tokenIDs))
		for i, tokenID := range tokenIDs {
			res[persist.Address(addr)][i] = persist.TokenID(tokenID)
		}
	}
	return res
}

// containsAddress checks whether an address exists in a slice
func containsAddress(a []persist.Address, b persist.Address) bool {
	for _, v := range a {
		if v == b {
			return true
		}
	}

	return false
}

// SetJWTCookie sets the cookie for auth on the response
func SetJWTCookie(c *gin.Context, token string) {
	mode := http.SameSiteStrictMode
	domain := ".gallery.so"
	httpOnly := true
	secure := true

	clientIsLocalhost := c.Request.Header.Get("Origin") == "http://localhost:3000"

	if viper.GetString("ENV") != "production" || clientIsLocalhost {
		mode = http.SameSiteNoneMode
		domain = ""
		httpOnly = false
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     JWTCookieKey,
		Value:    token,
		MaxAge:   cookieMaxAge,
		Path:     "/",
		Secure:   secure,
		HttpOnly: httpOnly,
		SameSite: mode,
		Domain:   domain,
	})
}
