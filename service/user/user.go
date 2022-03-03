package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mikeydub/go-gallery/graphql/model"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/indexer"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/pubsub"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var errUserCannotRemoveAllAddresses = errors.New("user does not have enough addresses to remove")
var errMustResolveENS = errors.New("ENS username must resolve to owner address")

// UpdateUserInput is the input for the user update pipeline
type UpdateUserInput struct {
	UserName string `json:"username" binding:"username"`
	BioStr   string `json:"bio" binding:"medium"`
}

// GetUserInput is the input for the user get pipeline
type GetUserInput struct {
	UserID   persist.DBID    `json:"user_id" form:"user_id"`
	Address  persist.Address `json:"address" form:"address" binding:"eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	Username string          `json:"username" form:"username"`
}

// GetUserOutput is the output of the user get pipeline
type GetUserOutput struct {
	UserID    persist.DBID         `json:"id"`
	Username  string               `json:"username"`
	BioStr    string               `json:"bio"`
	Addresses []persist.Address    `json:"addresses"`
	CreatedAt persist.CreationTime `json:"created_at"`
}

// AddUserAddressesInput is the input for the user add addresses pipeline and also user creation pipeline given that they have the same requirements
type AddUserAddressesInput struct {

	// needed because this is a new user that cant be logged into, and the client creating
	// the user still needs to prove ownership of their address.
	Signature  string          `json:"signature" binding:"signature"`
	Nonce      string          `json:"nonce"`
	Address    persist.Address `json:"address"   binding:"required,eth_addr"` // len=42"` // standard ETH "0x"-prefixed address
	WalletType auth.WalletType `json:"wallet_type"`
}

// AddUserAddressOutput is the output of the user add address pipeline
type AddUserAddressOutput struct {
	SignatureValid bool `json:"signature_valid"`
}

// RemoveUserAddressesInput is the input for the user remove addresses pipeline
type RemoveUserAddressesInput struct {
	Addresses []persist.Address `json:"addresses"   binding:"required"`
}

// CreateUserOutput is the output of the user create pipeline
type CreateUserOutput struct {
	SignatureValid bool         `json:"signature_valid"`
	JWTtoken       string       `json:"jwt_token"` // JWT token is sent back to user to use to continue onboarding
	UserID         persist.DBID `json:"user_id"`
	GalleryID      persist.DBID `json:"gallery_id"`
}

// AddAddressPubSubInput is the input for the user add address pubsub pipeline
type AddAddressPubSubInput struct {
	UserID  persist.DBID    `json:"user_id"`
	Address persist.Address `json:"address"`
}

// MergeUsersInput is the input for the user merge pipeline
type MergeUsersInput struct {
	SecondUserID persist.DBID    `json:"second_user_id" binding:"required"`
	Signature    string          `json:"signature" binding:"signature"`
	Nonce        string          `json:"nonce"`
	Address      persist.Address `json:"address"   binding:"required,eth_addr"`
	WalletType   auth.WalletType `json:"wallet_type"`
}

// CreateUserToken creates a JWT token for the user
func CreateUserToken(pCtx context.Context, pInput AddUserAddressesInput, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, galleryRepo persist.GalleryTokenRepository, ethClient *ethclient.Client, psub pubsub.PubSub) (CreateUserOutput, error) {

	output := &CreateUserOutput{}

	nonce, id, _ := auth.GetUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if nonce == "" {
		return CreateUserOutput{}, auth.ErrNonceNotFound{Address: pInput.Address}
	}
	if id != "" {
		return CreateUserOutput{}, ErrUserAlreadyExists{Address: pInput.Address}
	}

	if pInput.WalletType != auth.WalletTypeEOA {
		if auth.NewNoncePrepend+nonce != pInput.Nonce && auth.NoncePrepend+nonce != pInput.Nonce {
			return CreateUserOutput{}, auth.ErrNonceMismatch
		}
	}

	sigValidBool, err := auth.VerifySignatureAllMethods(pInput.Signature,
		nonce,
		pInput.Address, pInput.WalletType, ethClient)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output.SignatureValid = sigValidBool
	if !sigValidBool {
		return *output, nil
	}

	user := persist.User{
		Addresses: []persist.Address{pInput.Address},
	}

	userID, err := userRepo.Create(pCtx, user)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output.UserID = userID

	defer func() {
		// user has been created successfully, shoot out a pubsub to notify any service that needs it as well as
		// validate that the user's NFTs are valid and have cached media content
		if viper.GetString("ENV") != "local" {
			go func() {
				err := publishUserSignup(pCtx, output.UserID, userRepo, psub)
				if err != nil {
					logrus.WithError(err).Error("failed to publish user signup")
				}
			}()
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := validateNFTsForUser(ctx, userID)
			if err != nil {
				logrus.WithError(err).Error("validateNFTsForUser")
			}
		}()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := ensureMediaContent(ctx, pInput.Address)
			if err != nil {
				logrus.WithError(err).Error("ensureMediaForUser")
			}
		}()
	}()

	jwtTokenStr, err := auth.JWTGeneratePipeline(pCtx, userID)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output.JWTtoken = jwtTokenStr

	err = auth.NonceRotate(pCtx, pInput.Address, userID, nonceRepo)
	if err != nil {
		return CreateUserOutput{}, err
	}

	galleryInsert := persist.GalleryTokenDB{OwnerUserID: userID, Collections: []persist.DBID{}}

	galleryID, err := galleryRepo.Create(pCtx, galleryInsert)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output.GalleryID = galleryID

	return *output, nil
}

