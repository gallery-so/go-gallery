package admin

import (
	"errors"

	"github.com/mikeydub/go-gallery/service/persist"
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

// func getNFTs(nftRepo persist.NFTRepository) gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		var input getNFTsInput
// 		if err := c.ShouldBindQuery(&input); err != nil {
// 			util.ErrResponse(c, http.StatusBadRequest, err)
// 			return
// 		}

// 		if input.Address == "" && input.UserID == "" {
// 			util.ErrResponse(c, http.StatusBadRequest, errGetNFTsInput)
// 			return
// 		}

// 		var nfts []persist.NFT
// 		var err error

// 		if input.Address == "" {
// 			nfts, err = nftRepo.GetByUserID(c, input.UserID)
// 		} else {
// 			nfts, err = nftRepo.GetByAddresses(c, []persist.EthereumAddress{input.Address})
// 		}
// 		if err != nil {
// 			util.ErrResponse(c, http.StatusInternalServerError, err)
// 			return
// 		}

// 		c.JSON(http.StatusOK, nfts)
// 	}
// }

// func ownsGeneral(ethClient *ethclient.Client) gin.HandlerFunc {
// 	general, err := contracts.NewIERC1155Caller(common.HexToAddress(env.GetString(ctx, "GENERAL_ADDRESS")), ethClient)
// 	if err != nil {
// 		panic(err)
// 	}
// 	return func(c *gin.Context) {
// 		var input ownsGeneralInput
// 		if err := c.ShouldBindQuery(&input); err != nil {
// 			util.ErrResponse(c, http.StatusBadRequest, err)
// 			return
// 		}
// 		bal, err := general.BalanceOf(&bind.CallOpts{Context: c}, input.Address.Address(), big.NewInt(0))
// 		if err != nil {
// 			util.ErrResponse(c, http.StatusInternalServerError, err)
// 			return
// 		}
// 		c.JSON(http.StatusOK, ownsGeneralOutput{Owns: bal.Uint64() > 0})
// 	}
// }

// func refreshOpensea(nftRepo persist.NFTRepository, userRepo postgres.UserRepository, collRepo postgres.CollectionRepository, galleryRepo postgres.GalleryRepository, backupRepo postgres.BackupRepository) gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		var input RefreshNFTsInput
// 		if err := c.ShouldBindJSON(&input); err != nil {
// 			util.ErrResponse(c, http.StatusBadRequest, err)
// 			return
// 		}
// 		if len(input.UserIDs) == 0 && len(input.Addresses) == 0 {
// 			util.ErrResponse(c, http.StatusBadRequest, errGetNFTsInput)
// 			return
// 		}
// 		err := RefreshOpensea(c, input, userRepo, nftRepo, collRepo, galleryRepo, backupRepo)
// 		if err != nil {
// 			util.ErrResponse(c, http.StatusInternalServerError, err)
// 			return
// 		}

// 		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
// 	}
// }

// // RefreshOpensea refreshes the opensea data for the given user ids and addresses
// func RefreshOpensea(c context.Context, input RefreshNFTsInput, userRepo postgres.UserRepository, nftRepo persist.NFTRepository, collRepo postgres.CollectionRepository, galleryRepo postgres.GalleryRepository, backupRepo postgres.BackupRepository) error {
// 	logrus.Debugf("refreshOpensea input: %+v", input)
// 	if input.UserIDs != nil && len(input.UserIDs) > 0 {
// 		for _, userID := range input.UserIDs {
// 			user, err := userRepo.GetByID(c, userID)
// 			if err != nil {
// 				return err
// 			}
// 			err = opensea.UpdateAssetsForAcc(c, user.ID, persist.WalletsToEthereumAddresses(user.Wallets), nftRepo, userRepo, collRepo, galleryRepo, backupRepo)
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}
// 	if input.Addresses != nil && len(input.Addresses) > 0 {
// 		for _, address := range input.Addresses {
// 			if _, err := opensea.UpdateAssetsForWallet(c, []persist.EthereumAddress{address}, nftRepo); err != nil {
// 				return err
// 			}
// 		}
// 	}
// 	return nil
// }
