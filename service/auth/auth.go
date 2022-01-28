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
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
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
	// UserIDContextKey is the key for the user id in the context
	UserIDContextKey = "user_id"
	// AuthContextKey is the key for the auth status in the context
	AuthContextKey = "authenticated"
)

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

// ErrNoJWT is returned when there is no JWT in the request
var ErrNoJWT = errors.New("no jwt passed as cookie")

// ErrInvalidAuthHeader is returned when the auth header is invalid
var ErrInvalidAuthHeader = errors.New("invalid auth header format")

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
	SignatureValid bool            `json:"signature_valid"`
	JWTtoken       string          `json:"jwt_token"`
	UserID         persist.DBID    `json:"user_id"`
	Address        persist.Address `json:"address"`
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

type errAddressDoesNotOwnRequiredNFT struct {
	address persist.Address
}

func (e errAddressDoesNotOwnRequiredNFT) Error() string {
	return fmt.Sprintf("required tokens not owned by address: %s", e.address)
}

// ErrUserNotFound is returned when a user is not found
type ErrUserNotFound struct {
	UserID   persist.DBID
	Address  persist.Address
	Username string
}

func (e ErrUserNotFound) Error() string {
	return fmt.Sprintf("user not found: address: %s, ID: %s, Username: %s", e.Address, e.UserID, e.Username)
}

// GenerateNonce generates a random nonce to be signed by a wallet
func GenerateNonce() string {
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	nonceInt := seededRand.Int()
	nonceStr := fmt.Sprintf("%d", nonceInt)
	return nonceStr
}

// LoginAndMemorizeAttempt will run the login pipeline and memorize the result
func LoginAndMemorizeAttempt(pCtx context.Context, pInput LoginInput,
	pReq *http.Request, userRepo persist.UserRepository, nonceRepo persist.NonceRepository,
	loginRepo persist.LoginAttemptRepository, ec *ethclient.Client) (LoginOutput, error) {

	output, err := LoginPipeline(pCtx, pInput, userRepo, nonceRepo, ec)
	if err != nil {
		return LoginOutput{}, err
	}

	loginAttempt := persist.UserLoginAttempt{

		Address:        pInput.Address,
		Signature:      persist.NullString(pInput.Signature),
		SignatureValid: persist.NullBool(output.SignatureValid),

		ReqHostAddr: persist.NullString(pReq.RemoteAddr),
		ReqHeaders:  persist.ReqHeaders(pReq.Header),
	}

	_, err = loginRepo.Create(pCtx, loginAttempt)
	if err != nil {
		return LoginOutput{}, err
	}

	return output, err
}

// LoginPipeline logs in a user by validating their signed nonce
func LoginPipeline(pCtx context.Context, pInput LoginInput, userRepo persist.UserRepository,
	nonceRepo persist.NonceRepository, ec *ethclient.Client) (LoginOutput, error) {

	output := LoginOutput{}

	nonce, userID, err := GetUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if err != nil {
		return LoginOutput{}, err
	}

	if pInput.WalletType != WalletTypeEOA {
		if NewNoncePrepend+nonce != pInput.Nonce && NoncePrepend+nonce != pInput.Nonce {
			return LoginOutput{}, ErrNonceMismatch
		}
	}

	sigValid, err := VerifySignatureAllMethods(pInput.Signature,
		nonce,
		pInput.Address, pInput.WalletType, ec)
	if err != nil {
		return LoginOutput{}, err
	}

	output.SignatureValid = sigValid
	if !sigValid {
		return output, nil
	}

	output.UserID = userID

	jwtTokenStr, err := JWTGeneratePipeline(pCtx, userID)
	if err != nil {
		return LoginOutput{}, err
	}

	output.JWTtoken = jwtTokenStr

	err = NonceRotate(pCtx, pInput.Address, userID, nonceRepo)
	if err != nil {
		return LoginOutput{}, err
	}

	return output, nil
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

// GetPreflight will establish if a user is permitted to preflight a login and generate a nonce to be signed
func GetPreflight(pCtx context.Context, pInput GetPreflightInput, pPreAuthed bool,
	userRepo persist.UserRepository, nonceRepo persist.NonceRepository, ethClient *ethclient.Client) (*GetPreflightOutput, error) {

	user, err := userRepo.GetByAddress(pCtx, pInput.Address)

	logrus.WithError(err).Error("error retrieving user by address for auth preflight")

	userExistsBool := user.ID != ""

	output := &GetPreflightOutput{
		UserExists: userExistsBool,
	}
	if !userExistsBool {

		if !pPreAuthed {

			req := GetAllowlistContracts()
			has := false
			for k, v := range req {

				hasNFT, err := eth.HasNFTs(pCtx, k, v, pInput.Address, ethClient)
				if err != nil {
					return nil, err
				}
				if hasNFT {
					has = true
					break
				}
			}
			if !has {
				return nil, errAddressDoesNotOwnRequiredNFT{pInput.Address}
			}

		}

		nonce, err := nonceRepo.Get(pCtx, pInput.Address)
		if err != nil || nonce.ID == "" {
			nonce = persist.UserNonce{
				Address: pInput.Address,
				Value:   persist.NullString(GenerateNonce()),
			}

			err = nonceRepo.Create(pCtx, nonce)
			if err != nil {
				return nil, err
			}
		}

		output.Nonce = NewNoncePrepend + nonce.Value.String()

	} else {
		nonce, err := nonceRepo.Get(pCtx, pInput.Address)
		if err != nil {
			return nil, err
		}
		output.Nonce = NewNoncePrepend + nonce.Value.String()
	}

	return output, nil
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
		return nonceValue, userID, ErrUserNotFound{Address: pAddress}
	}

	return nonceValue, userID, nil
}

// GetUserIDFromCtx returns the user ID from the context
func GetUserIDFromCtx(c *gin.Context) persist.DBID {
	return c.MustGet(UserIDContextKey).(persist.DBID)
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
		MaxAge:   viper.GetInt("JWT_TTL"),
		Path:     "/",
		Secure:   secure,
		HttpOnly: httpOnly,
		SameSite: mode,
		Domain:   domain,
	})
}
