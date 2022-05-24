package membership

import (
	"context"
	"fmt"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/logger"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/multichain/opensea"
	"github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

// MembershipTierIDs is a list of all membership tiers
var MembershipTierIDs = []persist.TokenID{"4", "1", "3", "5", "6", "8"}

// PremiumCards is the contract address for the premium membership cards
const PremiumCards persist.EthereumAddress = "0xe01569ca9b39e55bc7c0dfa09f05fa15cb4c7698"

// UpdateMembershipTiers fetches all membership cards for all token IDs
func UpdateMembershipTiers(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, walletRepository persist.WalletRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) ([]persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()
	membershipTiers := make([]persist.MembershipTier, len(MembershipTierIDs))
	tierChan := make(chan persist.MembershipTier)
	for _, v := range MembershipTierIDs {
		md, err := getTokenMetadata(ctx, v, PremiumCards, ipfsClient, arweaveClient, stg)
		if err != nil {
			return nil, fmt.Errorf("Failed to get token metadata for token: %s, %v", v, err)
		}
		owners, err := getOwnersForToken(ctx, v, PremiumCards)
		if err != nil {
			return nil, fmt.Errorf("Failed to get owners for token: %s, %v", v, err)
		}
		if len(owners) == 0 {
			logger.For(ctx).Errorf("No owners found for token: %s", v)
			continue
		}
		go func(id persist.TokenID, o []persist.EthereumAddress) {
			tier, err := processOwners(ctx, id, md, o, ethClient, userRepository, galleryRepository, membershipRepository, walletRepository)
			if err != nil {
				logger.For(ctx).Errorf("Failed to process membership events for token: %s, %v", id, err)
			}
			tierChan <- tier
		}(v, owners)
	}

	for i := 0; i < len(MembershipTierIDs); i++ {
		membershipTiers[i] = <-tierChan
	}
	return membershipTiers, nil
}

// UpdateMembershipTier fetches all membership cards for a token ID
func UpdateMembershipTier(pTokenID persist.TokenID, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, walletRepository persist.WalletRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := processCurrentTier(ctx, pTokenID, ethClient, userRepository, galleryRepository, membershipRepository, walletRepository)
	if err != nil {
		return persist.MembershipTier{}, fmt.Errorf("Failed to process membership events for token: %s, %v", pTokenID, err)
	}
	md, err := getTokenMetadata(ctx, pTokenID, PremiumCards, ipfsClient, arweaveClient, stg)
	if err != nil {
		return persist.MembershipTier{}, fmt.Errorf("Failed to get token metadata for token: %s, %v", pTokenID, err)
	}
	owners, err := getOwnersForToken(ctx, pTokenID, PremiumCards)
	if err != nil {
		return persist.MembershipTier{}, fmt.Errorf("Failed to get owners for token: %s, %v", pTokenID, err)
	}
	if len(owners) == 0 {
		return persist.MembershipTier{}, fmt.Errorf("No owners found for token: %s", pTokenID)
	}
	return processOwners(ctx, pTokenID, md, owners, ethClient, userRepository, galleryRepository, membershipRepository, walletRepository)
}

// UpdateMembershipTiersToken fetches all membership cards for a token ID
func UpdateMembershipTiersToken(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenGalleryRepository, galleryRepository persist.GalleryTokenRepository, walletRepository persist.WalletRepository, ethClient *ethclient.Client) ([]persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	membershipTiers := make([]persist.MembershipTier, len(MembershipTierIDs))
	tierChan := make(chan persist.MembershipTier)
	for _, v := range MembershipTierIDs {
		go func(id persist.TokenID) {
			tier, err := processEventsToken(ctx, id, ethClient, userRepository, nftRepository, galleryRepository, membershipRepository, walletRepository)
			if err != nil {
				logger.For(ctx).Errorf("Failed to process membership events for token: %s, %v", id, err)
			}
			tierChan <- tier
		}(v)
	}

	for i := 0; i < len(MembershipTierIDs); i++ {
		membershipTiers[i] = <-tierChan
	}
	return membershipTiers, nil
}

// UpdateMembershipTierToken fetches all membership cards for a token ID
func UpdateMembershipTierToken(pTokenID persist.TokenID, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenGalleryRepository, galleryRepository persist.GalleryTokenRepository, walletRepository persist.WalletRepository, ethClient *ethclient.Client) (persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := processCurrentTierToken(ctx, pTokenID, ethClient, userRepository, galleryRepository, membershipRepository, walletRepository)
	if err != nil {
		return persist.MembershipTier{}, fmt.Errorf("Failed to process membership events for token: %s, %v", pTokenID, err)
	}
	events, err := OpenseaFetchMembershipCards(PremiumCards, pTokenID, 0, 0)
	if err != nil {
		return persist.MembershipTier{}, fmt.Errorf("Failed to fetch membership cards for token: %s, %v", pTokenID, err)
	}
	if len(events) == 0 {
		return persist.MembershipTier{}, fmt.Errorf("No membership cards found for token: %s", pTokenID)
	}

	return processEventsToken(ctx, pTokenID, ethClient, userRepository, nftRepository, galleryRepository, membershipRepository, walletRepository)
}

// OpenseaFetchMembershipCards recursively fetches all membership cards for a token ID
func OpenseaFetchMembershipCards(contractAddress persist.EthereumAddress, tokenID persist.TokenID, pOffset int, pRetry int) ([]opensea.Event, error) {

	client := &http.Client{
		Timeout: time.Minute,
	}

	result := []opensea.Event{}

	urlStr := fmt.Sprintf("https://api.opensea.io/api/v1/events?asset_contract_address=%s&token_id=%s&only_opensea=false&offset=%d&limit=50", contractAddress, tokenID.Base10String(), pOffset)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", viper.GetString("OPENSEA_API_KEY"))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		if resp.StatusCode == 429 {
			if pRetry > 3 {
				return nil, fmt.Errorf("timed out fetching membership cards %d at url: %s", tokenID.Base10Int(), urlStr)
			}

			logger.For(nil).Warnf("Opensea API rate limit exceeded, retrying in 5 seconds")
			time.Sleep(time.Second * 2 * time.Duration(pRetry+1))
			return OpenseaFetchMembershipCards(contractAddress, tokenID, pOffset, pRetry+1)
		}
		return nil, fmt.Errorf("unexpected status code: %d - url: %s", resp.StatusCode, urlStr)
	}

	response := &opensea.Events{}
	err = util.UnmarshallBody(response, resp.Body)
	if err != nil {
		return nil, err
	}
	err = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	result = append(result, response.Events...)
	if len(response.Events) == 50 {
		next, err := OpenseaFetchMembershipCards(contractAddress, tokenID, pOffset+50, pRetry)
		if err != nil {
			return nil, err
		}
		result = append(result, next...)
	}

	return result, nil
}

