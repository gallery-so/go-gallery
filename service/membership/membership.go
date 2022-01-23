package membership

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gammazero/workerpool"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// MembershipTierIDs is a list of all membership tiers
var MembershipTierIDs = []persist.TokenID{"4", "3", "5", "6", "8"}

// PremiumCards is the contract address for the premium membership cards
const PremiumCards persist.Address = "0xe01569ca9b39E55Bc7C0dFa09F05fa15CB4C7698"

// UpdateMembershipTiers fetches all membership cards for all token IDs
func UpdateMembershipTiers(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, ethClient *ethclient.Client) ([]persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	defer cancel()
	membershipTiers := make([]persist.MembershipTier, len(MembershipTierIDs))
	tierChan := make(chan persist.MembershipTier)
	for _, v := range MembershipTierIDs {
		events, err := OpenseaFetchMembershipCards(PremiumCards, persist.TokenID(v), 0, 0)
		if err != nil {
			return nil, fmt.Errorf("Failed to fetch membership cards for token: %s, %w", v, err)
		}
		if len(events) == 0 {
			continue
		}
		time.Sleep(time.Second)
		go func(id persist.TokenID, events []opensea.Event) {
			tierChan <- processEvents(ctx, id, events, ethClient, userRepository, galleryRepository, membershipRepository)
		}(v, events)
	}

	for i := 0; i < len(MembershipTierIDs); i++ {
		membershipTiers[i] = <-tierChan
	}
	return membershipTiers, nil
}

// UpdateMembershipTier fetches all membership cards for a token ID
func UpdateMembershipTier(pTokenID persist.TokenID, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, ethClient *ethclient.Client) (persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	events, err := OpenseaFetchMembershipCards(PremiumCards, pTokenID, 0, 0)
	if err != nil {
		return persist.MembershipTier{}, fmt.Errorf("Failed to fetch membership cards for token: %s, %w", pTokenID, err)
	}
	if len(events) == 0 {
		return persist.MembershipTier{}, fmt.Errorf("No membership cards found for token: %s", pTokenID)
	}

	return processEvents(ctx, pTokenID, events, ethClient, userRepository, galleryRepository, membershipRepository), nil
}

