package community

import (
	"context"
	"fmt"
	"math/big"

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

// Communities - right now these Communities are hard coded, if we wanted to have this be dynamic we can get all of the information we need on a collection
// from opensea or the indexed NFTs (indexed NFTs won't have a banner image)
var Communities = map[persist.Address]communityData{
	// cryptocoven
	"0x5180db8f5c931aae63c74266b211f580155ecac8": {
		Name:            "CryptoCoven",
		Description:     "it's the season of the witch. 🌙",
		BannerImageURL:  "https://lh3.googleusercontent.com/M42Xf9Vbu_yodzKVFA1I6TYXIx5Hz699gEtp2lDg9vGT7g-S4z_5cx2iYPub1kytnOlexV5WDdGOmpGeuH4-N0CYXi7FaC_iqEm4gQ=h600",
		ProfileImageURL: "https://lh3.googleusercontent.com/E8MVasG7noxC0Fa_duhnexc2xze1PzT1jzyeaHsytOC4722C2Zeo7EhUR8-T6mSem9-4XE5ylrCtoAsceZ_lXez_kTaMufV5pfLc3Fk=s130",
	},
	// poolsuite exec
	"0xb228d7b6e099618ca71bd5522b3a8c3788a8f172": {
		Name:            "Poolsuite - Executive Member",
		ProfileImageURL: "https://lh3.googleusercontent.com/p2lvRuQZalsroxpmS-q57pGRzyseAzEkLOGGsR6N6tXh_d4x6osxQtZBKqUMRreepnXJcuR80d-9YRIeMy5XnEsPp8aQQcWOyyPqjg=s130",
		BannerImageURL:  "https://lh3.googleusercontent.com/pN8dsrbR5TnTRCtTD7qNqZABzny1JB1tx5xXSSz3XdlovlptmEymNcQ7JE1DZJTZZHEqDg2dzArvcucoUL0dDHdnAMLxU_Nf-gsFSQ=h600",
	},
	// poolsuite member
	"0x123214ef2bb526d1b3fb84a6d448985f537d9763": {
		Name:            "Poolsuite - Pool Member",
		ProfileImageURL: "https://lh3.googleusercontent.com/y8b77O5wH-7pd2jK2eRBDkkJV_SO56FkCPqx3_L6UVdKLFXx1IxqUUYoGSh0m-2jQhyzS4TKdmFPNnsBZlVRQ6rxjLVPaLUOjQS-Gfk=s130",
		BannerImageURL:  "https://lh3.googleusercontent.com/S-Zjma4LRM3_Krpb_Uu_lPEStcOQ5vgolc3yqfafCOzc3o6O1ZKRCmdKwN2Dpg5cIJ54XFP639D11wOQT7dx6rADnsb1AR9f8PNi=h600",
	},
	// poolsuite patron
	"0x364547adfc7a180744d762056df35eeaea803ec4": {
		Name:            "Poolsuite - Patron of the Pool",
		ProfileImageURL: "https://lh3.googleusercontent.com/9U-ga_5kztUukT_6bpEBtyFVbmTM9ywObB7MDbB_9iwnY9ML1cpCh63yeU2H2Q43thMlGuGhNAOZDrpN8ATAQU1Y9O_Ffqprvk-U=s130",
		BannerImageURL:  "https://lh3.googleusercontent.com/MAkRl41geA5QaqEk4EYBEazEUkVZyvzNOQSn63QcX4kKMDKqj-okb23UtF6q5Mw0Ry_bHUyPb_nzuHj-PjRCHE2tT_PEE8Od0O9S=h600",
	},
	"0x13aae6f9599880edbb7d144bb13f1212cee99533": {
		Name:        "The Leggendas",
		Description: "Future pasts, lost civilisations. A reminiscence of the new renaissance. What will remain of us, at the end of time? Who will stand, to tell our story?",
		TokenIDRanges: []persist.TokenIDRange{
			{Start: persist.TokenID(big.NewInt(1000000).Text(16)), End: persist.TokenID(big.NewInt(1000887).Text(16))},
		},
		BannerImageURL:  "https://lh3.googleusercontent.com/l4j8nhI86Jj80fBrhnvHXksaXBr4Ou4yW4mvoqlzC2fsSi-MlE1mErg8mf5PGiUQjUufq2A9A3Mheg0eSnu8i_wxoLP8gVzgmcpMXaw=h600",
		ProfileImageURL: "https://lh3.googleusercontent.com/ParWg5Mn9HltQgoBapkIDzmeG4N2yIYtwdnQuibiscX8FIjFc8bfOeAl7Qt3ccrkU1BAUjeGqyMW3AeKbNMvcQEVET2tIME6fi0-ns8=w192",
	},
}

// UpdateCommunities updates the communities in the database
func UpdateCommunities(pCtx context.Context, communityRepository persist.CommunityRepository, galleryRepository persist.GalleryRepository, userRepository persist.UserRepository, nftRepository persist.NFTRepository) error {
	done := make(chan error)
	for a := range Communities {
		go func(addr persist.Address) {
			if err := UpdateCommunity(pCtx, addr, nftRepository, userRepository, galleryRepository, communityRepository); err != nil {
				done <- err
				return
			}
		}(a)
	}
	for i := 0; i < len(Communities); i++ {
		if err := <-done; err != nil {
			return err
		}
	}
	return nil
}

// UpdateCommunity updates a community in the database
func UpdateCommunity(pCtx context.Context, addr persist.Address, nftRepository persist.NFTRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryRepository, communityRepository persist.CommunityRepository) error {
	data, ok := Communities[addr]
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

	seenUsers := make(map[persist.DBID]bool)
	for _, anN := range allNFTs {
		user, err := userRepository.GetByAddress(pCtx, anN.OwnerAddress)
		if err != nil {
			continue
		}
		if seenUsers[user.ID] {
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
		seenUsers[user.ID] = true
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
	for a := range Communities {
		go func(addr persist.Address) {
			if err := UpdateCommunityToken(pCtx, addr, tokenRepository, userRepository, galleryRepository, communityRepository); err != nil {
				done <- err
				return
			}
		}(a)
	}
	for i := 0; i < len(Communities); i++ {
		if err := <-done; err != nil {
			return err
		}
	}
	return nil
}

// UpdateCommunityToken updates a community in the database
func UpdateCommunityToken(pCtx context.Context, addr persist.Address, tokenRepository persist.TokenRepository, userRepository persist.UserRepository, galleryRepository persist.GalleryTokenRepository, communityRepository persist.CommunityRepository) error {
	data := Communities[addr]
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
