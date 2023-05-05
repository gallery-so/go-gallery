package auth

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/magiclabs/magic-admin-go"
	magicclient "github.com/magiclabs/magic-admin-go/client"
	"github.com/magiclabs/magic-admin-go/token"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/persist/postgres"

	"github.com/mikeydub/go-gallery/service/logger"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

// TODO: ExternalWallet? Something to denote "wallet, but not part of our ecosystem"
// Or do we call it something like "AccountInfo" or "AddressInfo" or something?
// Would also be nice to have a method on AuthResult that searches by ChainAddress
// for a result. Maybe instead of a map from address+chain+type to existing wallets
// (where they exist), we just have a wallet pointer field on our results that points
// to a user's wallet if one exists? OwnedWallet? WalletInput? RawWallet?

// AuthenticatedAddress contains address information that has been successfully verified
// by an authenticator.
type AuthenticatedAddress struct {
	// A ChainAddress that has had its ownership successfully verified by an authenticator
	ChainAddress persist.ChainAddress

	// The WalletType of the verified ChainAddress
	WalletType persist.WalletType

	// The ID of an existing Wallet entity in our database, or an empty string if the wallet is not in the database
	WalletID persist.DBID
}

const (
	// Context keys for auth data
	userIDContextKey         = "auth.user_id"
	userAuthedContextKey     = "auth.authenticated"
	authErrorContextKey      = "auth.auth_error"
	userRolesContextKey      = "auth.roles"
	userRolesExistContextKey = "auth.roles_exist"
	roleErrorContextKey      = "auth.roles_error"
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

var ErrInvalidMagicLink = errors.New("invalid magic link")

var ErrUserNotFound = errors.New("no user found")

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
	User      *persist.User
	Addresses []AuthenticatedAddress
}

func (a *AuthResult) GetAuthenticatedAddress(chainAddress persist.ChainAddress) (AuthenticatedAddress, bool) {
	for _, address := range a.Addresses {
		if address.ChainAddress == chainAddress {
			return address, true
		}
	}

	return AuthenticatedAddress{}, false
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
	addresses []persist.ChainAddress
}

func (e ErrDoesNotOwnRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by any addresses: %s", e.addresses)
}

type ErrNonceNotFound struct {
	ChainAddress persist.ChainAddress
}

func (e ErrNonceNotFound) Error() string {
	return fmt.Sprintf("nonce not found for address: %s", e.ChainAddress)
}

// GenerateNonce generates a random nonce to be signed by a wallet
func GenerateNonce() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt := seededRand.Int()
	nonceStr := fmt.Sprintf("%d", nonceInt)
	return nonceStr
}

type NonceAuthenticator struct {
	ChainPubKey        persist.ChainPubKey
	Nonce              string
	Signature          string
	WalletType         persist.WalletType
	UserRepo           *postgres.UserRepository
	NonceRepo          *postgres.NonceRepository
	WalletRepo         *postgres.WalletRepository
	EthClient          *ethclient.Client
	MultichainProvider *multichain.Provider
}

func (e NonceAuthenticator) GetDescription() string {
	return fmt.Sprintf("NonceAuthenticator(address: %s, nonce: %s, signature: %s, walletType: %v)", e.ChainPubKey, e.Nonce, e.Signature, e.WalletType)
}

