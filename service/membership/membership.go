package membership

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/opensea"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

// UpdateMembershipTiers fetches all membership cards for a token ID
func UpdateMembershipTiers(pCtx context.Context, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.NFTRepository, ethClient *eth.Client) ([]persist.MembershipTier, error) {
	membershipTiers := make([]persist.MembershipTier, len(middleware.RequiredNFTs))
	tierChan := make(chan persist.MembershipTier)
	for _, v := range middleware.RequiredNFTs {
		go func(id persist.TokenID) {
			tier := persist.MembershipTier{
				TokenID: id,
			}

			events, err := openseaFetchMembershipCards(persist.Address(viper.GetString("CONTRACT_ADDRESS")), persist.TokenID(id), 0)
			if err != nil || len(events) == 0 {
				tierChan <- tier
				return
			}
			asset := events[0].Asset
			tier.Name = asset.Name
			tier.AssetURL = asset.ImageURL
			tier.Owners = make([]persist.MembershipOwner, 0, len(events)*2)

			ownersChan := make(chan []persist.MembershipOwner)
			for _, e := range events {
				go func(event opensea.Event) {
					owners := make([]persist.MembershipOwner, 0, 2)
					hasNFT, _ := ethClient.HasNFT(pCtx, id, event.FromAccount.Address)
					if hasNFT {
						membershipOwner := persist.MembershipOwner{}
						if glryUser, err := userRepository.GetByAddress(pCtx, event.FromAccount.Address); err == nil && glryUser.UserName != "" {
							membershipOwner.Username = glryUser.UserName
							membershipOwner.UserID = glryUser.ID
							membershipOwner.Address = event.FromAccount.Address

							nfts, err := nftRepository.GetByUserID(pCtx, glryUser.ID)
							if err == nil && len(nfts) > 0 {
								nftURLs := make([]string, 0, 3)
								for i, nft := range nfts {
									if i == 3 {
										break
									}
									if nft.ImagePreviewURL != "" {
										nftURLs = append(nftURLs, nft.ImagePreviewURL)
									} else if nft.ImageURL != "" {
										nftURLs = append(nftURLs, nft.ImageURL)
									} else {
										i--
										continue
									}
								}
								membershipOwner.PreviewNFTs = nftURLs
							}
						} else {
							membershipOwner.Address = event.FromAccount.Address
						}
						owners = append(owners, membershipOwner)
					}
					hasNFT, _ = ethClient.HasNFT(pCtx, id, event.ToAccount.Address)
					if hasNFT {
						membershipOwner := persist.MembershipOwner{}
						if glryUser, err := userRepository.GetByAddress(pCtx, event.ToAccount.Address); err == nil && glryUser.UserName != "" {
							membershipOwner.Username = glryUser.UserName
							membershipOwner.UserID = glryUser.ID
							membershipOwner.Address = event.FromAccount.Address

							nfts, err := nftRepository.GetByUserID(pCtx, glryUser.ID)
							if err == nil && len(nfts) > 0 {
								nftURLs := make([]string, 0, 3)
								for i, nft := range nfts {
									if i == 3 {
										break
									}
									if nft.ImagePreviewURL != "" {
										nftURLs = append(nftURLs, nft.ImagePreviewURL)
									} else if nft.ImageURL != "" {
										nftURLs = append(nftURLs, nft.ImageURL)
									} else {
										i--
										continue
									}
								}
								membershipOwner.PreviewNFTs = nftURLs
							}
						} else {
							membershipOwner.Address = event.FromAccount.Address
						}
						owners = append(owners, membershipOwner)
					}
					ownersChan <- owners
				}(e)
			}
			for i := 0; i < len(events); i++ {
				incomingOwners := <-ownersChan
				tier.Owners = append(tier.Owners, incomingOwners...)
			}
			go membershipRepository.UpsertByTokenID(pCtx, id, tier)
			tierChan <- tier
		}(v)
	}

	for i := 0; i < len(middleware.RequiredNFTs); i++ {
		membershipTiers[i] = <-tierChan
	}
	return membershipTiers, nil
}