func filterTokenHolders(holdersChannel chan persist.TokenHolder, numHolders int, tokenID persist.TokenID) []persist.TokenHolder {
	receivedWalletIDs := map[persist.DBID]bool{}
	tokenHolderByUserId := map[persist.DBID]*persist.TokenHolder{}

	for i := 0; i < numHolders; i++ {
		owner := <-holdersChannel
		for _, walletID := range owner.WalletIDs {
			if walletID == "" || receivedWalletIDs[walletID] {
				logger.For(nil).Debugf("Skipping duplicate or empty walletID for ID %s: %s", tokenID, walletID)
				continue
			}

			if owner.UserID == "" {
				logger.For(nil).Debugf("Skipping empty userID for ID %s: userID=%s", tokenID, owner.UserID)
				continue
			}

			if existingUser, ok := tokenHolderByUserId[owner.UserID]; ok {
				existingUser.WalletIDs = append(existingUser.WalletIDs, walletID)
				continue
			}

			tokenHolderByUserId[owner.UserID] = &persist.TokenHolder{
				UserID:      owner.UserID,
				WalletIDs:   []persist.DBID{walletID},
				PreviewNFTs: owner.PreviewNFTs,
			}
		}
	}

	filtered := make([]persist.TokenHolder, 0, len(tokenHolderByUserId))
	for _, tokenHolder := range tokenHolderByUserId {
		filtered = append(filtered, *tokenHolder)
	}

	return filtered
}

