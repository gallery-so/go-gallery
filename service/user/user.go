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

type errUserNotFound struct {
	userID   persist.DBID
	address  persist.Address
	username string
}

type errNonceNotFound struct {
	address persist.Address
}
type errUserExistsWithAddress struct {
	address persist.Address
}

type errCouldNotEnsureMediaForAddress struct {
	address persist.Address
}

// CreateUserToken creates a JWT token for the user
func CreateUserToken(pCtx context.Context, pInput AddUserAddressesInput, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, galleryRepo persist.GalleryTokenRepository, ethClient *ethclient.Client, psub pubsub.PubSub) (CreateUserOutput, error) {

	output := &CreateUserOutput{}

	nonce, id, _ := auth.GetUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if nonce == "" {
		return CreateUserOutput{}, errNonceNotFound{address: pInput.Address}
	}
	if id != "" {
		return CreateUserOutput{}, errUserExistsWithAddress{address: pInput.Address}
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
func CreateUser(pCtx context.Context, pInput AddUserAddressesInput, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, galleryRepo persist.GalleryRepository, ethClient *ethclient.Client) (CreateUserOutput, error) {

	output := CreateUserOutput{}

	nonce, id, _ := auth.GetUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if nonce == "" {
		return CreateUserOutput{}, errNonceNotFound{pInput.Address}
	}
	if id != "" {
		return CreateUserOutput{}, errUserExistsWithAddress{address: pInput.Address}
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
		return output, nil
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

	galleryInsert := persist.GalleryDB{OwnerUserID: userID, Collections: []persist.DBID{}}

	galleryID, err := galleryRepo.Create(pCtx, galleryInsert)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output.GalleryID = galleryID

	return output, nil
}

// RemoveAddressesFromUser removes any amount of addresses from a user in the DB
func RemoveAddressesFromUser(pCtx context.Context, pUserID persist.DBID, pInput RemoveUserAddressesInput,
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

// AddAddressToUser adds a single address to a user in the DB because a signature needs to be provided and validated per address
func AddAddressToUser(pCtx context.Context, pUserID persist.DBID, pInput AddUserAddressesInput,
	userRepo persist.UserRepository, nonceRepo persist.NonceRepository, ethClient *ethclient.Client, psub pubsub.PubSub) (AddUserAddressOutput, error) {

	output := AddUserAddressOutput{}

	nonce, userID, _ := auth.GetUserWithNonce(pCtx, pInput.Address, userRepo, nonceRepo)
	if nonce == "" {
		return AddUserAddressOutput{}, errNonceNotFound{pInput.Address}
	}
	if userID != "" {
		return AddUserAddressOutput{}, errUserExistsWithAddress{pInput.Address}
	}

	if pInput.WalletType != auth.WalletTypeEOA {
		if auth.NewNoncePrepend+nonce != pInput.Nonce && auth.NoncePrepend+nonce != pInput.Nonce {
			return AddUserAddressOutput{}, auth.ErrNonceMismatch
		}
	}

	dataStr := nonce
	sigValidBool, err := auth.VerifySignatureAllMethods(pInput.Signature,
		dataStr,
		pInput.Address, pInput.WalletType, ethClient)
	if err != nil {
		return AddUserAddressOutput{}, err
	}

	output.SignatureValid = sigValidBool
	if !sigValidBool {
		return output, nil
	}

	defer func() {
		// user has successfully added an address, shoot out a pubsub to notify any service that needs it as well as
		// validate that the user's NFTs are valid and have cached media content
		if viper.GetString("ENV") != "local" {
			go func() {
				err := publishUserAddAddress(pCtx, pUserID, pInput.Address, psub)
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
			err := ensureMediaContent(ctx, pInput.Address)
			if err != nil {
				logrus.WithError(err).Error("ensureMediaForUser")
			}
		}()
	}()

	if err = userRepo.AddAddresses(pCtx, pUserID, []persist.Address{pInput.Address}); err != nil {
		return AddUserAddressOutput{}, err
	}

	err = auth.NonceRotate(pCtx, pInput.Address, pUserID, nonceRepo)
	if err != nil {
		return AddUserAddressOutput{}, err
	}

	return output, nil
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
		return GetUserOutput{}, auth.ErrUserNotFound{UserID: pInput.UserID, Address: pInput.Address, Username: pInput.Username}
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
func UpdateUser(pCtx context.Context, userID persist.DBID, input UpdateUserInput, userRepository persist.UserRepository, ethClient *ethclient.Client) error {
	if strings.HasSuffix(strings.ToLower(input.UserName), ".eth") {
		user, err := userRepository.GetByID(pCtx, userID)
		if err != nil {
			return err
		}
		can := false
		for _, addr := range user.Addresses {
			if resolves, _ := eth.ResolvesENS(pCtx, input.UserName, addr, ethClient); resolves {
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
			UsernameIdempotent: persist.NullString(strings.ToLower(input.UserName)),
			Username:           persist.NullString(input.UserName),
			Bio:                persist.NullString(validate.SanitizationPolicy.Sanitize(input.BioStr)),
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
		return errNonceNotFound{address: pInput.Address}
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

func (e errNonceNotFound) Error() string {
	return fmt.Sprintf("nonce not found for address: %s", e.address)
}

func (e errUserExistsWithAddress) Error() string {
	return fmt.Sprintf("user already exists with address: %s", e.address)
}

func (e errCouldNotEnsureMediaForAddress) Error() string {
	return fmt.Sprintf("could not ensure media for address: %s", e.address)
}