// UpdateMembershipTiersToken fetches all membership cards for a token ID
func UpdateMembershipTiersToken(pCtx context.Context, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, nftRepository persist.TokenRepository, ethClient *eth.Client) ([]persist.MembershipTier, error) {
	membershipTiers := make([]persist.MembershipTier, len(middleware.RequiredNFTs))
	tierChan := make(chan persist.MembershipTier)
	for _, v := range middleware.RequiredNFTs {
		go func(id persist.TokenID) {
			tier := persist.MembershipTier{
				TokenID: id,
			}

			tokens, err := nftRepository.GetByTokenIdentifiers(pCtx, persist.TokenID(id), persist.Address(viper.GetString("CONTRACT_ADDRESS")), -1, 0)
			if err != nil || len(tokens) == 0 {
				tierChan <- tier
				return
			}
			initialToken := tokens[0]

			tier.Name = initialToken.Name
			tier.AssetURL = initialToken.Media.MediaURL
			tier.Owners = make([]persist.MembershipOwner, 0, len(tokens))

			ownersChan := make(chan persist.MembershipOwner)
			for _, e := range tokens {
				go func(token persist.Token) {
					membershipOwner := persist.MembershipOwner{}
					if glryUser, err := userRepository.GetByAddress(pCtx, token.OwnerAddress); err == nil && glryUser.UserName != "" {
						membershipOwner.Username = glryUser.UserName
						membershipOwner.UserID = glryUser.ID
						membershipOwner.Address = token.OwnerAddress

						nfts, err := nftRepository.GetByUserID(pCtx, glryUser.ID, -1, 0)
						if err == nil && len(nfts) > 0 {
							nftURLs := make([]string, 0, 3)
							for i, nft := range nfts {
								if i == 3 {
									break
								}
								if nft.Media.PreviewURL != "" {
									nftURLs = append(nftURLs, nft.Media.PreviewURL)
								} else if nft.Media.MediaURL != "" {
									nftURLs = append(nftURLs, nft.Media.MediaURL)
								} else {
									i--
									continue
								}
							}
							membershipOwner.PreviewNFTs = nftURLs
						}
					} else {
						membershipOwner.Address = token.OwnerAddress
					}
					ownersChan <- membershipOwner
				}(e)
			}
			for i := 0; i < len(tokens); i++ {
				incomingOwner := <-ownersChan
				tier.Owners = append(tier.Owners, incomingOwner)
			}
			go membershipRepository.UpsertByTokenID(pCtx, id, tier)
			tierChan <- tier
		}(v)
	}

	for i := 0; i < len(middleware.RequiredNFTs); i++ {
		membershipTiers[i] = <-tierChan
	}
	return membershipTiers, nil
}

// recursively fetches all membership cards for a token ID
func openseaFetchMembershipCards(contractAddress persist.Address, tokenID persist.TokenID, pOffset int) ([]opensea.Event, error) {

	client := &http.Client{
		Timeout: time.Second * 5,
	}

	result := []opensea.Event{}

	urlStr := fmt.Sprintf("https://api.opensea.io/api/v1/events?asset_contract_address=%s&token_id=%s&only_opensea=false&offset=%d&limit=100", contractAddress, tokenID.Base10String(), pOffset)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code: %d - url: %s", resp.StatusCode, urlStr)
	}

	response := &opensea.Events{}
	err = util.UnmarshallBody(response, resp.Body)
	if err != nil {
		return nil, err
	}
	result = append(result, response.Events...)
	if len(response.Events) == 100 {
		next, err := openseaFetchMembershipCards(contractAddress, tokenID, pOffset+100)
		if err != nil {
			return nil, err
		}
		result = append(result, next...)
	}

	return result, nil
}
