package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/mikeydub/go-gallery/service/farcaster"
	"math/rand"
	"net/http"
	"strings"
	"time"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/redis"

	"github.com/magiclabs/magic-admin-go"
	magicclient "github.com/magiclabs/magic-admin-go/client"
	"github.com/magiclabs/magic-admin-go/token"
	"github.com/mikeydub/go-gallery/env"
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
}

const (
	// Context keys for auth data
	userAuthedContextKey = "auth.authenticated"
	userIDContextKey     = "auth.user_id"
	sessionIDContextKey  = "auth.session_id"
	authErrorContextKey  = "auth.auth_error"
	userRolesContextKey  = "auth.roles"
)

// We don't want our cookies to expire, so their max age is arbitrarily set to 10 years.
// Note that browsers can still cap this expiration time (e.g. Brave limits cookies to 6 months).
const cookieMaxAge int = 60 * 60 * 24 * 365 * 10

// NoncePrepend is prepended to a nonce to make our default signing message
const NoncePrepend = "Gallery uses this cryptographic signature in place of a password: "

// AuthCookieKey is the key used to store the auth token in the cookie
const AuthCookieKey = "GLRY_JWT"

// RefreshCookieKey is the key used to store the refresh token in the cookie
const RefreshCookieKey = "GLRY_REFRESH_JWT"

// ErrNonceMismatch is returned when the nonce does not match the expected nonce
var ErrNonceMismatch = errors.New("incorrect nonce input")

// ErrMessageDoesNotContainNonce is returned when a nonce authenticator's message does not contain its nonce
var ErrMessageDoesNotContainNonce = errors.New("message does not contain nonce")

// ErrInvalidJWT is returned when the JWT is invalid
var ErrInvalidJWT = errors.New("invalid or expired auth token")

// ErrNoCookie is returned when there is no JWT in the request
var ErrNoCookie = errors.New("no jwt passed as cookie")

var ErrSessionInvalidated = errors.New("session has been invalidated")

// ErrSignatureInvalid is returned when the signed nonce's signature is invalid
var ErrSignatureInvalid = errors.New("signature invalid")

var ErrInvalidMagicLink = errors.New("invalid magic link")

// TODO: Figure out a better scheme for handling user-facing errors
var ErrEmailUnverified = errors.New("The email address you provided is unverified. Login with QR code instead, or verify your email at gallery.so/settings.")

var ErrEmailAlreadyUsed = errors.New("email already in use")

type Authenticator interface {
	// GetDescription returns information about the authenticator for error and logging purposes.
	// NOTE: GetDescription should NOT include any sensitive data (passwords, auth tokens, etc)
	// that we wouldn't want showing up in logs!
	GetDescription() string

	Authenticate(context.Context) (*AuthResult, error)
}

type AuthResult struct {
	User      *db.User
	Addresses []AuthenticatedAddress
	Email     *persist.Email
	PrivyDID  *string
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
	L1ChainAddress persist.L1ChainAddress
}

func (e ErrNonceNotFound) Error() string {
	return fmt.Sprintf("nonce not found for address: %s", e.L1ChainAddress)
}

// GenerateNonce generates a random nonce to be signed by a wallet
func GenerateNonce() (string, error) {
	nonceBytes := make([]byte, 16)
	_, err := rand.Read(nonceBytes)
	if err != nil {
		return "", err
	}
	// Encode to a hex string
	nonceStr := hex.EncodeToString(nonceBytes)
	return nonceStr, nil
}

type NonceAuthenticator struct {
	ChainPubKey        persist.ChainPubKey
	Nonce              string
	Message            string
	Signature          string
	WalletType         persist.WalletType
	EthClient          *ethclient.Client
	MultichainProvider *multichain.Provider
	Queries            *db.Queries
}

func (e NonceAuthenticator) GetDescription() string {
	return fmt.Sprintf("NonceAuthenticator(address: %s, nonce: %s, message: %s, signature: %s, walletType: %v)", e.ChainPubKey, e.Nonce, e.Message, e.Signature, e.WalletType)
}