// UpdateMembershipTiersToken fetches all membership cards for a token ID
func UpdateMembershipTiersToken(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenRepository, galleryRepository persist.GalleryTokenRepository, ethClient *ethclient.Client) ([]persist.MembershipTier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	membershipTiers := make([]persist.MembershipTier, len(MembershipTierIDs))
	tierChan := make(chan persist.MembershipTier)
	for _, v := range MembershipTierIDs {
		go func(id persist.TokenID) {
			tierChan <- processEventsToken(ctx, id, ethClient, userRepository, nftRepository, galleryRepository, membershipRepository)
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
	events, err := OpenseaFetchMembershipCards(PremiumCards, pTokenID, 0, 0)
	if err != nil {
		return persist.MembershipTier{}, fmt.Errorf("Failed to fetch membership cards for token: %s, %w", pTokenID, err)
	}
	if len(events) == 0 {
		return persist.MembershipTier{}, fmt.Errorf("No membership cards found for token: %s", pTokenID)
	}

	return processEventsToken(ctx, pTokenID, ethClient, userRepository, nftRepository, galleryRepository, membershipRepository), nil
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
			time.Sleep(time.Second * 2)
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

func processEvents(ctx context.Context, id persist.TokenID, events []opensea.Event, ethClient *ethclient.Client, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, membershipRepository persist.MembershipRepository) persist.MembershipTier {
	tier := persist.MembershipTier{
		TokenID:     id,
		LastUpdated: persist.LastUpdatedTime(time.Now()),
	}
	logrus.Infof("Fetching membership tier: %s", id)

	asset := events[0].Asset
	tier.Name = persist.NullString(asset.Name)
	tier.AssetURL = persist.NullString(asset.ImageURL)

	logrus.Infof("Fetched membership cards for token %s with name %s and asset URL %s ", id, tier.Name, tier.AssetURL)
	tier.Owners = make([]persist.MembershipOwner, 0, len(events))

	ownersChan := make(chan persist.MembershipOwner)
	wp := workerpool.New(10)
	for i, e := range events {
		event := e
		f := func() {
			logrus.Debugf("Processing event for ID %s %+v %d", id, event.ToAccount.Address, i)
			if event.ToAccount.Address != persist.ZeroAddress {
				logrus.Debug("Event is to real address")
				membershipOwner := persist.MembershipOwner{Address: event.ToAccount.Address}
				// does to have the NFT?
				hasNFT, _ := eth.HasNFT(ctx, PremiumCards, id, event.ToAccount.Address, ethClient)
				if hasNFT {
					if glryUser, err := userRepository.GetByAddress(ctx, event.ToAccount.Address); err == nil && glryUser.Username != "" {
						membershipOwner.Username = glryUser.Username
						membershipOwner.UserID = glryUser.ID

						galleries, err := galleryRepository.GetByUserID(ctx, glryUser.ID)
						if err == nil && len(galleries) > 0 {
							gallery := galleries[0]
							if gallery.Collections != nil || len(gallery.Collections) > 0 {
								membershipOwner.PreviewNFTs = getPreviewsFromCollections(gallery.Collections)
							}
						}
					}
				}
				logrus.Debugf("Adding membership owner %s for ID %s", membershipOwner.Address, id)
				ownersChan <- membershipOwner
				return
			}
			logrus.Debugf("Event is to 0x0000000000000000000000000000000000000000 for ID %s", id)
			ownersChan <- persist.MembershipOwner{}
		}
		wp.Submit(f)
	}
	receivedOwners := map[persist.Address]bool{}
	receivedUsers := map[string]bool{}
	for i := 0; i < len(events); i++ {
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
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		err := membershipRepository.UpsertByTokenID(ctx, id, tier)
		if err != nil {
			logrus.Errorf("Error upserting membership tier %s: %s", id, err)
		}
	}()
	return tier
}

func processEventsToken(ctx context.Context, id persist.TokenID, ethClient *ethclient.Client, userRepository persist.UserRepository, nftRepository persist.TokenRepository, galleryRepository persist.GalleryTokenRepository, membershipRepository persist.MembershipRepository) persist.MembershipTier {
	tier := persist.MembershipTier{
		TokenID:     id,
		LastUpdated: persist.LastUpdatedTime(time.Now()),
	}
	logrus.Infof("Fetching membership tier: %s", id)

	tokens, err := nftRepository.GetByTokenIdentifiers(ctx, persist.TokenID(id), PremiumCards, -1, 0)
	if err != nil || len(tokens) == 0 {
		logrus.WithError(err).Errorf("Failed to fetch membership cards for token: %s", id)
		return tier
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
		f := func() {
			membershipOwner := persist.MembershipOwner{Address: token.OwnerAddress}
			if glryUser, err := userRepository.GetByAddress(ctx, token.OwnerAddress); err == nil && glryUser.Username != "" {
				membershipOwner.Username = glryUser.Username
				membershipOwner.UserID = glryUser.ID

				galleries, err := galleryRepository.GetByUserID(ctx, glryUser.ID)
				if err == nil && len(galleries) > 0 {
					gallery := galleries[0]
					if gallery.Collections != nil && len(gallery.Collections) > 0 {

						membershipOwner.PreviewNFTs = getPreviewsFromCollectionsToken(gallery.Collections)
					}
				}
			}
			ownersChan <- membershipOwner
		}
		wp.Submit(f)
	}
	for i := 0; i < len(tokens); i++ {
		tier.Owners = append(tier.Owners, <-ownersChan)
	}
	wp.StopWait()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		err := membershipRepository.UpsertByTokenID(ctx, id, tier)
		if err != nil {
			logrus.Errorf("Error upserting membership tier: %s", err)
		}
	}()
	return tier
}

func getPreviewsFromCollections(pColls []persist.Collection) []persist.NullString {
	result := make([]persist.NullString, 0, 3)

outer:
	for _, c := range pColls {
		for _, n := range c.NFTs {
			if n.ImageThumbnailURL != "" {
				result = append(result, n.ImageThumbnailURL)
			}
			if len(result) > 2 {
				break outer
			}
		}
		if len(result) > 2 {
			break outer
		}
	}
	return result

}

func getPreviewsFromCollectionsToken(pColls []persist.CollectionToken) []persist.NullString {
	result := make([]persist.NullString, 0, 3)

outer:
	for _, c := range pColls {
		for _, n := range c.NFTs {
			if n.Media.ThumbnailURL != "" {
				result = append(result, n.Media.ThumbnailURL)
			}
			if len(result) > 2 {
				break outer
			}
		}
		if len(result) > 2 {
			break outer
		}
	}
	return result

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
