package auth

import (
	"context"
	"errors"
	"fmt"
	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/redis"
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
	userAuthedContextKey = "auth.authenticated"
	userIDContextKey     = "auth.user_id"
	sessionIDContextKey  = "auth.session_id"
	authErrorContextKey  = "auth.auth_error"
)

// We don't want our cookies to expire, so their max age is arbitrarily set to 10 years.
// Note that browsers can still cap this expiration time (e.g. Brave limits cookies to 6 months).
const cookieMaxAge int = 60 * 60 * 24 * 365 * 10

// NoncePrepend is what is prepended to every nonce
const NoncePrepend = "Gallery uses this cryptographic signature in place of a password, verifying that you are the owner of this Ethereum address: "

// NewNoncePrepend is what will now be prepended to every nonce
const NewNoncePrepend = "Gallery uses this cryptographic signature in place of a password: "

// AuthCookieKey is the key used to store the auth token in the cookie
const AuthCookieKey = "GLRY_JWT"

// RefreshCookieKey is the key used to store the refresh token in the cookie
const RefreshCookieKey = "GLRY_REFRESH_JWT"

// ErrAddressSignatureMismatch is returned when the address signature does not match the address cryptographically
var ErrAddressSignatureMismatch = errors.New("address does not match signature")

// ErrNonceMismatch is returned when the nonce does not match the expected nonce
var ErrNonceMismatch = errors.New("incorrect nonce input")

// ErrInvalidJWT is returned when the JWT is invalid
var ErrInvalidJWT = errors.New("invalid or expired auth token")

// ErrNoCookie is returned when there is no JWT in the request
var ErrNoCookie = errors.New("no jwt passed as cookie")

// ErrSignatureInvalid is returned when the signed nonce's signature is invalid
var ErrSignatureInvalid = errors.New("signature invalid")

var ErrInvalidMagicLink = errors.New("invalid magic link")

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
		return nil, err
	}

	return &AuthResult{
		User:      &user,
		Addresses: []AuthenticatedAddress{},
	}, nil
}

func NewMagicLinkClient() *magicclient.API {
	return magicclient.New(env.GetString("MAGIC_LINK_SECRET_KEY"), magic.NewDefaultClient())
}

type OneTimeLoginTokenAuthenticator struct {
	ConsumedTokenCache *redis.Cache
	UserRepo           *postgres.UserRepository
	LoginToken         string
}

func (a OneTimeLoginTokenAuthenticator) GetDescription() string {
	return fmt.Sprintf("OneTimeLoginTokenAuthenticator")
}

func (a OneTimeLoginTokenAuthenticator) Authenticate(ctx context.Context) (*AuthResult, error) {
	userID, expiresAt, err := ParseOneTimeLoginToken(ctx, a.LoginToken)
	if err != nil {
		return nil, err
	}

	// Use redis to stop this token from being used again (and add an extra minute to the TTL account for clock differences)
	ttl := time.Until(expiresAt) + time.Minute
	success, err := a.ConsumedTokenCache.SetNX(ctx, a.LoginToken, []byte{1}, ttl)
	if err != nil {
		return nil, err
	}

	if !success {
		return nil, errors.New("token already used")
	}

	user, err := a.UserRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	authResult := AuthResult{
		Addresses: []AuthenticatedAddress{},
		User:      &user,
	}

	return &authResult, nil
}

// Login logs in a user with a given authentication scheme
func Login(ctx context.Context, queries *db.Queries, authenticator Authenticator) (persist.DBID, error) {
	gc := util.MustGetGinContext(ctx)

	authResult, err := authenticator.Authenticate(ctx)
	if err != nil {
		return "", ErrAuthenticationFailed{WrappedErr: err}
	}

	if authResult.User == nil || authResult.User.Universal.Bool() {
		return "", persist.ErrUserNotFound{Authenticator: authenticator.GetDescription()}
	}

	userID := authResult.User.ID

	err = StartSession(gc, queries, userID)
	if err != nil {
		return "", err
	}

	return authResult.User.ID, nil
}