func (e NonceAuthenticator) Authenticate(ctx context.Context) (*AuthResult, error) {
	asChainAddress := e.ChainPubKey.ToChainAddress()
	asL1 := asChainAddress.ToL1ChainAddress()

	// The message can be arbitrary, but it must contain the nonce
	if !strings.Contains(e.Message, e.Nonce) {
		return nil, ErrMessageDoesNotContainNonce
	}

	sigValid, err := e.MultichainProvider.VerifySignature(ctx, e.Signature, e.Message, e.ChainPubKey, e.WalletType)
	if err != nil {
		return nil, ErrSignatureVerificationFailed{err}
	}

	if !sigValid {
		return nil, ErrSignatureVerificationFailed{ErrSignatureInvalid}
	}

	err = ConsumeAuthNonce(ctx, e.Queries, e.Nonce)
	if err != nil {
		return nil, err
	}

	var user *db.User
	u, err := e.Queries.GetUserByAddressAndL1(ctx, db.GetUserByAddressAndL1Params{
		Address: asL1.Address(),
		L1Chain: asL1.L1Chain(),
	})

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	} else {
		user = &u
	}

	authResult := AuthResult{
		Addresses: []AuthenticatedAddress{{ChainAddress: asChainAddress, WalletType: e.WalletType}},
		User:      user,
	}

	return &authResult, nil
}

type NeynarAuthenticator struct {
	CustodyAuth    NonceAuthenticator
	PrimaryAddress *persist.ChainPubKey
	NeynarClient   *farcaster.NeynarAPI
	Queries        *db.Queries
}

func (e NeynarAuthenticator) GetDescription() string {
	return fmt.Sprintf("NeynarAuthenticator(primaryAddress: %s)", e.PrimaryAddress)
}

func (e NeynarAuthenticator) Authenticate(ctx context.Context) (*AuthResult, error) {
	nonceAuthResult, err := e.CustodyAuth.Authenticate(ctx)
	if err != nil {
		return nil, err
	}

	// At this point, we've verified ownership of the custody address. If there's no primary address,
	// we're done. Otherwise, we'll need to hit the Neynar API to see if the primary address is associated
	// with the same account as the custody address.
	if e.PrimaryAddress == nil {
		return nonceAuthResult, nil
	}

	custodyChainAddress := e.CustodyAuth.ChainPubKey.ToChainAddress()
	custodyAddress := custodyChainAddress.Address()
	primaryChainAddress := e.PrimaryAddress.ToChainAddress()
	primaryAddress := primaryChainAddress.Address()

	users, err := e.NeynarClient.UsersByAddresses(ctx, []persist.Address{custodyAddress}, true)
	if err != nil {
		return nil, err
	}

	if neynarUsers, ok := users[custodyAddress]; ok {
		for _, neynarUser := range neynarUsers {
			for _, verifiedAddress := range neynarUser.VerifiedAddresses.EthAddresses {
				if strings.EqualFold(verifiedAddress.String(), primaryAddress.String()) {
					// We've verified that the user owns the primary address
					if err := e.addPrimaryAddressToAuthResult(ctx, primaryChainAddress, nonceAuthResult); err != nil {
						return nil, err
					}

					return nonceAuthResult, nil
				}
			}
		}

		return nil, fmt.Errorf("could not find primary address (%s) in Neynar verified addresses for custody address (%s)", primaryAddress, custodyAddress)
	}

	return nil, fmt.Errorf("could not find user with custody address: %s", custodyAddress)
}

func (e NeynarAuthenticator) addPrimaryAddressToAuthResult(ctx context.Context, primaryAddress persist.ChainAddress, authResult *AuthResult) error {
	asL1 := primaryAddress.ToL1ChainAddress()

	u, err := e.Queries.GetUserByAddressAndL1(ctx, db.GetUserByAddressAndL1Params{
		Address: asL1.Address(),
		L1Chain: asL1.L1Chain(),
	})

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	} else {
		authResult.User = &u
	}

	// Put the primary address at the front of the list, since that's what gets used to set up an account
	authResult.Addresses = append([]AuthenticatedAddress{{ChainAddress: primaryAddress, WalletType: persist.WalletTypeEOA}}, authResult.Addresses...)
	return nil
}

