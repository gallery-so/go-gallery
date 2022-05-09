package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/everFinance/goar"
	shell "github.com/ipfs/go-ipfs-api"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/service/auth"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/mikeydub/go-gallery/validate"
	"github.com/sirupsen/logrus"
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
	UserID   persist.DBID         `json:"user_id" form:"user_id"`
	Address  persist.AddressValue `json:"address" form:"address"`
	Chain    persist.Chain        `json:"chain" form:"chain"`
	Username string               `json:"username" form:"username"`
}

// GetUserOutput is the output of the user get pipeline
type GetUserOutput struct {
	UserID    persist.DBID         `json:"id"`
	Username  string               `json:"username"`
	BioStr    string               `json:"bio"`
	Addresses []persist.Wallet     `json:"addresses"`
	CreatedAt persist.CreationTime `json:"created_at"`
}

// AddUserAddressesInput is the input for the user add addresses pipeline and also user creation pipeline given that they have the same requirements
type AddUserAddressesInput struct {

	// needed because this is a new user that cant be logged into, and the client creating
	// the user still needs to prove ownership of their address.
	Signature  string               `json:"signature" binding:"signature"`
	Nonce      string               `json:"nonce"`
	Address    persist.AddressValue `json:"address"   binding:"required"`
	Chain      persist.Chain        `json:"chain"`
	WalletType persist.WalletType   `json:"wallet_type"`
}

// AddUserAddressOutput is the output of the user add address pipeline
type AddUserAddressOutput struct {
	SignatureValid bool `json:"signature_valid"`
}

// RemoveUserAddressesInput is the input for the user remove addresses pipeline
type RemoveUserAddressesInput struct {
	Addresses []persist.AddressValue `json:"addresses"   binding:"required"`
	Chains    []persist.Chain        `json:"chains"      binding:"required"`
}

// CreateUserOutput is the output of the user create pipeline
type CreateUserOutput struct {
	SignatureValid bool         `json:"signature_valid"`
	JWTtoken       string       `json:"jwt_token"` // JWT token is sent back to user to use to continue onboarding
	UserID         persist.DBID `json:"user_id"`
	GalleryID      persist.DBID `json:"gallery_id"`
}

// MergeUsersInput is the input for the user merge pipeline
type MergeUsersInput struct {
	SecondUserID persist.DBID         `json:"second_user_id" binding:"required"`
	Signature    string               `json:"signature" binding:"signature"`
	Nonce        string               `json:"nonce"`
	Address      persist.AddressValue `json:"address"   binding:"required"`
	Chain        persist.Chain        `json:"chain"`
	WalletType   persist.WalletType   `json:"wallet_type"`
}

