package community

import (
	"context"
	"fmt"

	"github.com/mikeydub/go-gallery/service/nft"
	"github.com/mikeydub/go-gallery/service/persist"
)

type communityData struct {
	Name            string
	Description     string
	ProfileImageURL string
	BannerImageURL  string
	TokenIDRanges   []persist.TokenIDRange
}

// right now these communities are hard coded, if we wanted to have this be dynamic we can get all of the information we need on a collection
// from opensea or the indexed NFTs (indexed NFTs won't have a banner image)
var communities = map[persist.Address]communityData{
	// cryptocoven
	"0x5180db8f5c931aae63c74266b211f580155ecac8": {
		Name:            "CryptoCoven",
		Description:     "it's the season of the witch. 🌙",
		BannerImageURL:  "https://lh3.googleusercontent.com/M42Xf9Vbu_yodzKVFA1I6TYXIx5Hz699gEtp2lDg9vGT7g-S4z_5cx2iYPub1kytnOlexV5WDdGOmpGeuH4-N0CYXi7FaC_iqEm4gQ=h600",
		ProfileImageURL: "https://lh3.googleusercontent.com/E8MVasG7noxC0Fa_duhnexc2xze1PzT1jzyeaHsytOC4722C2Zeo7EhUR8-T6mSem9-4XE5ylrCtoAsceZ_lXez_kTaMufV5pfLc3Fk=s130",
	},
}

// UpdateCommunities updates the communities in the database
func UpdateCommunities(pCtx context.Context, communityRepository persist.CommunityRepository, galleryRepository persist.GalleryRepository, userRepository persist.UserRepository, nftRepository persist.NFTRepository) error {
	done := make(chan error)
	for a := range communities {
		go func(addr persist.Address) {
			if err := UpdateCommunity(pCtx, addr, nftRepository, userRepository, galleryRepository, communityRepository); err != nil {
				done <- err
				return
			}
		}(a)
	}
	for i := 0; i < len(communities); i++ {
		if err := <-done; err != nil {
			return err
		}
	}
	return nil
}

// UpdateCommunity updates a community in the database
func UpdateCommunity(pCtx context.Context, addr persist.Address, nftRepository persist.NFTRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, communityRepository persist.CommunityRepository) error {
	data, ok := communities[addr]
	if !ok {
		return fmt.Errorf("community %s not found", addr)
	}
	community := persist.Community{
		ContractAddress: addr,
		TokenIDRanges:   data.TokenIDRanges,
		Name:            persist.NullString(data.Name),
		Description:     persist.NullString(data.Description),
		ProfileImageURL: persist.NullString(data.ProfileImageURL),
		BannerImageURL:  persist.NullString(data.BannerImageURL),
	}
	owners := make([]persist.CommunityTokenOwner, 0, 50)
	allNFTs, err := nftRepository.GetByContractAddress(pCtx, addr)
	if err != nil {
		return err
	}
	allNFTs = filterValidRanges(data.TokenIDRanges, allNFTs)

	for _, anN := range allNFTs {
		user, err := userRepository.GetByAddress(pCtx, anN.OwnerAddress)
		if err != nil {
			continue
		}
		galleries, err := galleryRepository.GetByUserID(pCtx, user.ID)
		if err != nil {
			continue
		}
	outer:
		for _, gallery := range galleries {
			for _, collection := range gallery.Collections {
				for _, n := range collection.NFTs {
					for _, tokenIDRange := range data.TokenIDRanges {
						if tokenIDRange.Contains(n.OpenseaTokenID) {
							community.Owners = append(owners, persist.CommunityTokenOwner{
								UserID:      user.ID,
								Address:     n.OwnerAddress,
								Username:    user.Username,
								PreviewNFTs: nft.GetPreviewsFromCollections(gallery.Collections),
							})
							break outer
						}
					}
				}
			}
		}
	}
	community.Owners = owners
	err = communityRepository.UpsertByContract(pCtx, addr, community)
	if err != nil {
		return err
	}
	return nil
}