func processCurrentTier(ctx context.Context, pTokenID persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, membershipRepository persist.MembershipRepository, walletRepository persist.WalletRepository) (persist.MembershipTier, error) {

	tier, err := membershipRepository.GetByTokenID(ctx, pTokenID)
	if err != nil {
		logger.For(ctx).Errorf("Failed to get membership tier for token: %s, %v", pTokenID, err)
		return persist.MembershipTier{}, nil
	}
	wp := workerpool.New(10)
	ownersChan := make(chan persist.TokenHolder)
	for _, v := range tier.Owners {
		owner := v
		wp.Submit(func() {
			owner := fillMembershipOwner(ctx, owner.WalletIDs, pTokenID, ethClient, userRepository, galleryRepository, walletRepository)
			ownersChan <- owner
		})
	}

	tier.Owners = filterTokenHolders(ownersChan, len(tier.Owners), pTokenID)
	wp.StopWait()
	logger.For(ctx).Debugf("Done receiving owners for token %s", pTokenID)

	err = membershipRepository.UpsertByTokenID(ctx, pTokenID, tier)
	if err != nil {
		logger.For(ctx).Errorf("Error upserting membership tier %s: %s", pTokenID, err)
		return persist.MembershipTier{}, err
	}

	return tier, nil
}

func processCurrentTierToken(ctx context.Context, pTokenID persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryTokenRepository, membershipRepository persist.MembershipRepository, walletRepository persist.WalletRepository) (persist.MembershipTier, error) {

	tier, err := membershipRepository.GetByTokenID(ctx, pTokenID)
	if err != nil {
		logger.For(ctx).Errorf("Failed to get membership tier for token: %s, %v", pTokenID, err)
		return persist.MembershipTier{}, nil
	}
	wp := workerpool.New(10)
	ownersChan := make(chan persist.TokenHolder)
	for _, v := range tier.Owners {
		owner := v
		wp.Submit(func() {
			owner := fillMembershipOwnerToken(ctx, owner.WalletIDs, pTokenID, ethClient, userRepository, galleryRepository, walletRepository)
			ownersChan <- owner
		})
	}

	tier.Owners = filterTokenHolders(ownersChan, len(tier.Owners), pTokenID)
	wp.StopWait()
	logger.For(ctx).Debugf("Done receiving owners for token %s", pTokenID)

	err = membershipRepository.UpsertByTokenID(ctx, pTokenID, tier)
	if err != nil {
		logger.For(ctx).Errorf("Error upserting membership tier %s: %s", pTokenID, err)
		return persist.MembershipTier{}, err
	}

	return tier, nil
}

func processOwners(ctx context.Context, id persist.TokenID, metadata alchemyNFTMetadata, owners []persist.EthereumAddress, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, membershipRepository persist.MembershipRepository, walletRepository persist.WalletRepository) (persist.MembershipTier, error) {
	tier := persist.MembershipTier{
		TokenID:     id,
		LastUpdated: persist.LastUpdatedTime(time.Now()),
	}
	logger.For(ctx).Infof("Fetching membership tier: %s", id)

	tier.Name = persist.NullString(metadata.Name)
	tier.AssetURL = persist.NullString(metadata.Image)

	logger.For(ctx).Infof("Fetched membership cards for token %s with name %s and asset URL %s ", id, tier.Name, tier.AssetURL)

	ownersChan := make(chan persist.TokenHolder)
	wp := workerpool.New(10)
	for i, o := range owners {
		addr := o
		wp.Submit(func() {
			logger.For(ctx).Debugf("Processing event for ID %s %+v %d", id, addr, i)
			if addr.String() != persist.ZeroAddress.String() {
				logger.For(ctx).Debug("Event is to real address")
				// does to have the NFT?
				wallet, err := walletRepository.GetByChainAddress(ctx, persist.ChainAddress{Address: persist.Address(addr), Chain: persist.ChainETH})
				if err != nil {
					logger.For(ctx).Debugf("Skipping membership owner %s for ID %s: no wallet found for address", addr, id)
					ownersChan <- persist.TokenHolder{}
					return
				}
				membershipOwner := fillMembershipOwner(ctx, []persist.DBID{wallet.ID}, id, ethClient, userRepository, galleryRepository, walletRepository)
				if membershipOwner.PreviewNFTs != nil && len(membershipOwner.PreviewNFTs) > 0 {
					logger.For(ctx).Debugf("Adding membership owner %s for ID %s", addr, id)
					ownersChan <- membershipOwner
				} else {
					logger.For(ctx).Debugf("Skipping membership owner %s for ID %s", addr, id)
					ownersChan <- persist.TokenHolder{}
				}
				return
			}
			logger.For(ctx).Debugf("Event is to 0x0000000000000000000000000000000000000000 for ID %s", id)
			ownersChan <- persist.TokenHolder{}
		})

	}

	tier.Owners = filterTokenHolders(ownersChan, len(owners), id)
	wp.StopWait()
	logger.For(ctx).Debugf("Done receiving owners for token %s", id)

	err := membershipRepository.UpsertByTokenID(ctx, id, tier)
	if err != nil {
		logger.For(ctx).Errorf("Error upserting membership tier %s: %s", id, err)
		return persist.MembershipTier{}, err
	}

	return tier, nil
}