// CreateUser creates a new user
func CreateUser(pCtx context.Context, authenticator auth.Authenticator, userRepo persist.UserRepository,
	galleryRepo persist.GalleryRepository) (*model.CreateUserPayload, error) {
	gc := util.GinContextFromContext(pCtx)

	authResult, err := authenticator.Authenticate(pCtx)
	if err != nil {
		return nil, auth.ErrAuthenticationFailed{WrappedErr: err}
	}

	if authResult.UserID != "" {
		return nil, ErrUserAlreadyExists{Authenticator: authenticator.GetDescription()}
	}

	// TODO: This currently takes the first authenticated address returned by the authenticator and creates
	// the user's account based on that address. This works because the only auth mechanism we have is nonce-based
	// auth and that supplies a single address. In the future, a user may authenticate in a way that makes
	// multiple authenticated addresses available for initial user creation, and we may want to add all of
	// those addresses to the user's account here.
	address := authResult.Addresses[0]

	user := persist.User{
		Addresses: []persist.Address{address},
	}

	userID, err := userRepo.Create(pCtx, user)
	if err != nil {
		return nil, err
	}

	defer func() {
		// user has been created successfully, validate that the user's NFTs are valid and have cached media content
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := validateNFTsForUser(ctx, userID)
			if err != nil {
				logrus.WithError(err).Error("validateNFTsForUser")
			}
		}()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := ensureMediaContent(ctx, address)
			if err != nil {
				logrus.WithError(err).Error("ensureMediaForUser")
			}
		}()
	}()

	jwtTokenStr, err := auth.JWTGeneratePipeline(pCtx, userID)
	if err != nil {
		return nil, err
	}

	galleryInsert := persist.GalleryDB{OwnerUserID: userID, Collections: []persist.DBID{}}

	galleryID, err := galleryRepo.Create(pCtx, galleryInsert)
	if err != nil {
		return nil, err
	}

	auth.SetJWTCookie(gc, jwtTokenStr)

	output := model.CreateUserPayload{
		UserID:    &userID,
		GalleryID: &galleryID,
	}

	return &output, nil
}

// CreateUserREST creates a new user
func CreateUserREST(pCtx context.Context, pInput AddUserAddressesInput, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, galleryRepo persist.GalleryRepository, ethClient *ethclient.Client) (CreateUserOutput, error) {

	authenticator := auth.EthereumNonceAuthenticator{
		Address:    pInput.Address,
		Nonce:      pInput.Nonce,
		Signature:  pInput.Signature,
		WalletType: pInput.WalletType,
		UserRepo:   userRepo,
		NonceRepo:  nonceRepo,
		EthClient:  ethClient,
	}

	gqlOutput, err := CreateUser(pCtx, authenticator, userRepo, galleryRepo)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output := CreateUserOutput{
		SignatureValid: true,
		UserID:         *gqlOutput.UserID,
		GalleryID:      *gqlOutput.GalleryID,
	}

	return output, nil
}

// RemoveAddressesFromUser removes any amount of addresses from a user in the DB
func RemoveAddressesFromUser(pCtx context.Context, pUserID persist.DBID, pAddresses []persist.Address,
	userRepo persist.UserRepository) error {

	user, err := userRepo.GetByID(pCtx, pUserID)
	if err != nil {
		return err
	}

	if len(user.Addresses) <= len(pAddresses) {
		return errUserCannotRemoveAllAddresses
	}

	return userRepo.RemoveAddresses(pCtx, pUserID, pAddresses)
}