func filterValidRanges(ranges []persist.TokenIDRange, nfts []persist.NFT) []persist.NFT {
	if ranges == nil || len(ranges) == 0 {
		return nfts
	}
	done := make(map[persist.TokenIdentifiers]bool)
	newNFTs := make([]persist.NFT, 0, len(nfts))
	for _, r := range ranges {
		for _, n := range nfts {
			tid := persist.NewTokenIdentifiers(n.Contract.ContractAddress, n.OpenseaTokenID)
			if done[tid] {
				continue
			}
			if r.Contains(n.OpenseaTokenID) {
				done[tid] = true
				newNFTs = append(newNFTs, n)
			}
		}
	}
	return newNFTs

}

// UpdateCommunitiesToken updates the communities in the database
func UpdateCommunitiesToken(pCtx context.Context, communityRepository persist.CommunityRepository, galleryRepository persist.GalleryTokenRepository, userRepository persist.UserRepository, tokenRepository persist.TokenRepository) error {
	done := make(chan error)
	for a := range communities {
		go func(addr persist.Address) {
			if err := UpdateCommunityToken(pCtx, addr, tokenRepository, userRepository, galleryRepository, communityRepository); err != nil {
				done <- err
				return
			}
		}(a)
	}
	for i := 0; i < len(communities); i++ {
		if err := <-done; err != nil {
			return err
		}
	}
	return nil
}

// UpdateCommunityToken updates a community in the database
func UpdateCommunityToken(pCtx context.Context, addr persist.Address, tokenRepository persist.TokenRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryTokenRepository, communityRepository persist.CommunityRepository) error {
	data := communities[addr]
	community := persist.Community{
		ContractAddress: addr,
		TokenIDRanges:   data.TokenIDRanges,
		Name:            persist.NullString(data.Name),
		Description:     persist.NullString(data.Description),
		ProfileImageURL: persist.NullString(data.ProfileImageURL),
		BannerImageURL:  persist.NullString(data.BannerImageURL),
	}
	owners := make([]persist.CommunityTokenOwner, 0, 50)
	allNFTs, err := tokenRepository.GetByContract(pCtx, addr, -1, -1)
	if err != nil {
		return err
	}
	allNFTs = filterValidRangesToken(data.TokenIDRanges, allNFTs)

	for _, anN := range allNFTs {
		user, err := userRepository.GetByAddress(pCtx, anN.OwnerAddress)
		if err != nil {
			continue
		}
		galleries, err := galleryRepository.GetByUserID(pCtx, user.ID)
		if err != nil {
			continue
		}
	outer:
		for _, gallery := range galleries {
			for _, collection := range gallery.Collections {
				for _, n := range collection.NFTs {
					for _, tokenIDRange := range data.TokenIDRanges {
						if tokenIDRange.Contains(n.TokenID) {
							community.Owners = append(owners, persist.CommunityTokenOwner{
								UserID:      user.ID,
								Address:     n.OwnerAddress,
								Username:    user.Username,
								PreviewNFTs: nft.GetPreviewsFromCollectionsToken(gallery.Collections),
							})
							break outer
						}
					}
				}
			}
		}
	}
	community.Owners = owners
	err = communityRepository.UpsertByContract(pCtx, addr, community)
	if err != nil {
		return err
	}
	return nil
}

func filterValidRangesToken(ranges []persist.TokenIDRange, nfts []persist.Token) []persist.Token {
	if ranges == nil || len(ranges) == 0 {
		return nfts
	}
	done := make(map[persist.TokenIdentifiers]bool)
	newNFTs := make([]persist.Token, 0, len(nfts))
	for _, r := range ranges {
		for _, n := range nfts {
			tid := persist.NewTokenIdentifiers(n.ContractAddress, n.TokenID)
			if done[tid] {
				continue
			}
			if r.Contains(n.TokenID) {
				done[tid] = true
				newNFTs = append(newNFTs, n)
			}
		}
	}
	return newNFTs

}
