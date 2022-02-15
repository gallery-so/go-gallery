package indexer

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/service/media"
	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/service/rpc"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"google.golang.org/appengine"
)

var errInvalidUpdateMediaInput = errors.New("must provide either owner_address or token_id and contract_address")

var mediaDownloadLock = &sync.Mutex{}

var bigZero = big.NewInt(0)

// UpdateMediaInput is the input for the update media endpoint that will find all of the media content
// for an addresses NFTs and cache it in a storage bucket
type UpdateMediaInput struct {
	OwnerAddress    persist.Address `json:"owner_address"`
	TokenID         persist.TokenID `json:"token_id"`
	ContractAddress persist.Address `json:"contract_address"`
	UpdateAll       bool            `json:"update_all"`
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
	Message string `json:"message,omitempty"`
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

func updateMedia(tokenRepository persist.TokenRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		input := UpdateMediaInput{}
		if err := c.ShouldBindJSON(&input); err != nil {
			util.ErrResponse(c, http.StatusBadRequest, err)
			return
		}

		var tokens []persist.Token
		var err error
		if input.OwnerAddress != "" {
			tokens, err = tokenRepository.GetByWallet(c, input.OwnerAddress, -1, -1)
		} else if input.TokenID != "" && input.ContractAddress != "" {
			tokens, err = tokenRepository.GetByTokenIdentifiers(c, input.TokenID, input.ContractAddress, 1, 0)
		} else {
			util.ErrResponse(c, http.StatusBadRequest, errInvalidUpdateMediaInput)
			return
		}
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if !input.UpdateAll {
			res := make([]persist.Token, 0, len(tokens))
			for _, token := range tokens {
				switch token.Media.MediaType {
				case persist.MediaTypeVideo:
					if token.Media.MediaURL == "" || token.Media.ThumbnailURL == "" {
						res = append(res, token)
					}
				default:
					if token.Media.MediaURL == "" || token.Media.MediaType == "" {
						res = append(res, token)
					}
				}
			}
			tokens = res
		}

		logrus.Infof("Updating %d tokens", len(tokens))

		updateByID := input.OwnerAddress != ""

		appCtx := appengine.WithContext(c, c.Request)

		updates, errChan := updateMediaForTokens(appCtx, tokens, ethClient, ipfsClient, arweaveClient, storageClient)
		for i := 0; i < len(tokens); i++ {
			select {
			case update := <-updates:
				if updateByID {
					if err := tokenRepository.UpdateByIDUnsafe(c, update.TokenDBID, update.Update); err != nil {
						logrus.WithError(err).Error("failed to update token in database")
						util.ErrResponse(c, http.StatusInternalServerError, err)
						return
					}
				} else {
					if err := tokenRepository.UpdateByTokenIdentifiersUnsafe(c, update.TokenID, update.ContractAddress, update.Update); err != nil {
						logrus.WithError(err).Error("failed to update token in database")
						util.ErrResponse(c, http.StatusInternalServerError, err)
						return
					}
				}
			case err := <-errChan:
				if err != nil {
					logrus.WithError(err).Error("failed to update media for token")
				}
			}
		}
		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})

	}
}

// updateMediaForTokens will return two channels that will collectively receive the length of the tokens passed in
func updateMediaForTokens(ctx context.Context, tokens []persist.Token, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, storageClient *storage.Client) (<-chan tokenUpdateMedia, <-chan error) {
	updateChan := make(chan tokenUpdateMedia)
	errChan := make(chan error)
	wp := workerpool.New(10)
	uniqueHandlers := getUniqueMetadataHandlers()
	for _, t := range tokens {
		token := t
		wp.Submit(func() {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			uri := token.TokenURI
			metadata := token.TokenMetadata

			if handler, ok := uniqueHandlers[persist.Address(token.ContractAddress.String())]; ok {
				logrus.Infof("Using %v metadata handler for %s", handler, token.ContractAddress)
				u, md, err := handler(token.TokenURI, token.ContractAddress, token.TokenID)
				if err != nil {
					errChan <- fmt.Errorf("failed to get unique metadata for token %s: %s", token.TokenURI, err)
					return
				}
				metadata = md
				uri = u
			} else {
				if _, ok := metadata["error"]; ok || uri == persist.InvalidTokenURI || token.Media.MediaType == persist.MediaTypeInvalid {
					logrus.Debugf("skipping token %s-%s", token.ContractAddress, token.TokenID)
					errChan <- nil
					return
				}

				if uri.Type() == persist.URITypeNone {
					u, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethClient)
					if err != nil {
						errChan <- fmt.Errorf("failed to get token URI: %v", err)
						return
					}
					uri = persist.TokenURI(strings.ReplaceAll(u.String(), "{id}", token.TokenID.ToUint256String()))
				}

				if metadata == nil || len(metadata) == 0 {
					md, err := rpc.GetMetadataFromURI(ctx, token.TokenURI, ipfsClient, arweaveClient)
					if err != nil {
						errChan <- fmt.Errorf("failed to get metadata for token %s: %v", token.TokenID, err)
						return
					}
					metadata = md
				}
			}

			med, err := media.MakePreviewsForMetadata(ctx, metadata, token.ContractAddress, token.TokenID, uri, ipfsClient, arweaveClient, storageClient)
			if err != nil {
				errChan <- fmt.Errorf("failed to make media for token %s-%s: %v", token.ContractAddress, token.TokenID, err)
				return
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

		})
	}
	return updateChan, errChan
}

