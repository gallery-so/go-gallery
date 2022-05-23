package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/mikeydub/go-gallery/service/logger"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

type Wallet struct {
	Address    persist.Address
	Chain      persist.Chain
	WalletType persist.WalletType
}

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

// ErrAddressSignatureMismatch is returned when the address signature does not match the address cryptographically
var ErrAddressSignatureMismatch = errors.New("address does not match signature")

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

// LoginInput is the input to the login pipeline
type LoginInput struct {
	Signature  string             `json:"signature" binding:"signature"`
	Address    persist.Address    `json:"address"   binding:"required"`
	Chain      persist.Chain      `json:"chain"`
	WalletType persist.WalletType `json:"wallet_type"`
	Nonce      string             `json:"nonce"`
}

// LoginOutput is the output of the login pipeline
type LoginOutput struct {
	SignatureValid bool         `json:"signature_valid"`
	UserID         persist.DBID `json:"user_id"`
}

// GetPreflightInput is the input to the preflight pipeline
type GetPreflightInput struct {
	Address persist.Address `json:"address" form:"address" binding:"required"`
	Chain   persist.Chain
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
	UserID  persist.DBID
	Wallets []Wallet
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
	addresses []persist.Address
}

func (e ErrDoesNotOwnRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by any addresses: %s", e.addresses)
}

type ErrNonceNotFound struct {
	Address persist.Address
	Chain   persist.Chain
}

func (e ErrNonceNotFound) Error() string {
	return fmt.Sprintf("nonce not found for address: %s - %d", e.Address, e.Chain)
}

// GenerateNonce generates a random nonce to be signed by a wallet
func GenerateNonce() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt := seededRand.Int()
	nonceStr := fmt.Sprintf("%d", nonceInt)
	return nonceStr
}

type NonceAuthenticator struct {
	Address            persist.Address
	Chain              persist.Chain
	Nonce              string
	Signature          string
	WalletType         persist.WalletType
	UserRepo           persist.UserRepository
	NonceRepo          persist.NonceRepository
	WalletRepo         persist.WalletRepository
	EthClient          *ethclient.Client
	MultichainProvider *multichain.Provider
}

func (e NonceAuthenticator) GetDescription() string {
	return fmt.Sprintf("EthereumNonceAuthenticator(address: %s, nonce: %s, signature: %s, walletType: %v)", e.Address, e.Nonce, e.Signature, e.WalletType)
}

func (e NonceAuthenticator) Authenticate(pCtx context.Context) (*AuthResult, error) {

	nonce, userID, _ := GetUserWithNonce(pCtx, e.Address, e.Chain, e.UserRepo, e.NonceRepo, e.WalletRepo)
	if nonce == "" {
		return nil, ErrNonceNotFound{Address: e.Address, Chain: e.Chain}
	}

	if e.WalletType != persist.WalletTypeEOA {
		if NewNoncePrepend+nonce != e.Nonce && NoncePrepend+nonce != e.Nonce {
			return nil, ErrNonceMismatch
		}
	}

	sigValid, err := e.MultichainProvider.VerifySignature(pCtx, e.Signature, nonce, e.Address, e.Chain, e.WalletType)
	if err != nil {
		return nil, ErrSignatureVerificationFailed{err}
	}

	if !sigValid {
		return nil, ErrSignatureVerificationFailed{ErrSignatureInvalid}
	}

	err = NonceRotate(pCtx, e.Address, e.Chain, userID, e.NonceRepo)
	if err != nil {
		return nil, err
	}
	var wallets []Wallet

	if userID != "" {
		user, err := e.UserRepo.GetByID(pCtx, userID)
		if err != nil {
			return nil, err
		}

		wallets = make([]Wallet, len(user.Wallets), len(user.Wallets)+1)
		for i, wallet := range user.Wallets {
			wallets[i] = Wallet{Address: wallet.Address, Chain: wallet.Chain, WalletType: wallet.WalletType}
		}

		if !containsAddress(wallets, Wallet{Address: e.Address, Chain: e.Chain, WalletType: e.WalletType}) {
			_, err := e.WalletRepo.Insert(pCtx, e.Address, e.Chain, e.WalletType)
			if err != nil {
				return nil, err
			}
			wallet, err := e.WalletRepo.GetByAddressDetails(pCtx, e.Address, e.Chain)
			if err != nil {
				return nil, err
			}
			wallets = wallets[:cap(wallets)]
			wallets[cap(wallets)-1] = Wallet{Address: wallet.Address, Chain: wallet.Chain, WalletType: wallet.WalletType}
		}
	} else {
		_, err := e.WalletRepo.Insert(pCtx, e.Address, e.Chain, e.WalletType)
		if err != nil {
			return nil, err
		}
		wallet, err := e.WalletRepo.GetByAddressDetails(pCtx, e.Address, e.Chain)
		if err != nil {
			return nil, err
		}
		wallets = []Wallet{{Address: wallet.Address, Chain: wallet.Chain, WalletType: wallet.WalletType}}
	}

	authResult := AuthResult{
		Wallets: wallets,
		UserID:  userID,
	}

	return &authResult, nil
}

