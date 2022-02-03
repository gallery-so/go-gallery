package indexer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
)

var errInvalidUpdateMediaInput = errors.New("must provide either owner_address or token_id and contract_address")

// UpdateMediaInput is the input for the update media endpoint that will find all of the media content
// for an addresses NFTs and cache it in a storage bucket
type UpdateMediaInput struct {
	OwnerAddress    persist.Address `json:"owner_address"`
	TokenID         persist.TokenID `json:"token_id"`
	ContractAddress persist.Address `json:"contract_address"`
}

// ValidateUsersNFTsInput is the input for the validate users NFTs endpoint that will return
// whether what opensea has on a user is the same as what we have in our database
type ValidateUsersNFTsInput struct {
	UserID persist.DBID `json:"user_id"`
}

// ValidateUsersNFTsOutput is the output of the validate users NFTs endpoint that will return
// whether what opensea has on a user is the same as what we have in our database
type ValidateUsersNFTsOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type tokenUpdateMedia struct {
	TokenDBID       persist.DBID
	TokenID         persist.TokenID
	ContractAddress persist.Address
	Update          persist.TokenUpdateMediaInput
}

func getStatus(i *Indexer, tokenRepository persist.TokenRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c, 10*time.Second)
		defer cancel()
		total, _ := tokenRepository.Count(ctx, persist.CountTypeTotal)
		mostRecent, _ := tokenRepository.MostRecentBlock(ctx)
		noMetadata, _ := tokenRepository.Count(ctx, persist.CountTypeNoMetadata)
		erc721, _ := tokenRepository.Count(ctx, persist.CountTypeERC721)
		erc1155, _ := tokenRepository.Count(ctx, persist.CountTypeERC1155)

		c.JSON(http.StatusOK, gin.H{
			"total_tokens": total,
			"recent_block": i.mostRecentBlock,
			"most_recent":  mostRecent,
			"bad_uris":     i.badURIs,
			"no_metadata":  noMetadata,
			"erc721":       erc721,
			"erc1155":      erc1155,
		})
	}
}

func updateMedia(tq *task.Queue, tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, storageClient *storage.Client) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		input := UpdateMediaInput{}
		if err := ginContext.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(ginContext, http.StatusBadRequest, err)
			return
		}

		var tokens []persist.Token
		var key string
		if input.OwnerAddress != "" {
			t, err := tokenRepository.GetByWallet(ginContext, input.OwnerAddress, -1, -1)
			if err != nil {
				util.ErrResponse(ginContext, http.StatusInternalServerError, err)
				return
			}
			tokens = t
			key = input.OwnerAddress.String()
		} else if input.TokenID != "" && input.ContractAddress != "" {
			t, err := tokenRepository.GetByTokenIdentifiers(ginContext, input.TokenID, input.ContractAddress, 1, 0)
			if err != nil {
				util.ErrResponse(ginContext, http.StatusInternalServerError, err)
				return
			}
			tokens = t
			key = persist.NewTokenIdentifiers(input.ContractAddress, input.TokenID).String()
		} else {
			util.ErrResponse(ginContext, http.StatusBadRequest, errInvalidUpdateMediaInput)
			return
		}
		ginContext.JSON(http.StatusOK, util.SuccessResponse{Success: true})

		tq.QueueTask(key, func() {
			c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			updates, errChan := updateMediaForTokens(c, tokens, ethClient, ipfsClient, storageClient)
			for i := 0; i < len(tokens); i++ {
				select {
				case update := <-updates:
					if input.OwnerAddress != "" {
						if err := tokenRepository.UpdateByIDUnsafe(c, update.TokenDBID, update.Update); err != nil {
							logrus.WithError(err).Error("failed to update token in database")
							return
						}
					} else if input.ContractAddress != "" && input.TokenID != "" {
						if err := tokenRepository.UpdateByTokenIdentifiersUnsafe(c, update.TokenID, update.ContractAddress, update.Update); err != nil {
							logrus.WithError(err).Error("failed to update token in database")
							return
						}
					}
				case err := <-errChan:
					if err != nil {
						logrus.WithError(err).Error("failed to update media for token")
					}
				}

			}

		})
	}
}