func (e NonceAuthenticator) Authenticate(pCtx context.Context) (*AuthResult, error) {
	asChainAddress := e.ChainPubKey.ToChainAddress()
	nonce, user, _ := GetUserWithNonce(pCtx, asChainAddress, e.UserRepo, e.NonceRepo, e.WalletRepo)
	if nonce == "" {
		return nil, ErrNonceNotFound{ChainAddress: asChainAddress}
	}

	if e.WalletType != persist.WalletTypeEOA {
		if NewNoncePrepend+nonce != e.Nonce && NoncePrepend+nonce != e.Nonce {
			return nil, ErrNonceMismatch
		}
	}

	sigValid, err := e.MultichainProvider.VerifySignature(pCtx, e.Signature, nonce, e.ChainPubKey, e.WalletType)
	if err != nil {
		return nil, ErrSignatureVerificationFailed{err}
	}

	if !sigValid {
		return nil, ErrSignatureVerificationFailed{ErrSignatureInvalid}
	}

	err = NonceRotate(pCtx, asChainAddress, e.NonceRepo)
	if err != nil {
		return nil, err
	}

	walletID := persist.DBID("")
	wallet, err := e.WalletRepo.GetByChainAddress(pCtx, asChainAddress)
	if err != nil {
		walletID = wallet.ID
	}

	authResult := AuthResult{
		Addresses: []AuthenticatedAddress{{ChainAddress: asChainAddress, WalletType: e.WalletType, WalletID: walletID}},
		User:      user,
	}

	return &authResult, nil
}

type MagicLinkAuthenticator struct {
	Token       token.Token
	MagicClient *magicclient.API
	UserRepo    *postgres.UserRepository
}

func (e MagicLinkAuthenticator) GetDescription() string {
	return "MagicLinkAuthenticator"
}

func (e MagicLinkAuthenticator) Authenticate(pCtx context.Context) (*AuthResult, error) {

	err := e.Token.Validate()
	if err != nil {
		return nil, ErrInvalidMagicLink
	}

	info, err := e.MagicClient.User.GetMetadataByIssuer(e.Token.GetIssuer())
	if err != nil {
		return nil, ErrInvalidMagicLink
	}

	user, err := e.UserRepo.GetByEmail(pCtx, persist.Email(info.Email))
	if err != nil {
		if _, ok := err.(persist.ErrUserNotFound); !ok {
			return nil, err
		}
		return nil, ErrUserNotFound

	}

	return &AuthResult{
		User:      &user,
		Addresses: []AuthenticatedAddress{},
	}, nil
}

func NewMagicLinkClient() *magicclient.API {
	return magicclient.New(env.GetString("MAGIC_LINK_SECRET_KEY"), magic.NewDefaultClient())
}

// Login logs in a user with a given authentication scheme
func Login(pCtx context.Context, authenticator Authenticator) (persist.DBID, error) {
	gc := util.MustGetGinContext(pCtx)

	authResult, err := authenticator.Authenticate(pCtx)
	if err != nil {
		return "", ErrAuthenticationFailed{WrappedErr: err}
	}

	if authResult.User == nil || authResult.User.Universal.Bool() {
		return "", persist.ErrUserNotFound{Authenticator: authenticator.GetDescription()}
	}

	jwtTokenStr, err := JWTGeneratePipeline(pCtx, authResult.User.ID)
	if err != nil {
		return "", err
	}

	SetAuthStateForCtx(gc, authResult.User.ID, nil)
	SetJWTCookie(gc, jwtTokenStr)

	return authResult.User.ID, nil
}

func Logout(pCtx context.Context) {
	gc := util.MustGetGinContext(pCtx)
	SetAuthStateForCtx(gc, "", ErrNoCookie)
	SetJWTCookie(gc, "")
	SetRolesForCtx(gc, []persist.Role{}, nil)
}