// LoginREST will run the login pipeline and memorize the result
func LoginREST(pCtx context.Context, pInput LoginInput,
	pReq *http.Request, userRepo persist.UserRepository, nonceRepo persist.NonceRepository,
	loginRepo persist.LoginAttemptRepository, ec *ethclient.Client) (LoginOutput, error) {

	authenticator := NonceAuthenticator{
		Address:    pInput.Address,
		Nonce:      pInput.Nonce,
		Signature:  pInput.Signature,
		WalletType: pInput.WalletType,
		UserRepo:   userRepo,
		NonceRepo:  nonceRepo,
		EthClient:  ec,
	}

	userID, err := Login(pCtx, authenticator)
	if err != nil {
		return LoginOutput{}, err
	}

	output := LoginOutput{
		SignatureValid: true,
		UserID:         userID,
	}

	return output, nil
}

// Login logs in a user with a given authentication scheme
func Login(pCtx context.Context, authenticator Authenticator) (persist.DBID, error) {
	gc := util.GinContextFromContext(pCtx)

	authResult, err := authenticator.Authenticate(pCtx)
	if err != nil {
		return "", ErrAuthenticationFailed{WrappedErr: err}
	}

	if authResult.UserID == "" {
		return "", persist.ErrUserNotFound{Authenticator: authenticator.GetDescription()}
	}

	jwtTokenStr, err := JWTGeneratePipeline(pCtx, authResult.UserID)
	if err != nil {
		return "", err
	}

	SetAuthStateForCtx(gc, authResult.UserID, nil)
	SetJWTCookie(gc, jwtTokenStr)

	return authResult.UserID, nil
}

func Logout(pCtx context.Context) {
	gc := util.GinContextFromContext(pCtx)
	SetAuthStateForCtx(gc, "", ErrNoCookie)
	SetJWTCookie(gc, "")
}

// GetAuthNonce will determine whether a user is permitted to log in, and if so, generate a nonce to be signed
func GetAuthNonce(pCtx context.Context, pAddress persist.Address, pChain persist.Chain, pPreAuthed bool,
	userRepo persist.UserRepository, nonceRepo persist.NonceRepository, walletRepository persist.WalletRepository, ethClient *ethclient.Client) (nonce string, userExists bool, err error) {

	wallet, err := walletRepository.GetByAddressDetails(pCtx, pAddress, pChain)
	if err != nil {
		return "", false, err
	}
	user, err := userRepo.GetByWallet(pCtx, wallet.ID)
	if err != nil {
		logger.For(pCtx).WithError(err).Error("error retrieving user by address to get login nonce")
	}

	userExists = user.ID != ""

	if !userExists {

		if !pPreAuthed {
			if hasNft, err := HasAllowlistNFT(pCtx, []persist.Address{pAddress}, ethClient); !hasNft {
				return "", false, err
			}
		}

		dbNonce, err := nonceRepo.Get(pCtx, wallet.ID)
		if err != nil || dbNonce.ID == "" {
			create := persist.CreateNonceInput{
				Address: pAddress,
				Value:   GenerateNonce(),
				Chain:   pChain,
			}

			err = nonceRepo.Create(pCtx, create)
			if err != nil {
				return "", false, err
			}

			dbNonce, err = nonceRepo.Get(pCtx, wallet.ID)
			if err != nil {
				return "", false, err
			}
		}

		nonce = NewNoncePrepend + dbNonce.Value.String()

	} else {
		dbNonce, err := nonceRepo.Get(pCtx, wallet.ID)
		if err != nil {
			return "", false, err
		}
		nonce = NewNoncePrepend + dbNonce.Value.String()
	}

	return nonce, userExists, nil
}

