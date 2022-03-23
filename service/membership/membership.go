package membership

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/everFinance/goar"
	"github.com/gammazero/workerpool"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// MembershipTierIDs is a list of all membership tiers
var MembershipTierIDs = []persist.TokenID{"4", "1", "3", "5", "6", "8"}

// PremiumCards is the contract address for the premium membership cards
const PremiumCards persist.Address = "0xe01569ca9b39e55bc7c0dfa09f05fa15cb4c7698"

// UpdateMembershipTiers fetches all membership cards for all token IDs
func UpdateMembershipTiers(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) ([]persist.MembershipTier, error) {
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
			logrus.Errorf("No owners found for token: %s", v)
			continue
		}
		go func(id persist.TokenID, o []persist.Address) {
			tier, err := processOwners(ctx, id, md, o, ethClient, userRepository, galleryRepository, membershipRepository)
			if err != nil {
				logrus.Errorf("Failed to process membership events for token: %s, %v", id, err)
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
func UpdateMembershipTier(pTokenID persist.TokenID, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) (persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := processCurrentTier(ctx, pTokenID, ethClient, userRepository, galleryRepository, membershipRepository)
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
	return processOwners(ctx, pTokenID, md, owners, ethClient, userRepository, galleryRepository, membershipRepository)
}

// UpdateMembershipTiersToken fetches all membership cards for a token ID
func UpdateMembershipTiersToken(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenRepository, galleryRepository persist.GalleryTokenRepository, ethClient *ethclient.Client) ([]persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	membershipTiers := make([]persist.MembershipTier, len(MembershipTierIDs))
	tierChan := make(chan persist.MembershipTier)
	for _, v := range MembershipTierIDs {
		go func(id persist.TokenID) {
			tier, err := processEventsToken(ctx, id, ethClient, userRepository, nftRepository, galleryRepository, membershipRepository)
			if err != nil {
				logrus.Errorf("Failed to process membership events for token: %s, %v", id, err)
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
func UpdateMembershipTierToken(pTokenID persist.TokenID, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenRepository, galleryRepository persist.GalleryTokenRepository, ethClient *ethclient.Client) (persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := processCurrentTierToken(ctx, pTokenID, ethClient, userRepository, galleryRepository, membershipRepository)
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

	return processEventsToken(ctx, pTokenID, ethClient, userRepository, nftRepository, galleryRepository, membershipRepository)
}

// OpenseaFetchMembershipCards recursively fetches all membership cards for a token ID
func OpenseaFetchMembershipCards(contractAddress persist.Address, tokenID persist.TokenID, pOffset int, pRetry int) ([]opensea.Event, error) {

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

			logrus.Warnf("Opensea API rate limit exceeded, retrying in 5 seconds")
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

func processCurrentTier(ctx context.Context, pTokenID persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, membershipRepository persist.MembershipRepository) (persist.MembershipTier, error) {

	tier, err := membershipRepository.GetByTokenID(ctx, pTokenID)
	if err != nil {
		logrus.Errorf("Failed to get membership tier for token: %s, %v", pTokenID, err)
		return persist.MembershipTier{}, nil
	}
	wp := workerpool.New(10)
	ownersChan := make(chan persist.MembershipOwner)
	for _, v := range tier.Owners {
		owner := v
		wp.Submit(func() {
			owner := fillMembershipOwner(ctx, owner.Address, pTokenID, ethClient, userRepository, galleryRepository)
			ownersChan <- owner
		})
	}
	receivedOwners := map[persist.Address]bool{}
	receivedUsers := map[string]bool{}
	newOwners := make([]persist.MembershipOwner, 0, len(tier.Owners))
	for i := 0; i < len(tier.Owners); i++ {
		owner := <-ownersChan
		if receivedOwners[owner.Address] || owner.Address == "" {
			logrus.Debugf("Skipping duplicate or empty owner for ID %s: %s", pTokenID, owner.Address)
			continue
		}
		if owner.Username != "" && receivedUsers[strings.ToLower(owner.Username.String())] {
			logrus.Debugf("Skipping duplicate username for ID %s: %s", pTokenID, owner.Username)
			continue
		}
		newOwners = append(newOwners, owner)
		receivedOwners[owner.Address] = true
		if owner.Username != "" {
			receivedUsers[strings.ToLower(owner.Username.String())] = true
		}
	}
	tier.Owners = newOwners
	wp.StopWait()
	logrus.Debugf("Done receiving owners for token %s", pTokenID)

	err = membershipRepository.UpsertByTokenID(ctx, pTokenID, tier)
	if err != nil {
		logrus.Errorf("Error upserting membership tier %s: %s", pTokenID, err)
		return persist.MembershipTier{}, err
	}

	return tier, nil
}

func processCurrentTierToken(ctx context.Context, pTokenID persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryTokenRepository, membershipRepository persist.MembershipRepository) (persist.MembershipTier, error) {

	tier, err := membershipRepository.GetByTokenID(ctx, pTokenID)
	if err != nil {
		logrus.Errorf("Failed to get membership tier for token: %s, %v", pTokenID, err)
		return persist.MembershipTier{}, nil
	}
	wp := workerpool.New(10)
	ownersChan := make(chan persist.MembershipOwner)
	for _, v := range tier.Owners {
		owner := v
		wp.Submit(func() {
			owner := fillMembershipOwnerToken(ctx, owner.Address, pTokenID, ethClient, userRepository, galleryRepository)
			ownersChan <- owner
		})
	}
	receivedOwners := map[persist.Address]bool{}
	receivedUsers := map[string]bool{}
	newOwners := make([]persist.MembershipOwner, 0, len(tier.Owners))
	for i := 0; i < len(tier.Owners); i++ {
		owner := <-ownersChan
		if receivedOwners[owner.Address] || owner.Address == "" {
			logrus.Debugf("Skipping duplicate or empty owner for ID %s: %s", pTokenID, owner.Address)
			continue
		}
		if owner.Username != "" && receivedUsers[strings.ToLower(owner.Username.String())] {
			logrus.Debugf("Skipping duplicate username for ID %s: %s", pTokenID, owner.Username)
			continue
		}
		newOwners = append(newOwners, owner)
		receivedOwners[owner.Address] = true
		if owner.Username != "" {
			receivedUsers[strings.ToLower(owner.Username.String())] = true
		}
	}
	tier.Owners = newOwners
	wp.StopWait()
	logrus.Debugf("Done receiving owners for token %s", pTokenID)

	err = membershipRepository.UpsertByTokenID(ctx, pTokenID, tier)
	if err != nil {
		logrus.Errorf("Error upserting membership tier %s: %s", pTokenID, err)
		return persist.MembershipTier{}, err
	}

	return tier, nil
}

func processOwners(ctx context.Context, id persist.TokenID, metadata alchemyNFTMetadata, owners []persist.Address, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, membershipRepository persist.MembershipRepository) (persist.MembershipTier, error) {
	tier := persist.MembershipTier{
		TokenID:     id,
		LastUpdated: persist.LastUpdatedTime(time.Now()),
	}
	logrus.Infof("Fetching membership tier: %s", id)

	tier.Name = persist.NullString(metadata.Name)
	tier.AssetURL = persist.NullString(metadata.Image)

	logrus.Infof("Fetched membership cards for token %s with name %s and asset URL %s ", id, tier.Name, tier.AssetURL)
	tier.Owners = make([]persist.MembershipOwner, 0, len(owners))

	ownersChan := make(chan persist.MembershipOwner)
	wp := workerpool.New(10)
	for i, o := range owners {
		addr := o
		wp.Submit(func() {
			logrus.Debugf("Processing event for ID %s %+v %d", id, addr, i)
			if addr.String() != persist.ZeroAddress.String() {
				logrus.Debug("Event is to real address")
				// does to have the NFT?
				membershipOwner := fillMembershipOwner(ctx, addr, id, ethClient, userRepository, galleryRepository)
				logrus.Debugf("Adding membership owner %s for ID %s", membershipOwner.Address, id)
				ownersChan <- membershipOwner
				return
			}
			logrus.Debugf("Event is to 0x0000000000000000000000000000000000000000 for ID %s", id)
			ownersChan <- persist.MembershipOwner{}
		})

	}
	receivedOwners := map[persist.Address]bool{}
	receivedUsers := map[string]bool{}
	for i := 0; i < len(owners); i++ {
		owner := <-ownersChan
		if receivedOwners[owner.Address] || owner.Address == "" {
			logrus.Debugf("Skipping duplicate or empty owner for ID %s: %s", id, owner.Address)
			continue
		}
		if owner.Username != "" && receivedUsers[strings.ToLower(owner.Username.String())] {
			logrus.Debugf("Skipping duplicate username for ID %s: %s", id, owner.Username)
			continue
		}
		tier.Owners = append(tier.Owners, owner)
		receivedOwners[owner.Address] = true
		if owner.Username != "" {
			receivedUsers[strings.ToLower(owner.Username.String())] = true
		}
	}
	wp.StopWait()
	logrus.Debugf("Done receiving owners for token %s", id)

	err := membershipRepository.UpsertByTokenID(ctx, id, tier)
	if err != nil {
		logrus.Errorf("Error upserting membership tier %s: %s", id, err)
		return persist.MembershipTier{}, err
	}

	return tier, nil
}

func fillMembershipOwner(ctx context.Context, pAddress persist.Address, id persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository) persist.MembershipOwner {
	membershipOwner := persist.MembershipOwner{Address: pAddress}

	hasNFT, _ := eth.HasNFT(ctx, PremiumCards, id, pAddress, ethClient)
	if hasNFT {
		glryUser, err := userRepository.GetByAddress(ctx, pAddress)
		if err != nil || glryUser.Username == "" {
			logrus.WithError(err).Errorf("Failed to get user for address %s", pAddress)
			return membershipOwner
		}
		membershipOwner.Username = glryUser.Username
		membershipOwner.UserID = glryUser.ID

		galleries, err := galleryRepository.GetByUserID(ctx, glryUser.ID)
		if err == nil && len(galleries) > 0 {
			gallery := galleries[0]
			if gallery.Collections != nil || len(gallery.Collections) > 0 {
				membershipOwner.PreviewNFTs = nft.GetPreviewsFromCollections(gallery.Collections)
			}
		}

	}
	return membershipOwner
}

func fillMembershipOwnerToken(ctx context.Context, pAddress persist.Address, id persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryTokenRepository) persist.MembershipOwner {
	membershipOwner := persist.MembershipOwner{Address: pAddress}

	hasNFT, _ := eth.HasNFT(ctx, PremiumCards, id, pAddress, ethClient)
	if hasNFT {
		glryUser, err := userRepository.GetByAddress(ctx, pAddress)
		if err != nil || glryUser.Username == "" {
			logrus.WithError(err).Errorf("Failed to get user for address %s", pAddress)
			return membershipOwner
		}
		membershipOwner.Username = glryUser.Username
		membershipOwner.UserID = glryUser.ID

		galleries, err := galleryRepository.GetByUserID(ctx, glryUser.ID)
		if err == nil && len(galleries) > 0 {
			gallery := galleries[0]
			if gallery.Collections != nil || len(gallery.Collections) > 0 {
				membershipOwner.PreviewNFTs = nft.GetPreviewsFromCollectionsToken(gallery.Collections)
			}
		}
	}
	return membershipOwner
}

func processEventsToken(ctx context.Context, id persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, nftRepository persist.TokenRepository, galleryRepository persist.GalleryTokenRepository, membershipRepository persist.MembershipRepository) (persist.MembershipTier, error) {
	tier := persist.MembershipTier{
		TokenID:     id,
		LastUpdated: persist.LastUpdatedTime(time.Now()),
	}
	logrus.Infof("Fetching membership tier: %s", id)

	tokens, err := nftRepository.GetByTokenIdentifiers(ctx, persist.TokenID(id), PremiumCards, -1, 0)
	if err != nil || len(tokens) == 0 {
		logrus.WithError(err).Errorf("Failed to fetch membership cards for token: %s", id)
		return tier, nil
	}
	initialToken := tokens[0]

	tier.Name = persist.NullString(initialToken.Name)
	tier.AssetURL = persist.NullString(initialToken.Media.MediaURL)

	logrus.Infof("Fetched membership cards for token %s with name %s and asset URL %s ", id, tier.Name, tier.AssetURL)

	tier.Owners = make([]persist.MembershipOwner, 0, len(tokens))

	ownersChan := make(chan persist.MembershipOwner)
	wp := workerpool.New(10)
	for _, t := range tokens {
		token := t
		wp.Submit(func() {
			membershipOwner := fillMembershipOwnerToken(ctx, token.OwnerAddress, id, ethClient, userRepository, galleryRepository)
			ownersChan <- membershipOwner
		})

	}
	for i := 0; i < len(tokens); i++ {
		tier.Owners = append(tier.Owners, <-ownersChan)
	}
	wp.StopWait()

	err = membershipRepository.UpsertByTokenID(ctx, id, tier)
	if err != nil {
		logrus.Errorf("Error upserting membership tier: %s", err)
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
	galleryRepository persist.GalleryRepository, ethClient *ethclient.Client, ipfsClient *shell.Shell, arweaveClient *goar.Client, stg *storage.Client) ([]persist.MembershipTier, error) {

	if forceRefresh {
		logrus.Infof("Force refresh - updating membership tiers")
	}

	allTiers, err := membershipRepository.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("Found %d membership tiers in the DB", len(allTiers))

	if len(allTiers) > 0 {
		if len(allTiers) != len(MembershipTierIDs) {
			tiers := make(map[persist.TokenID]bool)

			for _, tier := range allTiers {
				tiers[tier.TokenID] = true
			}

			for _, tierID := range MembershipTierIDs {
				if ok := tiers[tierID]; !ok {
					logrus.Infof("Tier not found - updating membership tier %s", tierID)
					newTier, err := UpdateMembershipTier(tierID, membershipRepository, userRepository, galleryRepository, ethClient, ipfsClient, arweaveClient, stg)
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
				logrus.Infof("Tier %s not updated in the last hour - updating membership tier", tier.TokenID)
				tiersToUpdate = append(tiersToUpdate, tier.TokenID)
			}
		}

		if len(tiersToUpdate) > 0 {
			go func() {
				for _, tierID := range tiersToUpdate {
					_, err := UpdateMembershipTier(tierID, membershipRepository, userRepository, galleryRepository, ethClient, ipfsClient, arweaveClient, stg)
					if err != nil {
						logrus.WithError(err).Errorf("Failed to update membership tier %s", tierID)
					}
				}
			}()
		}

		return OrderMembershipTiers(allTiers), nil
	}

	logrus.Infof("No tiers found - updating membership tiers")
	membershipTiers, err := UpdateMembershipTiers(membershipRepository, userRepository, galleryRepository, ethClient, ipfsClient, arweaveClient, stg)
	if err != nil {
		return nil, err
	}

	return OrderMembershipTiers(membershipTiers), nil
}