func validateUsersNFTs(tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, userRepository persist.UserRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) gin.HandlerFunc {
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

		output := ValidateUsersNFTsOutput{Success: true}

		newMsg, err := processAccountedForNFTs(c, currentNFTs, tokenRepository, ethcl, ipfsClient, arweaveClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		output.Message += newMsg

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

		if len(unaccountedFor) > 0 {
			unaccountedForKeys := make([]string, 0, len(unaccountedFor))
			for k := range unaccountedFor {
				unaccountedForKeys = append(unaccountedForKeys, k)
			}
			accountedForKeys := make([]string, 0, len(accountedFor))
			for k := range accountedFor {
				accountedForKeys = append(accountedForKeys, k.String())
			}

			allUnaccountedForAssets := make([]opensea.Asset, 0, len(unaccountedFor))
			for _, asset := range unaccountedFor {
				allUnaccountedForAssets = append(allUnaccountedForAssets, asset)
			}

			if err := processUnaccountedForNFTs(c, allUnaccountedForAssets, user.Addresses, tokenRepository, contractRepository, userRepository, ethcl, ipfsClient, arweaveClient, stg); err != nil {
				logrus.WithError(err).Error("failed to process unaccounted for NFTs")
				util.ErrResponse(c, http.StatusInternalServerError, err)
				return
			}
			output.Success = false
			output.Message += fmt.Sprintf("user %s has unaccounted for NFTs: %v | accounted for NFTs: %v", input.UserID, unaccountedForKeys, accountedForKeys)
		}
		c.JSON(http.StatusOK, output)

	}
}

func processAccountedForNFTs(ctx context.Context, tokens []persist.Token, tokenRepository persist.TokenRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client) (string, error) {
	msgToAdd := ""
	for _, token := range tokens {

		needsUpdate := false

		uri, err := rpc.GetTokenURI(ctx, token.TokenType, token.ContractAddress, token.TokenID, ethcl)
		if err == nil {
			token.TokenURI = persist.TokenURI(strings.ReplaceAll(uri.String(), "{id}", token.TokenID.ToUint256String()))
			needsUpdate = true
		} else {
			logrus.Errorf("failed to get token URI for token %s-%s: %v", token.ContractAddress, token.TokenID, err)
			msgToAdd += fmt.Sprintf("failed to get token URI for token %s-%s: %v\n", token.ContractAddress, token.TokenID, err)
			continue
		}

		metadata, err := rpc.GetMetadataFromURI(ctx, token.TokenURI, ipfsClient, arweaveClient)
		if err == nil {
			token.TokenMetadata = metadata
			needsUpdate = true
		} else {
			logrus.Errorf("failed to get token metadata for token %s-%s with uri %s: %v", token.ContractAddress, token.TokenID, token.TokenURI, err)
			msgToAdd += fmt.Sprintf("failed to get token metadata for token %s-%s with uri %s: %v", token.ContractAddress, token.TokenID, token.TokenURI, err)
			continue
		}

		if needsUpdate {
			update := persist.TokenUpdateMediaInput{
				TokenURI: token.TokenURI,
				Metadata: token.TokenMetadata,
				Media:    token.Media,
			}

			if err := tokenRepository.UpdateByIDUnsafe(ctx, token.ID, update); err != nil {
				return "", fmt.Errorf("failed to update token %s: %v", token.TokenID, err)
			}
		}

	}
	return msgToAdd, nil
}
func processUnaccountedForNFTs(ctx context.Context, assets []opensea.Asset, addresses []persist.Address, tokenRepository persist.TokenRepository, contractRepository persist.ContractRepository, userRepository persist.UserRepository, ethcl *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) error {
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

		bs, err := rpc.GetDataFromURI(ctx, asURI, ipfsClient, arweaveClient)
		if err == nil {
			mediaType := persist.SniffMediaType(bs)
			if mediaType != persist.MediaTypeUnknown {
				media = persist.Media{
					MediaURL:     persist.NullString(a.ImageURL),
					ThumbnailURL: persist.NullString(a.ImagePreviewURL),
					MediaType:    mediaType,
				}
			}
		}

		logrus.Debugf("media: %+v", media)

		metadata, _ := rpc.GetMetadataFromURI(ctx, persist.TokenURI(a.TokenMetadataURL), ipfsClient, arweaveClient)

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