// GetAuthNonce will determine whether a user is permitted to log in, and if so, generate a nonce to be signed
func GetAuthNonce(pCtx context.Context, pChainAddress persist.ChainAddress, userRepo *postgres.UserRepository, nonceRepo *postgres.NonceRepository,
	walletRepository *postgres.WalletRepository, earlyAccessRepo *postgres.EarlyAccessRepository, ethClient *ethclient.Client) (nonce string, userExists bool, err error) {

	user, err := userRepo.GetByChainAddress(pCtx, pChainAddress)
	if err != nil {
		logger.For(pCtx).WithError(err).Error("error retrieving user by address to get login nonce")
	}

	userExists = user.ID != "" && !user.Universal.Bool()

	if userExists {
		dbNonce, err := nonceRepo.Get(pCtx, pChainAddress)
		if err != nil {
			return "", false, err
		}

		nonce = NewNoncePrepend + dbNonce.Value.String()
		return nonce, userExists, nil
	}

	dbNonce, err := nonceRepo.Get(pCtx, pChainAddress)
	if err != nil || dbNonce.ID == "" {
		err = nonceRepo.Create(pCtx, GenerateNonce(), pChainAddress)
		if err != nil {
			return "", false, err
		}

		dbNonce, err = nonceRepo.Get(pCtx, pChainAddress)
		if err != nil {
			return "", false, err
		}
	}

	nonce = NewNoncePrepend + dbNonce.Value.String()
	return nonce, userExists, nil
}

// NonceRotate will rotate a nonce for a user
func NonceRotate(pCtx context.Context, pChainAddress persist.ChainAddress, nonceRepo *postgres.NonceRepository) error {
	err := nonceRepo.Create(pCtx, GenerateNonce(), pChainAddress)
	if err != nil {
		return err
	}
	return nil
}

// GetUserWithNonce returns nonce value string, user id
// will return empty strings and error if no nonce found
// will return empty string if no user found
func GetUserWithNonce(pCtx context.Context, pChainAddress persist.ChainAddress, userRepo *postgres.UserRepository, nonceRepo *postgres.NonceRepository, walletRepository *postgres.WalletRepository) (nonceValue string, user *persist.User, err error) {
	nonce, err := nonceRepo.Get(pCtx, pChainAddress)
	if err != nil {
		return nonceValue, user, err
	}

	nonceValue = nonce.Value.String()

	dbUser, err := userRepo.GetByChainAddress(pCtx, pChainAddress)
	if err != nil {
		return nonceValue, user, err
	}
	if dbUser.ID != "" {
		user = &dbUser
	} else {
		return nonceValue, user, persist.ErrUserNotFound{ChainAddress: pChainAddress}
	}

	return nonceValue, user, nil
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

func SetRolesForCtx(c *gin.Context, roles []persist.Role, err error) {
	c.Set(userRolesContextKey, roles)
	c.Set(roleErrorContextKey, err)
	c.Set(userRolesExistContextKey, len(roles) > 0 && err == nil)
}

func GetRolesFromCtx(c *gin.Context) []persist.Role {
	return c.MustGet(userRolesContextKey).([]persist.Role)
}

func GetUserRolesExistFromCtx(c *gin.Context) bool {
	return c.GetBool(userRolesExistContextKey)
}

func GetRolesErrorFromCtx(c *gin.Context) error {
	err := c.MustGet(roleErrorContextKey)

	if err != nil {
		return nil
	}

	return err.(error)
}

// SetJWTCookie sets the cookie for auth on the response
func SetJWTCookie(c *gin.Context, token string) {
	mode := http.SameSiteStrictMode
	domain := ".gallery.so"
	httpOnly := true
	secure := true

	clientIsLocalhost := c.Request.Header.Get("Origin") == "http://localhost:3000"

	if env.GetString("ENV") != "production" || clientIsLocalhost {
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

func toAuthWallets(pWallets []persist.Wallet) []AuthenticatedAddress {
	res := make([]AuthenticatedAddress, len(pWallets))
	for i, w := range pWallets {
		res[i] = AuthenticatedAddress{
			ChainAddress: persist.NewChainAddress(w.Address, w.Chain),
			WalletType:   w.WalletType,
		}
	}
	return res
}

// ContainsAuthWallet checks whether an auth.Wallet is in a slice of perist.Wallet
func ContainsWallet(pWallets []AuthenticatedAddress, pWallet AuthenticatedAddress) bool {
	for _, w := range pWallets {
		if w.ChainAddress == pWallet.ChainAddress {
			return true
		}
	}
	return false
}