// AddAddressToUser adds a single address to a user in the DB because a signature needs to be provided and validated per address
func AddAddressToUser(pCtx context.Context, pUserID persist.DBID, pAddress persist.Address, addressAuth auth.Authenticator,
	userRepo persist.UserRepository, psub pubsub.PubSub) error {

	authResult, err := addressAuth.Authenticate(pCtx)
	if err != nil {
		return err
	}

	addressUserID := authResult.UserID

	if addressUserID != "" {
		return ErrAddressOwnedByUser{Address: pAddress, OwnerID: addressUserID}
	}

	if !containsAddress(authResult.Addresses, pAddress) {
		return ErrAddressNotOwnedByUser{Address: pAddress, UserID: addressUserID}
	}

	defer func() {
		// user has successfully added an address, shoot out a pubsub to notify any service that needs it as well as
		// validate that the user's NFTs are valid and have cached media content
		if viper.GetString("ENV") != "local" {
			go func() {
				err := publishUserAddAddress(pCtx, pUserID, pAddress, psub)
				if err != nil {
					logrus.WithError(err).Error("failed to publish user signup")
				}
			}()
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := validateNFTsForUser(ctx, pUserID)
			if err != nil {
				logrus.WithError(err).Error("validateNFTsForUser")
			}
		}()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := ensureMediaContent(ctx, pAddress)
			if err != nil {
				logrus.WithError(err).Error("ensureMediaForUser")
			}
		}()
	}()

	if err = userRepo.AddAddresses(pCtx, pUserID, []persist.Address{pAddress}); err != nil {
		return err
	}

	return nil
}

// RemoveAddressesFromUserToken removes any amount of addresses from a user in the DB
func RemoveAddressesFromUserToken(pCtx context.Context, pUserID persist.DBID, pInput RemoveUserAddressesInput,
	userRepo persist.UserRepository) error {

	user, err := userRepo.GetByID(pCtx, pUserID)
	if err != nil {
		return err
	}

	if len(user.Addresses) <= len(pInput.Addresses) {
		return errUserCannotRemoveAllAddresses
	}

	return userRepo.RemoveAddresses(pCtx, pUserID, pInput.Addresses)

}

// GetUser returns a user by ID or address or username
func GetUser(pCtx context.Context, pInput GetUserInput, userRepo persist.UserRepository) (GetUserOutput, error) {

	//------------------

	var user persist.User
	var err error
	switch {
	case pInput.UserID != "":
		user, err = userRepo.GetByID(pCtx, pInput.UserID)
		if err != nil {
			return GetUserOutput{}, err
		}
		break
	case pInput.Username != "":
		user, err = userRepo.GetByUsername(pCtx, pInput.Username)
		if err != nil {
			return GetUserOutput{}, err
		}
		break
	case pInput.Address != "":
		user, err = userRepo.GetByAddress(pCtx, pInput.Address)
		if err != nil {
			return GetUserOutput{}, err
		}
		break
	}

	if user.ID == "" {
		return GetUserOutput{}, persist.ErrUserNotFound{UserID: pInput.UserID, Address: pInput.Address, Username: pInput.Username}
	}

	output := GetUserOutput{
		UserID:    user.ID,
		Username:  user.Username.String(),
		BioStr:    user.Bio.String(),
		CreatedAt: user.CreationTime,
		Addresses: user.Addresses,
	}

	return output, nil
}

// UpdateUser updates a user by ID and ensures that if they are using an ENS name as a username that their address resolves to that ENS
func UpdateUser(pCtx context.Context, userID persist.DBID, username string, bio string, userRepository persist.UserRepository, ethClient *ethclient.Client) error {
	if strings.HasSuffix(strings.ToLower(username), ".eth") {
		user, err := userRepository.GetByID(pCtx, userID)
		if err != nil {
			return err
		}
		can := false
		for _, addr := range user.Addresses {
			if resolves, _ := eth.ResolvesENS(pCtx, username, addr, ethClient); resolves {
				can = true
				break
			}
		}
		if !can {
			return errMustResolveENS
		}
	}

	err := userRepository.UpdateByID(
		pCtx,
		userID,
		persist.UserUpdateInfoInput{
			UsernameIdempotent: persist.NullString(strings.ToLower(username)),
			Username:           persist.NullString(username),
			Bio:                persist.NullString(validate.SanitizationPolicy.Sanitize(bio)),
		},
	)
	if err != nil {
		return err
	}
	return nil
}