// updateMediaForTokens will return two channels that will collectively receive the length of the tokens passed in
func updateMediaForTokens(ctx context.Context, tokens []persist.Token, ethClient *ethclient.Client, ipfsClient *shell.Shell, storageClient *storage.Client) (<-chan tokenUpdateMedia, <-chan error) {
	updateChan := make(chan tokenUpdateMedia)
	errChan := make(chan error)
	for _, t := range tokens {
		go func(token persist.Token) {

			uri := token.TokenURI
			metadata := token.TokenMetadata
			med := token.Media

			if _, ok := metadata["error"]; ok || uri == persist.InvalidTokenURI || med.MediaType == persist.MediaTypeInvalid {
				errChan <- nil
				return
			}

			if uri.Type() == persist.URITypeNone {
				u, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethClient)
				if err != nil {
					errChan <- fmt.Errorf("failed to get token URI: %v", err)
					return
				}
				uri = u
			}

			if metadata == nil || len(metadata) == 0 {
				md, err := rpc.GetMetadataFromURI(ctx, token.TokenURI, ipfsClient)
				if err != nil {
					errChan <- fmt.Errorf("failed to get metadata for token %s: %v", token.TokenID, err)
					return
				}
				metadata = md
			}

			if med.MediaType == "" && med.MediaURL == "" {
				m, err := media.MakePreviewsForMetadata(ctx, metadata, token.ContractAddress, token.TokenID, uri, ipfsClient, storageClient)
				if err != nil {
					errChan <- fmt.Errorf("failed to make media for token %s: %v", token.TokenID, err)
					return
				}
				med = m
			}

			updateChan <- tokenUpdateMedia{
				TokenDBID:       token.ID,
				TokenID:         token.TokenID,
				ContractAddress: token.ContractAddress,
				Update: persist.TokenUpdateMediaInput{
					TokenURI: uri,
					Metadata: metadata,
					Media:    med,
				},
			}
		}(t)
	}
	return updateChan, errChan
}

func validateUsersNFTs(tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, userRepository persist.UserRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, stg *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input ValidateUsersNFTsInput
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}
		user, err := userRepository.GetByID(c, input.UserID)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		currentNFTs, err := tokenRepository.GetByUserID(c, input.UserID, -1, 0)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := processAccountForNFTs(c, currentNFTs, tokenRepository, ethcl, ipfsClient); err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		openseaAssets := make([]opensea.Asset, 0, len(currentNFTs))
		for _, address := range user.Addresses {
			assets, err := opensea.FetchAssetsForWallet(address, 0, 0, nil)
			if err != nil {
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			openseaAssets = append(openseaAssets, assets...)
		}

		accountedFor := make(map[persist.DBID]bool)
		unaccountedFor := make(map[string]opensea.Asset)

		for _, asset := range openseaAssets {
			af := false
			for _, nft := range currentNFTs {
				if accountedFor[nft.ID] {
					continue
				}
				if asset.Contract.ContractAddress == nft.ContractAddress && asset.TokenID.String() == nft.TokenID.Base10String() {
					accountedFor[nft.ID] = true
					af = true
					break
				}
			}
			if !af {
				unaccountedFor[asset.Contract.ContractAddress.String()+" -- "+asset.TokenID.String()] = asset
			}
		}

		var output ValidateUsersNFTsOutput

		if len(unaccountedFor) > 0 {
			unaccountedForKeys := make([]string, 0, len(unaccountedFor))
			for k := range unaccountedFor {
				unaccountedForKeys = append(unaccountedForKeys, k)
			}
			accountedForKeys := make([]string, 0, len(accountedFor))
			for k := range accountedFor {
				accountedForKeys = append(accountedForKeys, k.String())
			}
			output.Success = false
			output.Message = fmt.Sprintf("user %s has unaccounted for NFTs: %v | accounted for NFTs: %v", input.UserID, unaccountedForKeys, accountedForKeys)

			allUnaccountedForAssets := make([]opensea.Asset, 0, len(unaccountedFor))
			for _, asset := range unaccountedFor {
				allUnaccountedForAssets = append(allUnaccountedForAssets, asset)
			}

			if err := processUnaccountedForNFTs(c, allUnaccountedForAssets, user.Addresses, tokenRepository, contractRepository, userRepository, ethcl, ipfsClient, stg); err != nil {
				logrus.WithError(err).Error("failed to process unaccounted for NFTs")
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
		}

		output.Success = true
		c.JSON(http.StatusOK, output)

	}
}