// CreateUserToken creates a JWT token for the user
func CreateUserToken(pCtx context.Context, pInput AddUserAddressesInput, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, galleryRepo persist.GalleryTokenRepository, tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractRepository, walletRepo persist.WalletRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client, multichainProvider *multichain.Provider) (CreateUserOutput, error) {

	output := &CreateUserOutput{}

	nonce, id, _ := auth.GetUserWithNonce(pCtx, pInput.Address, pInput.Chain, userRepo, nonceRepo, walletRepo)
	if nonce == "" {
		return CreateUserOutput{}, auth.ErrNonceNotFound{Address: pInput.Address, Chain: pInput.Chain}
	}
	if id != "" {
		return CreateUserOutput{}, persist.ErrUserAlreadyExists{Address: pInput.Address, Chain: pInput.Chain}
	}

	if pInput.WalletType != persist.WalletTypeEOA {
		if auth.NewNoncePrepend+nonce != pInput.Nonce && auth.NoncePrepend+nonce != pInput.Nonce {
			return CreateUserOutput{}, auth.ErrNonceMismatch
		}
	}

	sigValidBool, err := multichainProvider.VerifySignature(pCtx, pInput.Signature,
		nonce,
		pInput.Address, pInput.Chain, pInput.WalletType)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output.SignatureValid = sigValidBool
	if !sigValidBool {
		return *output, nil
	}

	user := persist.CreateUserInput{
		Address:    pInput.Address,
		Chain:      pInput.Chain,
		WalletType: pInput.WalletType,
	}

	userID, err := userRepo.Create(pCtx, user)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output.UserID = userID

	defer func() {
		// user has been created successfully
		// validate that the user's NFTs are valid and have cached media content
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := validateNFTsForUser(ctx, userID, userRepo, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, stg)
			if err != nil {
				logrus.WithError(err).Error("validateNFTsForUser")
			}
		}()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := ensureMediaContent(ctx, pInput.Address, pInput.Chain, tokenRepo, ethClient, ipfsClient, arweaveClient, stg)
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

	err = auth.NonceRotate(pCtx, pInput.Address, pInput.Chain, userID, nonceRepo)
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
	galleryRepo persist.GalleryRepository) (userID persist.DBID, galleryID persist.DBID, err error) {
	gc := util.GinContextFromContext(pCtx)

	authResult, err := authenticator.Authenticate(pCtx)
	if err != nil {
		return "", "", auth.ErrAuthenticationFailed{WrappedErr: err}
	}

	if authResult.UserID != "" {
		return "", "", persist.ErrUserAlreadyExists{Authenticator: authenticator.GetDescription()}
	}

	// TODO: This currently takes the first authenticated address returned by the authenticator and creates
	// the user's account based on that address. This works because the only auth mechanism we have is nonce-based
	// auth and that supplies a single address. In the future, a user may authenticate in a way that makes
	// multiple authenticated addresses available for initial user creation, and we may want to add all of
	// those addresses to the user's account here.
	address := authResult.Wallets[0]

	user := persist.CreateUserInput{
		Address:    address.Address,
		Chain:      address.Chain,
		WalletType: address.WalletType,
	}

	userID, err = userRepo.Create(pCtx, user)
	if err != nil {
		return "", "", err
	}

	jwtTokenStr, err := auth.JWTGeneratePipeline(pCtx, userID)
	if err != nil {
		return "", "", err
	}

	galleryInsert := persist.GalleryDB{OwnerUserID: userID, Collections: []persist.DBID{}}

	galleryID, err = galleryRepo.Create(pCtx, galleryInsert)
	if err != nil {
		return "", "", err
	}

	auth.SetAuthStateForCtx(gc, userID, nil)
	auth.SetJWTCookie(gc, jwtTokenStr)

	return userID, galleryID, nil
}

// CreateUserREST creates a new user
func CreateUserREST(pCtx context.Context, pInput AddUserAddressesInput, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, galleryRepo persist.GalleryRepository, ethClient *ethclient.Client) (CreateUserOutput, error) {

	authenticator := auth.NonceAuthenticator{
		Address:    pInput.Address,
		Nonce:      pInput.Nonce,
		Signature:  pInput.Signature,
		WalletType: pInput.WalletType,
		UserRepo:   userRepo,
		NonceRepo:  nonceRepo,
		EthClient:  ethClient,
	}

	userID, galleryID, err := CreateUser(pCtx, authenticator, userRepo, galleryRepo)
	if err != nil {
		return CreateUserOutput{}, err
	}

	output := CreateUserOutput{
		SignatureValid: true,
		UserID:         userID,
		GalleryID:      galleryID,
	}

	return output, nil
}

// RemoveAddressesFromUser removes any amount of addresses from a user in the DB
func RemoveAddressesFromUser(pCtx context.Context, pUserID persist.DBID, pAddresses []persist.AddressValue, pChains []persist.Chain,
	userRepo persist.UserRepository, walletRepo persist.WalletRepository) error {
	if len(pAddresses) != len(pChains) {
		return fmt.Errorf("number of addresses (%d) does not match number of chains (%d)", len(pAddresses), len(pChains))
	}

	user, err := userRepo.GetByID(pCtx, pUserID)
	if err != nil {
		return err
	}

	if len(user.Wallets) <= len(pAddresses) {
		return errUserCannotRemoveAllAddresses
	}
	for i := 0; i < len(pAddresses); i++ {
		if err := userRepo.RemoveWallet(pCtx, pUserID, pAddresses[i], pChains[i]); err != nil {
			return err
		}
	}

	return nil
}