func fillMembershipOwner(ctx context.Context, pWalletIDs []persist.DBID, id persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, walletRepository persist.WalletRepository) persist.TokenHolder {
	membershipOwner := persist.TokenHolder{WalletIDs: pWalletIDs}

	for _, walletID := range pWalletIDs {
		glryUser, err := userRepository.GetByWallet(ctx, walletID)
		if err != nil || glryUser.Username == "" {
			logger.For(ctx).WithError(err).Errorf("Failed to get user for address %s", walletID)
			continue
		}

		membershipOwner.UserID = glryUser.ID

		galleries, err := galleryRepository.GetByUserID(ctx, glryUser.ID)
		if err == nil && len(galleries) > 0 {
			gallery := galleries[0]
			if gallery.Collections != nil && len(gallery.Collections) > 0 {
				membershipOwner.PreviewNFTs = nft.GetPreviewsFromCollections(gallery.Collections)
			}
		}

		return membershipOwner
	}

	return membershipOwner
}

func fillMembershipOwnerToken(ctx context.Context, pWalletIDs []persist.DBID, id persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryTokenRepository, walletRepository persist.WalletRepository) persist.TokenHolder {
	membershipOwner := persist.TokenHolder{WalletIDs: pWalletIDs}

	for _, walletID := range pWalletIDs {
		wallet, err := walletRepository.GetByID(ctx, walletID)
		if err != nil {
			logger.For(ctx).WithError(err).Errorf("Failed to get wallet with ID %s", walletID)
			continue
		}

		if wallet.Chain != persist.ChainETH {
			continue
		}

		if hasNFT, _ := eth.HasNFT(ctx, PremiumCards, id, persist.EthereumAddress(wallet.Address), ethClient); !hasNFT {
			continue
		}

		glryUser, err := userRepository.GetByWallet(ctx, walletID)
		if err != nil || glryUser.Username == "" {
			logger.For(ctx).WithError(err).Errorf("Failed to get user for walletID %s", walletID)
			continue
		}

		membershipOwner.UserID = glryUser.ID

		galleries, err := galleryRepository.GetByUserID(ctx, glryUser.ID)
		if err == nil && len(galleries) > 0 {
			gallery := galleries[0]
			if gallery.Collections != nil || len(gallery.Collections) > 0 {
				membershipOwner.PreviewNFTs = nft.GetPreviewsFromCollectionsToken(gallery.Collections)
			}
		}

		return membershipOwner
	}

	return membershipOwner
}