// MergeUsers merges two users together
func MergeUsers(pCtx context.Context, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, pUserID persist.DBID, pInput MergeUsersInput, ethClient *ethclient.Client) error {
	nonce, id, _ := auth.GetUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if nonce == "" {
		return auth.ErrNonceNotFound{Address: pInput.Address}
	}
	if id != pInput.SecondUserID {
		return fmt.Errorf("wrong nonce: user %s is not the second user", pInput.SecondUserID)
	}

	if pInput.WalletType != auth.WalletTypeEOA {
		if auth.NewNoncePrepend+nonce != pInput.Nonce && auth.NoncePrepend+nonce != pInput.Nonce {
			return auth.ErrNonceMismatch
		}
	}

	sigValidBool, err := auth.VerifySignatureAllMethods(pInput.Signature,
		nonce,
		pInput.Address, pInput.WalletType, ethClient)
	if err != nil {
		return err
	}

	if !sigValidBool {
		return fmt.Errorf("signature is invalid for address %s", pInput.Address)
	}

	return userRepo.MergeUsers(pCtx, pUserID, pInput.SecondUserID)

}

func validateNFTsForUser(pCtx context.Context, pUserID persist.DBID) error {
	endpoint := viper.GetString("INDEXER_HOST") + "/nfts/validate"
	input := indexer.ValidateUsersNFTsInput{
		UserID: pUserID,
	}
	client := &http.Client{}
	deadline, ok := pCtx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second * 10)
	}
	client.Timeout = time.Until(deadline)

	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	output := indexer.ValidateUsersNFTsOutput{}
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return err
	}

	if !output.Success {
		return errors.New(output.Message)
	}

	return nil
}

func ensureMediaContent(pCtx context.Context, pAddress persist.Address) error {
	endpoint := viper.GetString("INDEXER_HOST") + "/media/update"
	input := indexer.UpdateMediaInput{
		OwnerAddress: pAddress,
	}
	client := &http.Client{}
	deadline, ok := pCtx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second * 10)
	}
	client.Timeout = time.Until(deadline)

	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	output := util.SuccessResponse{}
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return err
	}

	if !output.Success {
		return errCouldNotEnsureMediaForAddress{address: pAddress}
	}

	return nil
}

func publishUserSignup(pCtx context.Context, pUserID persist.DBID, userRepository persist.UserRepository, psub pubsub.PubSub) error {
	user, err := userRepository.GetByID(pCtx, pUserID)
	if err == nil {
		asJSON, err := json.Marshal(user)
		if err == nil {
			psub.Publish(pCtx, viper.GetString("SIGNUP_TOPIC"), asJSON, true)
		} else {
			return fmt.Errorf("error marshalling user: %v", err)
		}
	} else {
		return fmt.Errorf("error getting user: %v", err)
	}
	return nil
}

func publishUserAddAddress(pCtx context.Context, pUserID persist.DBID, pAddress persist.Address, psub pubsub.PubSub) error {
	input := AddAddressPubSubInput{
		UserID:  pUserID,
		Address: pAddress,
	}
	asJSON, err := json.Marshal(input)
	if err == nil {
		psub.Publish(pCtx, viper.GetString("ADD_ADDRESS_TOPIC"), asJSON, true)
	} else {
		return fmt.Errorf("error marshalling user: %v", err)
	}

	return nil
}

type ErrUserAlreadyExists struct {
	Address       persist.Address
	Authenticator string
}

func (e ErrUserAlreadyExists) Error() string {
	return fmt.Sprintf("user already exists: address: %s, authenticator: %s", e.Address, e.Authenticator)
}

type ErrAddressOwnedByUser struct {
	Address persist.Address
	OwnerID persist.DBID
}

func (e ErrAddressOwnedByUser) Error() string {
	return fmt.Sprintf("address is owned by user: address: %s, ownerID: %s", e.Address, e.OwnerID)
}

type ErrAddressNotOwnedByUser struct {
	Address persist.Address
	UserID  persist.DBID
}

func (e ErrAddressNotOwnedByUser) Error() string {
	return fmt.Sprintf("address is not owned by user: address: %s, userID: %s", e.Address, e.UserID)
}

func (e errCouldNotEnsureMediaForAddress) Error() string {
	return fmt.Sprintf("could not ensure media for address: %s", e.address)
}

type errCouldNotEnsureMediaForAddress struct {
	address persist.Address
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