type MagicLinkAuthenticator struct {
	Token       token.Token
	MagicClient *magicclient.API
	Queries     *db.Queries
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

	authedEmail := persist.Email(info.Email)

	authResult := AuthResult{
		Addresses: []AuthenticatedAddress{},
		Email:     &authedEmail,
	}

	user, err := e.Queries.GetUserByVerifiedEmailAddress(pCtx, authedEmail.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &authResult, persist.ErrUserNotFound{Email: authedEmail}
		}
		return &authResult, err
	}

	authResult.User = &user

	return &authResult, nil
}

func NewMagicLinkClient() *magicclient.API {
	return magicclient.New(env.GetString("MAGIC_LINK_SECRET_KEY"), magic.NewDefaultClient())
}

type OneTimeLoginTokenAuthenticator struct {
	ConsumedTokenCache *redis.Cache
	Queries            *db.Queries
	LoginToken         string
}

func (a OneTimeLoginTokenAuthenticator) GetDescription() string {
	return "OneTimeLoginTokenAuthenticator"
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

	user, err := a.Queries.GetUserById(ctx, userID)
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

	if authResult.User == nil || authResult.User.Universal {
		return "", persist.ErrUserNotFound{Authenticator: authenticator.GetDescription()}
	}

	userID := authResult.User.ID

	// Start a new session if:
	// - no user is currently authenticated, or
	// - a user is authenticated, but it's not the one who just logged in
	// Otherwise, this user is already logged in, and we don't need to do
	// anything here. Their existing session should continue as usual.
	if !GetUserAuthedFromCtx(gc) || GetUserIDFromCtx(gc) != userID {
		err = StartSession(gc, queries, userID)
		if err != nil {
			return "", err
		}
	}

	return authResult.User.ID, nil
}

func Logout(ctx context.Context, queries *db.Queries, authRefreshCache *redis.Cache) {
	gc := util.MustGetGinContext(ctx)
	EndSession(gc, queries, authRefreshCache)
}

func GenerateAuthNonce(ctx context.Context, queries *db.Queries) (nonce string, message string, err error) {
	var errs []error

	// Retry up to 4 times, though we wouldn't typically expect any failures
	for i := 0; i < 4; i++ {
		if i > 0 {
			time.Sleep(50 * time.Millisecond)
		}

		nonce, err = GenerateNonce()
		if err != nil {
			err = fmt.Errorf("error generating nonce: %w", err)
			logger.For(ctx).Error(err)
			errs = append(errs, err)
			continue
		}

		params := db.InsertNonceParams{
			ID:    persist.GenerateID(),
			Value: nonce,
		}

		_, err = queries.InsertNonce(ctx, params)
		if err != nil {
			// The query returns pgx.ErrNoRows if the nonce already exists
			if errors.Is(err, pgx.ErrNoRows) {
				err = fmt.Errorf("nonce value %s already exists", nonce)
			}
			err = fmt.Errorf("error inserting nonce: %w", err)
			logger.For(ctx).Error(err)
			errs = append(errs, err)
			continue
		}

		return nonce, NoncePrepend + nonce, nil
	}

	return "", "", util.MultiErr(errs)
}

func ConsumeAuthNonce(ctx context.Context, queries *db.Queries, nonce string) error {
	_, err := queries.ConsumeNonce(ctx, nonce)

	// The query returns pgx.ErrNoRows if a nonce with this value could not be found or consumed
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		err = errors.New("nonce does not exist or is expired")
	}

	return err
}