func processEventsToken(ctx context.Context, id persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, nftRepository persist.TokenGalleryRepository, galleryRepository persist.GalleryTokenRepository, membershipRepository persist.MembershipRepository, walletRepository persist.WalletRepository) (persist.MembershipTier, error) {
	tier := persist.MembershipTier{
		TokenID:     id,
		LastUpdated: persist.LastUpdatedTime(time.Now()),
	}
	logger.For(ctx).Infof("Fetching membership tier: %s", id)

	tokens, err := nftRepository.GetByTokenIdentifiers(ctx, persist.TokenID(id), persist.Address(PremiumCards), persist.ChainETH, -1, 0)
	if err != nil || len(tokens) == 0 {
		logger.For(ctx).WithError(err).Errorf("Failed to fetch membership cards for token: %s", id)
		return tier, nil
	}
	initialToken := tokens[0]

	tier.Name = persist.NullString(initialToken.Name)
	tier.AssetURL = persist.NullString(initialToken.Media.MediaURL)

	logger.For(ctx).Infof("Fetched membership cards for token %s with name %s and asset URL %s ", id, tier.Name, tier.AssetURL)

	ownersChan := make(chan persist.TokenHolder)
	wp := workerpool.New(10)
	for _, t := range tokens {
		token := t
		wp.Submit(func() {
			walletIDs := make([]persist.DBID, len(token.OwnerAddresses))
			for i, w := range token.OwnerAddresses {
				walletIDs[i] = w.ID
			}
			membershipOwner := fillMembershipOwnerToken(ctx, walletIDs, id, ethClient, userRepository, galleryRepository, walletRepository)
			if membershipOwner.PreviewNFTs != nil && len(membershipOwner.PreviewNFTs) > 0 {
				ownersChan <- membershipOwner
			} else {
				ownersChan <- persist.TokenHolder{}
			}
		})

	}

	tier.Owners = filterTokenHolders(ownersChan, len(tokens), id)
	wp.StopWait()

	err = membershipRepository.UpsertByTokenID(ctx, id, tier)
	if err != nil {
		logger.For(ctx).Errorf("Error upserting membership tier: %s", err)
		return persist.MembershipTier{}, err
	}

	return tier, nil
}

// OrderMembershipTiers orders the membership tiers in the desired order determined for the membership page
func OrderMembershipTiers(pTiers []persist.MembershipTier) []persist.MembershipTier {
	result := make([]persist.MembershipTier, 0, len(pTiers))
	for _, v := range MembershipTierIDs {
		for _, t := range pTiers {
			if t.TokenID == v {
				result = append(result, t)
			}
		}
	}
	return result
}

// GetMembershipTiers returns the most recent membership tiers and potentially updates tiers
func GetMembershipTiers(ctx context.Context, forceRefresh bool, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository,
	galleryRepository persist.GalleryRepository, walletRepository persist.WalletRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) ([]persist.MembershipTier, error) {

	if forceRefresh {
		logger.For(ctx).Infof("Force refresh - updating membership tiers")
	}

	allTiers, err := membershipRepository.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	logger.For(ctx).Debugf("Found %d membership tiers in the DB", len(allTiers))

	if len(allTiers) > 0 {
		if len(allTiers) != len(MembershipTierIDs) {
			tiers := make(map[persist.TokenID]bool)

			for _, tier := range allTiers {
				tiers[tier.TokenID] = true
			}

			for _, tierID := range MembershipTierIDs {
				if ok := tiers[tierID]; !ok {
					logger.For(ctx).Infof("Tier not found - updating membership tier %s", tierID)
					newTier, err := UpdateMembershipTier(tierID, membershipRepository, userRepository, galleryRepository, walletRepository, ethClient, ipfsClient, arweaveClient, stg)
					if err != nil {
						return nil, err
					}
					allTiers = append(allTiers, newTier)
				}
			}
		}

		tiersToUpdate := make([]persist.TokenID, 0, len(allTiers))
		for _, tier := range allTiers {
			if time.Since(tier.LastUpdated.Time()) > time.Hour || forceRefresh {
				logger.For(ctx).Infof("Tier %s not updated in the last hour - updating membership tier", tier.TokenID)
				tiersToUpdate = append(tiersToUpdate, tier.TokenID)
			}
		}

		if len(tiersToUpdate) > 0 {
			go func() {
				for _, tierID := range tiersToUpdate {
					_, err := UpdateMembershipTier(tierID, membershipRepository, userRepository, galleryRepository, walletRepository, ethClient, ipfsClient, arweaveClient, stg)
					if err != nil {
						logger.For(ctx).WithError(err).Errorf("Failed to update membership tier %s", tierID)
					}
				}
			}()
		}

		return OrderMembershipTiers(allTiers), nil
	}

	logger.For(ctx).Infof("No tiers found - updating membership tiers")
	membershipTiers, err := UpdateMembershipTiers(membershipRepository, userRepository, galleryRepository, walletRepository, ethClient, ipfsClient, arweaveClient, stg)
	if err != nil {
		return nil, err
	}

	return OrderMembershipTiers(membershipTiers), nil
}