func processAccountForNFTs(ctx context.Context, tokens []persist.Token, tokenRepository persist.TokenRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell) error {
	for _, token := range tokens {
		uriType := token.TokenURI.Type()
		needsUpdate := false
		if uriType == persist.URITypeNone || uriType == persist.URITypeUnknown {
			uri, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethcl)
			if err != nil {
				return fmt.Errorf("failed to get token URI for token %s: %v", token.TokenID, err)
			}
			token.TokenURI = uri
			needsUpdate = true
		}

		if token.TokenMetadata == nil || len(token.TokenMetadata) == 0 {
			metadata, err := rpc.GetMetadataFromURI(ctx, token.TokenURI, ipfsClient)
			if err != nil {
				return fmt.Errorf("failed to get token metadata for token %s: %v", token.TokenID, err)
			}
			token.TokenMetadata = metadata
			needsUpdate = true
		}

		if needsUpdate {
			update := persist.TokenUpdateMediaInput{
				TokenURI: token.TokenURI,
				Metadata: token.TokenMetadata,
				Media:    token.Media,
			}

			if err := tokenRepository.UpdateByIDUnsafe(ctx, token.ID, update); err != nil {
				return fmt.Errorf("failed to update token %s: %v", token.TokenID, err)
			}
		}

	}
	return nil
}
func processUnaccountedForNFTs(ctx context.Context, assets []opensea.Asset, addresses []persist.Address, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, userRepository persist.UserRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, stg *storage.Client) error {
	allTokens := make([]persist.Token, 0, len(assets))
	cntracts := make([]persist.Contract, 0, len(assets))
	block, err := ethcl.BlockNumber(ctx)
	if err != nil {
		return err
	}
	for _, a := range assets {

		logrus.Debugf("processing asset: %+v", a)

		asURI := persist.TokenURI(a.ImageURL)
		media := persist.Media{}

		bs, err := rpc.GetDataFromURI(ctx, asURI, ipfsClient)
		if err == nil {
			mediaType := persist.SniffMediaType(bs)
			if mediaType != persist.MediaTypeUnknown {
				media = persist.Media{
					MediaURL:     persist.NullString(a.ImageURL),
					ThumbnailURL: persist.NullString(a.ImageThumbnailURL),
					PreviewURL:   persist.NullString(a.ImagePreviewURL),
					MediaType:    mediaType,
				}
			}
		}

		logrus.Debugf("media: %+v", media)

		metadata, _ := rpc.GetMetadataFromURI(ctx, persist.TokenURI(a.TokenMetadataURL), ipfsClient)

		logrus.Debugf("metadata: %+v", metadata)
		t := persist.Token{
			Name:            persist.NullString(a.Name),
			Description:     persist.NullString(a.Description),
			Chain:           persist.ChainETH,
			TokenID:         persist.TokenID(a.TokenID.ToBase16()),
			ContractAddress: a.Contract.ContractAddress,
			OwnerAddress:    a.Owner.Address,
			TokenURI:        persist.TokenURI(a.TokenMetadataURL),
			ExternalURL:     persist.NullString(a.ExternalURL),
			TokenMetadata:   metadata,
			Media:           media,
			BlockNumber:     persist.BlockNumber(block),
		}
		if a.AnimationURL != "" {
			t.TokenMetadata["animation"] = a.AnimationURL
		}
		switch a.Contract.ContractSchemaName {
		case "ERC721":
			t.TokenType = persist.TokenTypeERC721
			allTokens = append(allTokens, t)
		case "ERC1155":
			t.TokenType = persist.TokenTypeERC1155
			ierc1155, err := contracts.NewIERC1155Caller(t.ContractAddress.Address(), ethcl)
			if err != nil {
				return err
			}
			bigZero := big.NewInt(0)
			for _, addr := range addresses {
				new := t
				bal, err := ierc1155.BalanceOf(&bind.CallOpts{Context: ctx}, addr.Address(), t.TokenID.BigInt())
				if err != nil {
					return err
				}
				if bal.Cmp(bigZero) > 0 {
					new.OwnerAddress = addr
					new.Quantity = persist.HexString(bal.Text(16))

					allTokens = append(allTokens, new)
				}
			}
		default:
			return fmt.Errorf("unsupported token type: %s", a.Contract.ContractSchemaName)
		}

		c := persist.Contract{
			Address:     a.Contract.ContractAddress,
			Symbol:      a.Contract.ContractSymbol,
			Name:        a.Contract.ContractName,
			LatestBlock: persist.BlockNumber(block),
		}
		cntracts = append(cntracts, c)
	}

	if err := contractRepository.BulkUpsert(ctx, cntracts); err != nil {
		return err
	}

	if err := tokenRepository.BulkUpsert(ctx, allTokens); err != nil {
		return err
	}

	return nil
}