// AddWalletToUser adds a single address to a user in the DB because a signature needs to be provided and validated per address
func AddWalletToUser(pCtx context.Context, pUserID persist.DBID, pAddress persist.AddressValue, pChain persist.Chain, addressAuth auth.Authenticator,
	userRepo persist.UserRepository, walletRepo persist.WalletRepository) error {

	authResult, err := addressAuth.Authenticate(pCtx)
	if err != nil {
		return err
	}

	addressUserID := authResult.UserID

	if addressUserID != "" {
		return ErrAddressOwnedByUser{Address: pAddress, Chain: pChain, OwnerID: addressUserID}
	}

	if !auth.ContainsWallet(authResult.Wallets, auth.Wallet{Address: pAddress, Chain: pChain}) {
		return ErrAddressNotOwnedByUser{Address: pAddress, Chain: pChain, UserID: addressUserID}
	}

	// TODO insert wallet and update user with wallet

	return nil
}

// AddAddressToUserToken adds a single address to a user in the DB because a signature needs to be provided and validated per address
func AddAddressToUserToken(pCtx context.Context, pUserID persist.DBID, pAddress persist.AddressValue, pChain persist.Chain, pWalletType persist.WalletType, addressAuth auth.Authenticator,
	userRepo persist.UserRepository, tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) error {

	authResult, err := addressAuth.Authenticate(pCtx)
	if err != nil {
		return err
	}

	addressUserID := authResult.UserID

	if addressUserID != "" {
		return ErrAddressOwnedByUser{Address: pAddress, OwnerID: addressUserID}
	}

	if !auth.ContainsWallet(authResult.Wallets, auth.Wallet{Address: pAddress, Chain: pChain}) {
		return ErrAddressNotOwnedByUser{Address: pAddress, UserID: addressUserID}
	}

	defer func() {
		// user has successfully added an address
		// validate that the user's NFTs are valid and have cached media content

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := validateNFTsForUser(ctx, pUserID, userRepo, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, stg)
			if err != nil {
				logrus.WithError(err).Error("validateNFTsForUser")
			}
		}()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
			defer cancel()
			err := ensureMediaContent(ctx, pAddress, pChain, tokenRepo, ethClient, ipfsClient, arweaveClient, stg)
			if err != nil {
				logrus.WithError(err).Error("ensureMediaForUser")
			}
		}()
	}()

	// TODO add address to user waterfalls to wallet and address table
	if err := userRepo.AddWallet(pCtx, pUserID, pAddress, pChain, pWalletType); err != nil {
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

	if len(user.Wallets) <= len(pInput.Addresses) {
		return errUserCannotRemoveAllAddresses
	}

	return nil
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
	case pInput.Address.String() != "":
		user, err = userRepo.GetByAddressDetails(pCtx, pInput.Address, pInput.Chain)
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
		Addresses: user.Wallets,
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
		for _, addr := range user.Wallets {
			if resolves, _ := eth.ResolvesENS(pCtx, username, persist.EthereumAddress(addr.Address.AddressValue), ethClient); resolves {
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
func MergeUsers(pCtx context.Context, userRepo persist.UserRepository, nonceRepo persist.NonceRepository, walletRepo persist.WalletRepository, pUserID persist.DBID, pInput MergeUsersInput, multichainProvider *multichain.Provider) error {
	nonce, id, _ := auth.GetUserWithNonce(pCtx, pInput.Address, pInput.Chain, userRepo, nonceRepo, walletRepo)
	if nonce == "" {
		return auth.ErrNonceNotFound{Address: pInput.Address}
	}
	if id != pInput.SecondUserID {
		return fmt.Errorf("wrong nonce: user %s is not the second user", pInput.SecondUserID)
	}

	if pInput.WalletType != persist.WalletTypeEOA {
		if auth.NewNoncePrepend+nonce != pInput.Nonce && auth.NoncePrepend+nonce != pInput.Nonce {
			return auth.ErrNonceMismatch
		}
	}

	sigValidBool, err := multichainProvider.VerifySignature(pCtx, pInput.Signature,
		nonce,
		pInput.Address, pInput.Chain, pInput.WalletType)
	if err != nil {
		return err
	}

	if !sigValidBool {
		return fmt.Errorf("signature is invalid for address %s", pInput.Address)
	}

	return userRepo.MergeUsers(pCtx, pUserID, pInput.SecondUserID)

}

// TODO need interface for interacting with other chains for this
func validateNFTsForUser(pCtx context.Context, pUserID persist.DBID, userRepo persist.UserRepository, tokenRepo persist.TokenGalleryRepository, contractRepo persist.ContractRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) error {
	// input := indexer.ValidateUsersNFTsInput{
	// 	UserID: pUserID,
	// }

	// _, err := indexer.ValidateNFTs(pCtx, input, userRepo, tokenRepo, contractRepo, ethClient, ipfsClient, arweaveClient, stg)
	// if err != nil {
	// 	logrus.Errorf("Error validating user NFTs %s: %s", pUserID, err)
	// 	return err
	// }
	return nil
}

// TODO need interface for interacting with other chains for this
func ensureMediaContent(pCtx context.Context, pAddress persist.AddressValue, pChain persist.Chain, tokenRepo persist.TokenGalleryRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) error {

	// input := indexer.UpdateMediaInput{
	// 	OwnerAddress: pAddress,
	// }

	// err := indexer.UpdateMedia(pCtx, input, tokenRepo, ethClient, ipfsClient, arweaveClient, stg)
	// if err != nil {
	// 	logrus.Errorf("Error ensuring media content for address %s: %s", pAddress, err)
	// 	return err
	// }
	return nil

}

// DoesUserOwnWallets checks if a user owns any wallets
func DoesUserOwnWallets(pCtx context.Context, userID persist.DBID, walletAddresses []persist.DBID, userRepo persist.UserRepository) (bool, error) {
	user, err := userRepo.GetByID(pCtx, userID)
	if err != nil {
		return false, err
	}
	walletIDs := make([]persist.DBID, len(user.Wallets))
	for i, wallet := range user.Wallets {
		walletIDs[i] = wallet.ID
	}
	for _, walletAddress := range walletAddresses {
		if !persist.ContainsDBID(walletAddresses, walletAddress) {
			return false, nil
		}
	}
	return true, nil
}

// ContainsWallets checks if an array of wallets contains another wallet
func ContainsWallets(a []persist.Wallet, b persist.Wallet) bool {
	for _, v := range a {
		if v.Address == b.Address {
			return true
		}
	}

	return false
}

type ErrDoesNotOwnWallets struct {
	ID        persist.DBID
	Addresses []persist.Wallet
}

func (e ErrDoesNotOwnWallets) Error() string {
	return fmt.Sprintf("user with ID %s does not own all wallets: %+v", e.ID, e.Addresses)
}

type ErrUserAlreadyExists struct {
	Address       persist.AddressValue
	Chain         persist.Chain
	Authenticator string
}

func (e ErrUserAlreadyExists) Error() string {
	return fmt.Sprintf("user already exists: address: %s, authenticator: %s", e.Address, e.Authenticator)
}

type ErrAddressOwnedByUser struct {
	Address persist.AddressValue
	Chain   persist.Chain
	OwnerID persist.DBID
}

func (e ErrAddressOwnedByUser) Error() string {
	return fmt.Sprintf("address is owned by user: address: %s, ownerID: %s", e.Address, e.OwnerID)
}

type ErrAddressNotOwnedByUser struct {
	Address persist.AddressValue
	Chain   persist.Chain
	UserID  persist.DBID
}

func (e ErrAddressNotOwnedByUser) Error() string {
	return fmt.Sprintf("address is not owned by user: address: %s, userID: %s", e.Address, e.UserID)
}

func (e errCouldNotEnsureMediaForAddress) Error() string {
	return fmt.Sprintf("could not ensure media for address: %s", e.address)
}

type errCouldNotEnsureMediaForAddress struct {
	address persist.Wallet
}

// containsWallet checks whether an address exists in a slice
func containsWallet(a []persist.Wallet, b persist.Wallet) bool {
	for _, v := range a {
		if v == b {
			return true
		}
	}

	return false
}
