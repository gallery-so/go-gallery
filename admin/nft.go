package admin

import (
	"context"
	"errors"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var errGetNFTsInput = errors.New("address or user_id must be provided")

type getNFTsInput struct {
	Address persist.EthereumAddress `form:"address"`
	UserID  persist.DBID            `form:"user_id"`
}

type ownsGeneralInput struct {
	Address persist.EthereumAddress `form:"address" binding:"required"`
}

type ownsGeneralOutput struct {
	Owns bool `json:"owns"`
}

// RefreshNFTsInput is the input for the refreshOpensea function
type RefreshNFTsInput struct {
	UserIDs   []persist.DBID            `json:"user_ids"`
	Addresses []persist.EthereumAddress `json:"addresses"`
}

func getNFTs(nftRepo persist.NFTRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input getNFTsInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		if input.Address == "" && input.UserID == "" {
			util.ErrResponse(c, http.StatusBadRequest, errGetNFTsInput)
			return
		}

		var nfts []persist.NFT
		var err error

		if input.Address == "" {
			nfts, err = nftRepo.GetByUserID(c, input.UserID)
		} else {
			nfts, err = nftRepo.GetByAddresses(c, []persist.EthereumAddress{input.Address})
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, nfts)
	}
}

func ownsGeneral(ethClient *ethclient.Client) gin.HandlerFunc {
	general, err := contracts.NewIERC1155Caller(common.HexToAddress(viper.GetString("GENERAL_ADDRESS")), ethClient)
	if err != nil {
		panic(err)
	}
	return func(c *gin.Context) {
		var input ownsGeneralInput
		if err := c.ShouldBindQuery(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		bal, err := general.BalanceOf(&bind.CallOpts{Context: c}, input.Address.Address(), big.NewInt(0))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, ownsGeneralOutput{Owns: bal.Uint64() > 0})
	}
}

func refreshOpensea(nftRepo persist.NFTRepository, userRepo persist.UserRepository, collRepo persist.CollectionRepository, galleryRepo persist.GalleryRepository, backupRepo persist.BackupRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input RefreshNFTsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		if len(input.UserIDs) == 0 && len(input.Addresses) == 0 {
			util.ErrResponse(c, http.StatusBadRequest, errGetNFTsInput)
			return
		}
		err := RefreshOpensea(c, input, userRepo, nftRepo, collRepo, galleryRepo, backupRepo)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
}

// RefreshOpensea refreshes the opensea data for the given user ids and addresses
func RefreshOpensea(c context.Context, input RefreshNFTsInput, userRepo persist.UserRepository, nftRepo persist.NFTRepository, collRepo persist.CollectionRepository, galleryRepo persist.GalleryRepository, backupRepo persist.BackupRepository) error {
	logrus.Debugf("refreshOpensea input: %+v", input)
	if input.UserIDs != nil && len(input.UserIDs) > 0 {
		for _, userID := range input.UserIDs {
			user, err := userRepo.GetByID(c, userID)
			if err != nil {
				return err
			}
			err = opensea.UpdateAssetsForAcc(c, user.ID, persist.WalletsToEthereumAddresses(user.Wallets), nftRepo, userRepo, collRepo, galleryRepo, backupRepo)
			if err != nil {
				return err
			}
		}
	}
	if input.Addresses != nil && len(input.Addresses) > 0 {
		for _, address := range input.Addresses {
			if _, err := opensea.UpdateAssetsForWallet(c, []persist.EthereumAddress{address}, nftRepo); err != nil {
				return err
			}
		}
	}
	return nil
}
