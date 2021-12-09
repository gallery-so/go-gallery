package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/middleware"
	"github.com/mikeydub/go-gallery/service/eth"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

type getMembershipTiersResponse struct {
	Tiers []persist.MembershipTier `json:"tiers"`
}

func getMembershipTiers(membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, ethClient *eth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		allTiers, err := membershipRepository.GetAll(c)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		if len(allTiers) > 0 {
			updatedRecently := true
			for _, tier := range allTiers {

				if time.Since(time.Time(tier.LastUpdated)) > time.Hour {
					updatedRecently = false
					break
				}
			}
			if !updatedRecently {
				go updateMembershipTiers(c.Copy(), membershipRepository, userRepository, ethClient)
			}
			c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: allTiers})
			return
		}
		membershipTiers, err := updateMembershipTiers(c, membershipRepository, userRepository, ethClient)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, getMembershipTiersResponse{Tiers: membershipTiers})
	}
}

func updateMembershipTiers(pCtx context.Context, membershipRepository persist.MembershipRepository, userRepository persist.UserRepository, ethClient *eth.Client) ([]persist.MembershipTier, error) {
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
			tier.Owners = []string{}

			ownersChan := make(chan []string)
			for _, e := range events {
				go func(event openseaEvent) {
					owners := []string{}
					hasNFT, _ := ethClient.HasNFT(pCtx, id, event.FromAccount.Address)
					if hasNFT {
						if glryUser, err := userRepository.GetByAddress(pCtx, event.FromAccount.Address); err == nil && glryUser.UserName != "" {
							owners = append(owners, glryUser.UserName)
						} else {
							owners = append(owners, event.FromAccount.Address.String())
						}
					}
					hasNFT, _ = ethClient.HasNFT(pCtx, id, event.ToAccount.Address)
					if hasNFT {
						if glryUser, err := userRepository.GetByAddress(pCtx, event.ToAccount.Address); err == nil && glryUser.UserName != "" {
							owners = append(owners, glryUser.UserName)
						} else {
							owners = append(owners, event.ToAccount.Address.String())
						}
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

// recursively fetches all membership cards for a token ID
func openseaFetchMembershipCards(contractAddress persist.Address, tokenID persist.TokenID, pOffset int) ([]openseaEvent, error) {

	client := &http.Client{
		Timeout: time.Second * 5,
	}

	result := []openseaEvent{}

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

	response := &openseaEvents{}
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