// GetSessionIDFromCtx returns the session ID from the context
func GetSessionIDFromCtx(c *gin.Context) persist.DBID {
	return c.MustGet(sessionIDContextKey).(persist.DBID)
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

func GetRolesFromCtx(c *gin.Context) []persist.Role {
	return c.MustGet(userRolesContextKey).([]persist.Role)
}

func setSessionStateForCtx(c *gin.Context, userID persist.DBID, sessionID persist.DBID, roles []persist.Role) {
	if userID == "" || sessionID == "" {
		logger.For(c).Errorf("attempted to set session state with missing values. userID: %s, sessionID: %s", userID, sessionID)
		err := errors.New("attempted to set session state with missing values")
		// We should never be trying to set a session with an empty userID or sessionID. If we find
		// ourselves here, clear the session and have the user log in again.
		clearSessionStateForCtx(c, err)
		clearSessionCookies(c)
		return
	}

	if roles == nil {
		roles = []persist.Role{}
	}

	c.Set(userIDContextKey, userID)
	c.Set(sessionIDContextKey, sessionID)
	c.Set(authErrorContextKey, nil)
	c.Set(userAuthedContextKey, true)
	c.Set(userRolesContextKey, roles)
}

func clearSessionStateForCtx(c *gin.Context, err error) {
	c.Set(userIDContextKey, "")
	c.Set(sessionIDContextKey, "")
	c.Set(authErrorContextKey, err)
	c.Set(userAuthedContextKey, false)
	c.Set(userRolesContextKey, []persist.Role{})
}

// ForceAuthTokenRefresh should be called whenever something happens that would result in existing auth
// tokens being out-of-date. For example, when a user's roles are changed, or a user logs out of a session,
// existing otherwise-valid auth tokens should be refreshed so they have the latest session state.
func ForceAuthTokenRefresh(ctx context.Context, authRefreshCache *redis.Cache, userID persist.DBID) error {
	// Keep the key long enough for any existing auth tokens to expire, plus an extra minute of wiggle room
	expiration := time.Duration(env.GetInt64("AUTH_JWT_TTL"))*time.Second + time.Minute
	return authRefreshCache.SetTime(ctx, userID.String(), time.Now(), expiration, true)
}

func mustRefreshAuthToken(ctx context.Context, authRefreshCache *redis.Cache, userID persist.DBID, issuedAt time.Time) bool {
	forceRefreshBefore, err := authRefreshCache.GetTime(ctx, userID.String())
	if err != nil {
		// If there's no key for this user, we don't need to force an auth token refresh
		var notFound redis.ErrKeyNotFound
		if errors.As(err, &notFound) {
			return false
		}

		// If we couldn't hit the redis cache, assume we need to refresh the auth token
		logger.For(ctx).Errorf("error checking auth refresh cache: %s", err)
		return true
	}

	return issuedAt.Before(forceRefreshBefore)
}

// StartSession begins a new session for the specified user. After calling StartSession,
// the current auth state can be queried with functions like GetUserAuthedFromCtx(),
// GetUserIDFromCtx(), etc.
func StartSession(c *gin.Context, queries *db.Queries, userID persist.DBID) error {
	sessionID := persist.GenerateID()

	// These are the first tokens for a new session, so parentRefreshID is an empty string
	err := issueSessionTokens(c, userID, sessionID, "", queries)
	if err != nil {
		// If we fail to issue tokens to start a new session, the user will need to log in
		// again (since we were starting a new session and the user doesn't have a valid refresh
		// token to present during their next request). Clear the session state and cookies.
		clearSessionStateForCtx(c, err)
		clearSessionCookies(c)
		return err
	}

	return nil
}

// ContinueSession checks the request cookies for an existing auth session and continues
// it if possible. If the request is for an expired or invalid session, the user will be
// logged out. After calling ContinueSession, the current auth state can be queried with
// functions like GetUserAuthedFromCtx(), GetUserIDFromCtx(), etc.
func ContinueSession(c *gin.Context, queries *db.Queries, authRefreshCache *redis.Cache) error {
	// If the user has a valid auth cookie, we can set their auth state and be done
	// (unless something like updating roles triggered a forced refresh of the auth token)
	authClaims, authTokenErr := getAndParseAuthToken(c)
	if authTokenErr == nil {
		// ----------------------------------------------------------------------------
		// Temporary handling for existing auth tokens that don't have session IDs.
		// Where it would normally be an error for an auth token to not have a session ID,
		// it's expected for tokens that were issued prior to the introduction of session IDs.
		// Can be removed in a month when all existing auth tokens will have expired.
		// This also applies to IssuedAt times, which are required now, but weren't present
		// in older tokens.
		if authClaims.SessionID == "" || authClaims.IssuedAt == nil {
			return StartSession(c, queries, authClaims.UserID)
		}
		// End of temporary handling
		// ----------------------------------------------------------------------------
		if !mustRefreshAuthToken(c, authRefreshCache, authClaims.UserID, authClaims.IssuedAt.Time) {
			setSessionStateForCtx(c, authClaims.UserID, authClaims.SessionID, authClaims.Roles)
			return nil
		}
	}

	// If the user doesn't have a valid auth cookie or a valid refresh cookie, they can't be
	// authenticated and they'll have to log in again. Clear their cookies in case they have
	// expired tokens.
	refreshClaims, refreshTokenErr := getAndParseRefreshToken(c)
	if refreshTokenErr != nil {
		clearSessionStateForCtx(c, refreshTokenErr)

		// The most common case here is that the user has no cookies at all, which is fine and expected.
		// If we encounter any other errors, log them and clear the user's cookies.
		if authTokenErr != ErrNoCookie || refreshTokenErr != ErrNoCookie {
			logger.For(c).Warnf("could not continue session: authTokenErr=%s, refreshTokenErr=%s", authTokenErr, refreshTokenErr)
			clearSessionCookies(c)
		}

		return refreshTokenErr
	}

	// At this point, the user has a valid refresh cookie, but the auth cookie needs to be reissued
	// (either because it's invalid or because it's being forced to refresh). Issue new tokens to continue
	// the existing session.
	err := issueSessionTokens(c, refreshClaims.UserID, refreshClaims.SessionID, refreshClaims.ID, queries)
	if err != nil {
		logger.For(c).Errorf("error issuing session tokens (userID=%s, sessionID=%s): %s", refreshClaims.UserID, refreshClaims.SessionID, err)

		// If we encountered an error issuing tokens, clear the context state so the user is "unauthenticated"
		// for the duration of this request.
		clearSessionStateForCtx(c, err)

		// Under most circumstances, we don't want to clear the user's cookies here to log them out.
		// They still have a valid refresh cookie that can authenticate their next request, and that
		// will be lower friction than being forced to log in again. The only exception is
		// ErrSessionInvalidated, which indicates that the session associated with the refresh token
		// is no longer available and the user will need to log in again.
		if err == ErrSessionInvalidated {
			clearSessionCookies(c)
		}

		return err
	}

	return nil
}

// EndSession invalidates the current session and clears the user's cookies
func EndSession(c *gin.Context, queries *db.Queries, authRefreshCache *redis.Cache) {
	if GetUserAuthedFromCtx(c) {
		if sessionID := GetSessionIDFromCtx(c); sessionID != "" {
			if err := queries.InvalidateSession(c, sessionID); err != nil {
				logger.For(c).Errorf("failed to invalidate session: %s", err)
			}
		}
		userID := GetUserIDFromCtx(c)
		if err := ForceAuthTokenRefresh(c, authRefreshCache, userID); err != nil {
			logger.For(c).Errorf("failed to force auth token refresh when ending session: %s", err)
		}
	}

	clearSessionStateForCtx(c, ErrNoCookie)
	clearSessionCookies(c)
}

// issueSessionTokens creates new tokens, updates the session in the database, and then sets the new
// tokens as request cookies and context state. parentRefreshID is the ID of the refresh token used to
// issue the new tokens; if this is the first set of tokens for a session, it should be an empty string.
// If an error occurs when issuing new tokens, no changes are made to cookies or context.
func issueSessionTokens(c *gin.Context, userID persist.DBID, sessionID persist.DBID, parentRefreshID string, queries *db.Queries) error {
	newRefreshID := persist.GenerateID().String()
	newRefreshToken, refreshExpiresAt, err := GenerateRefreshToken(c, newRefreshID, parentRefreshID, userID, sessionID)
	if err != nil {
		logger.For(c).Errorf("error generating refresh token for userID=%s, sessionID=%s: %s", userID, sessionID, err)
		return err
	}

	roles, err := RolesByUserID(c, queries, userID)
	if err != nil {
		logger.For(c).Errorf("error getting roles for userID=%s, sessionID=%s: %s", userID, sessionID, err)
		return err
	}

	newAuthToken, err := GenerateAuthToken(c, userID, sessionID, parentRefreshID, roles)
	if err != nil {
		logger.For(c).Errorf("error generating auth token for userID=%s, sessionID=%s: %s", userID, sessionID, err)
		return err
	}

	session, err := queries.UpsertSession(c, db.UpsertSessionParams{
		ID:               sessionID,
		UserID:           userID,
		UserAgent:        c.GetHeader("User-Agent"),
		Platform:         c.GetHeader("X-Platform"),
		Os:               c.GetHeader("X-OS"),
		CurrentRefreshID: newRefreshID,
		ActiveUntil:      refreshExpiresAt,
	})

	if err != nil {
		logger.For(c).Errorf("error upserting session data for userID=%s, sessionID=%s: %s", userID, sessionID, err)
		return err
	}

	if session.Invalidated {
		return ErrSessionInvalidated
	}

	setSessionStateForCtx(c, userID, sessionID, roles)
	setSessionCookies(c, newAuthToken, newRefreshToken)

	return nil
}

func setSessionCookies(c *gin.Context, authToken string, refreshToken string) {
	setCookie(c, AuthCookieKey, authToken)
	setCookie(c, RefreshCookieKey, refreshToken)
}

func clearSessionCookies(c *gin.Context) {
	clearCookie(c, AuthCookieKey)
	clearCookie(c, RefreshCookieKey)
}

func getAndParseAuthToken(c *gin.Context) (AuthTokenClaims, error) {
	authToken, err := getCookie(c, AuthCookieKey)
	if err != nil {
		return AuthTokenClaims{}, err
	}

	return ParseAuthToken(c, authToken)
}

func getAndParseRefreshToken(c *gin.Context) (RefreshTokenClaims, error) {
	refreshToken, err := getCookie(c, RefreshCookieKey)
	if err != nil {
		return RefreshTokenClaims{}, err
	}

	return ParseRefreshToken(c, refreshToken)
}

func getCookie(c *gin.Context, cookieName string) (string, error) {
	cookie, err := c.Cookie(cookieName)

	// Treat empty cookies the same way we treat missing cookies, since setting a cookie to the empty
	// string is how we "delete" them.
	if (err == nil && cookie == "") || err == http.ErrNoCookie {
		err = ErrNoCookie
	}

	if err != nil {
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

	if env.GetString("ENV") == "local" {
		userAgent := c.GetHeader("User-Agent")

		// WebKit-based clients (e.g. Safari and our mobile app) won't set a secure cookie unless the
		// request uses HTTPS, but local development doesn't use HTTPS, so we need to disable secure
		// cookies for local environments when receiving requests from these platforms.

		// Mobile app
		if strings.Contains(userAgent, "GalleryLabs") && strings.Contains(userAgent, "Darwin") {
			secure = false
			logger.For(c).Info("Request is from mobile app, setting local auth cookie with secure=false")
		}

		// Safari mentions "Safari" in its User-Agent string, but it doesn't mention Chrome or Chromium.
		if strings.Contains(userAgent, "Safari") && !strings.Contains(userAgent, "Chrome") && !strings.Contains(userAgent, "Chromium") {
			secure = false
			logger.For(c).Info("Request is from Safari, setting local auth cookie with secure=false")
		}
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