func Logout(ctx context.Context) {
	gc := util.MustGetGinContext(ctx)
	EndSession(gc)
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

// GetSessionIDFromCtx returns the session ID from the context
func GetSessionIDFromCtx(c *gin.Context) string {
	return c.MustGet(sessionIDContextKey).(string)
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

func setSessionStateForCtx(c *gin.Context, userID persist.DBID, sessionID string) {
	if userID == "" || sessionID == "" {
		logger.For(c).Errorf("attempted to set session state with missing values. userID: %s, sessionID: %s", userID, sessionID)
		err := errors.New("attempted to set session state with missing values")
		clearSessionStateForCtx(c, err)
		return
	}

	c.Set(userIDContextKey, userID)
	c.Set(sessionIDContextKey, sessionID)
	c.Set(authErrorContextKey, nil)
	c.Set(userAuthedContextKey, true)
}

func clearSessionStateForCtx(c *gin.Context, err error) {
	c.Set(userIDContextKey, "")
	c.Set(sessionIDContextKey, "")
	c.Set(authErrorContextKey, err)
	c.Set(userAuthedContextKey, false)
}

func StartSession(c *gin.Context, queries *db.Queries, userID persist.DBID) error {
	sessionID := persist.GenerateID().String()

	err := issueSessionTokens(c, userID, sessionID, queries)
	if err != nil {
		return err
	}

	return nil
}

func ContinueSession(c *gin.Context, queries *db.Queries) error {
	// If the user has a valid auth cookie, we can set their auth state and be done
	userID, sessionID, err := getAuthToken(c)
	if err == nil {
		// ----------------------------------------------------------------------------
		// Temporary handling for existing auth tokens that don't have session IDs.
		// Where it would normally be an error for an auth token to not have a session ID,
		// it's expected for tokens that were issued prior to the introduction of session IDs.
		// Can be removed in a month when all existing auth tokens will have expired.
		if sessionID == "" {
			sessionID = persist.GenerateID().String()
			err = issueSessionTokens(c, userID, sessionID, queries)
			if err != nil {
				return err
			}

			return nil
		}
		// End of temporary handling
		// ----------------------------------------------------------------------------
		logger.For(c).Info("found valid auth cookie")
		setSessionStateForCtx(c, userID, sessionID)
		return nil
	}

	// If the user doesn't have a valid auth cookie or a valid refresh cookie, they can't be
	// authenticated and they'll have to log in again.
	userID, sessionID, err = getRefreshToken(c)
	if err != nil {
		logger.For(c).Info("no valid auth or refresh cookies found")
		clearSessionStateForCtx(c, err)
		return err
	}

	// At this point, the user has a valid refresh cookie, but not a valid auth cookie.
	// Give them new cookies to continue their existing session.
	err = issueSessionTokens(c, userID, sessionID, queries)
	if err != nil {
		return err
	}

	return nil
}

// EndSession clears the state and cookies for the current session. Note that it does not
// actually invalidate the session, so the session could still be continued in the future
// if a client continued to use the tokens. We may add this functionality in the future, though;
// all it would take is a database table of ended session IDs and a cron job to delete entries
// from the table after the expiration date of the session's last issued refresh token.
func EndSession(c *gin.Context) {
	clearCookie(c, RefreshCookieKey)
	clearCookie(c, AuthCookieKey)
	clearSessionStateForCtx(c, ErrNoCookie)
}

func issueSessionTokens(c *gin.Context, userID persist.DBID, sessionID string, queries *db.Queries) error {
	newRefreshToken, err := GenerateRefreshToken(c, userID, sessionID)
	if err != nil {
		clearSessionStateForCtx(c, err)
		return err
	}

	newAuthToken, err := GenerateAuthToken(c, userID, sessionID)
	if err != nil {
		clearSessionStateForCtx(c, err)
		return err
	}

	// TODO: Update the sessions table here

	setSessionStateForCtx(c, userID, sessionID)
	setCookie(c, RefreshCookieKey, newRefreshToken)
	setCookie(c, AuthCookieKey, newAuthToken)

	return nil
}

func getAuthToken(c *gin.Context) (persist.DBID, string, error) {
	authToken, err := getCookie(c, AuthCookieKey)
	if err != nil {
		return "", "", err
	}

	userID, sessionID, err := ParseAuthToken(c, authToken)
	if err != nil {
		return "", "", err
	}

	return userID, sessionID, nil
}

func getRefreshToken(c *gin.Context) (persist.DBID, string, error) {
	refreshToken, err := getCookie(c, RefreshCookieKey)
	if err != nil {
		return "", "", err
	}

	userID, sessionID, err := ParseRefreshToken(c, refreshToken)
	if err != nil {
		return "", "", err
	}

	return userID, sessionID, nil
}

func getCookie(c *gin.Context, cookieName string) (string, error) {
	cookie, err := c.Cookie(cookieName)

	// Treat empty cookies the same way we treat missing cookies, since setting a cookie to the empty
	// string is how we "delete" them.
	if err == nil && cookie == "" {
		err = http.ErrNoCookie
	}

	if err != nil {
		if err == http.ErrNoCookie {
			err = ErrNoCookie
		}

		return "", err
	}

	return cookie, nil
}

func setCookie(c *gin.Context, cookieName string, value string) {
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
		Name:     cookieName,
		Value:    value,
		MaxAge:   cookieMaxAge,
		Path:     "/",
		Secure:   secure,
		HttpOnly: httpOnly,
		SameSite: mode,
		Domain:   domain,
	})
}

func clearCookie(c *gin.Context, cookieName string) {
	setCookie(c, cookieName, "")
}