func HasAllowlistNFT(ctx context.Context, addresses []persist.Address, ethClient *ethclient.Client) (bool, error) {
	allowlist := GetAllowlistContracts()
	for _, addr := range addresses {
		for k, v := range allowlist {
			found, err := eth.HasNFTs(ctx, k, v, addr, ethClient)
			if found {
				return true, nil
			} else if err != nil {
				logger.For(ctx).Warnf("error checking whether address %s owns NFTs with contractAddress: %s and ids: %v: %s\n", addr, k, v, err)
			}
		}
	}

	return false, ErrDoesNotOwnRequiredNFT{addresses: addresses}
}

// GetAuthNonceREST will determine whether a user is permitted to log in, and if so, generate a nonce to be signed
func GetAuthNonceREST(pCtx context.Context, pInput GetPreflightInput, pPreAuthed bool,
	userRepo persist.UserRepository, nonceRepo persist.NonceRepository, walletRepo persist.WalletRepository, ethClient *ethclient.Client) (*GetPreflightOutput, error) {

	nonce, userExists, err := GetAuthNonce(pCtx, pInput.Address, pInput.Chain, pPreAuthed, userRepo, nonceRepo, walletRepo, ethClient)
	if err != nil {
		return nil, err
	}

	output := GetPreflightOutput{
		Nonce:      nonce,
		UserExists: userExists,
	}

	return &output, nil
}

// NonceRotate will rotate a nonce for a user
func NonceRotate(pCtx context.Context, pAddress persist.Address, pChain persist.Chain, pUserID persist.DBID, nonceRepo persist.NonceRepository) error {

	newNonce := persist.CreateNonceInput{
		Value:   GenerateNonce(),
		Address: pAddress,
		Chain:   pChain,
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
func GetUserWithNonce(pCtx context.Context, pAddress persist.Address, pChain persist.Chain, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, walletRepository persist.WalletRepository) (nonceValue string, userID persist.DBID, err error) {

	wallet, err := walletRepository.GetByAddressDetails(pCtx, pAddress, pChain)
	if err != nil {
		return "", "", err
	}

	nonce, err := nonceRepo.Get(pCtx, wallet.ID)
	if err != nil {
		return nonceValue, userID, err
	}

	nonceValue = nonce.Value.String()

	user, err := userRepo.GetByWallet(pCtx, wallet.ID)
	if err != nil {
		return nonceValue, userID, err
	}
	if user.ID != "" {
		userID = user.ID
	} else {
		return nonceValue, userID, persist.ErrUserNotFound{Address: pAddress, Chain: pChain}
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
func GetAllowlistContracts() map[persist.EthereumAddress][]persist.TokenID {
	addrs := viper.GetString("CONTRACT_ADDRESSES")
	spl := strings.Split(addrs, "|")
	logger.For(nil).Info("contract addresses:", spl)
	res := make(map[persist.EthereumAddress][]persist.TokenID)
	for _, addr := range spl {
		nextSpl := strings.Split(addr, "=")
		if len(nextSpl) != 2 {
			panic("invalid contract address")
		}
		addr := nextSpl[0]
		tokens := nextSpl[1]
		tokens = strings.TrimLeft(tokens, "[")
		tokens = strings.TrimRight(tokens, "]")
		logger.For(nil).Info("token_ids:", tokens)
		tokenIDs := strings.Split(tokens, ",")
		logger.For(nil).Infof("tids %v and length %d", tokenIDs, len(tokenIDs))
		res[persist.EthereumAddress(addr)] = make([]persist.TokenID, len(tokenIDs))
		for i, tokenID := range tokenIDs {
			res[persist.EthereumAddress(addr)][i] = persist.TokenID(tokenID)
		}
	}
	return res
}

// containsAddress checks whether an address exists in a slice
func containsAddress(a []Wallet, b Wallet) bool {
	for _, v := range a {
		if v.Address.String() == b.Address.String() && v.Chain == b.Chain {
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

func toAuthWallets(pWallets []persist.Wallet) []Wallet {
	res := make([]Wallet, len(pWallets))
	for i, w := range pWallets {
		res[i] = Wallet{
			Address:    w.Address,
			Chain:      w.Chain,
			WalletType: w.WalletType,
		}
	}
	return res
}

// ContainsAuthWallet checks whether an auth.Wallet is in a slice of perist.Wallet
func ContainsWallet(pWallets []Wallet, pWallet Wallet) bool {
	for _, w := range pWallets {
		if w.Address == pWallet.Address && w.Chain == pWallet.Chain {
			return true
		}
	}
	return false
}
