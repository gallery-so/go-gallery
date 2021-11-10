package server

import (
	"github.com/dukex/mixpanel"
	"github.com/gin-gonic/gin"
	shell "github.com/ipfs/go-ipfs-api"
	"github.com/mikeydub/go-gallery/eth"
	"github.com/mikeydub/go-gallery/persist"
)

var requiredNFTs = []persist.TokenID{"0", "1", "2", "3", "4", "5", "6", "7", "8"}

func handlersInit(router *gin.Engine, repos *repositories, ethClient *eth.Client, ipfsClient *shell.Shell, mp mixpanel.Mixpanel) *gin.Engine {

	apiGroupV1 := router.Group("/glry/v1")
	apiGroupV2 := router.Group("/glry/v2")

	nftHandlersInit(apiGroupV1, repos, ethClient, mp)
	tokenHandlersInit(apiGroupV2, repos, ethClient, ipfsClient, mp)

	return router
}

func authHandlersInitToken(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client, mp mixpanel.Mixpanel) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", jwtOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))
	authGroup.GET("/jwt_valid", jwtOptional(), validateJwt())

	// USER

	usersGroup.POST("/login", mixpanelTrack("User Login", nil), login(repos.userRepository, repos.nonceRepository, repos.loginRepository))
	usersGroup.POST("/update/info", mixpanelTrack("User Update Info", []string{analyticsKeyUserUpdateWithBio}), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", mixpanelTrack("User Add Addresses", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), addUserAddress(repos.userRepository, repos.nonceRepository))
	usersGroup.POST("/update/addresses/remove", mixpanelTrack("User Remove Addresses", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), removeAddressesToken(repos.userRepository, repos.collectionTokenRepository))
	usersGroup.GET("/get", jwtOptional(), getUser(repos.userRepository))
	usersGroup.GET("/membership", getMembershipTiers(repos.membershipRepository, repos.userRepository, ethClient))
	usersGroup.POST("/create", mixpanelTrack("User Create", nil), createUserToken(repos.userRepository, repos.nonceRepository, repos.galleryTokenRepository))

}

func authHandlersInitNFT(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client, mp mixpanel.Mixpanel) {

	usersGroup := parent.Group("/users")

	authGroup := parent.Group("/auth")

	// AUTH
	authGroup.GET("/get_preflight", jwtOptional(), getAuthPreflight(repos.userRepository, repos.nonceRepository, ethClient))
	authGroup.GET("/jwt_valid", jwtOptional(), validateJwt())

	// USER

	usersGroup.POST("/login", mixpanelTrack("User Login", nil), login(repos.userRepository, repos.nonceRepository, repos.loginRepository))
	usersGroup.POST("/update/info", mixpanelTrack("User Update Info", []string{analyticsKeyUserUpdateWithBio}), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateUserInfo(repos.userRepository, ethClient))
	usersGroup.POST("/update/addresses/add", mixpanelTrack("User Add Addresses", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), addUserAddress(repos.userRepository, repos.nonceRepository))
	usersGroup.POST("/update/addresses/remove", mixpanelTrack("User Remove Addresses", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), removeAddresses(repos.userRepository, repos.collectionRepository))
	usersGroup.GET("/get", jwtOptional(), getUser(repos.userRepository))
	usersGroup.GET("/membership", getMembershipTiers(repos.membershipRepository, repos.userRepository, ethClient))
	usersGroup.POST("/create", mixpanelTrack("User Create", nil), createUser(repos.userRepository, repos.nonceRepository, repos.galleryRepository))

}

func tokenHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client, ipfsClient *shell.Shell, mp mixpanel.Mixpanel) {

	// AUTH

	authHandlersInitToken(parent, repos, ethClient, mp)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", jwtOptional(), getGalleryByIDToken(repos.galleryTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	galleriesGroup.GET("/user_get", jwtOptional(), getGalleriesByUserIDToken(repos.galleryTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	galleriesGroup.POST("/update", mixpanelTrack("Gallery Update", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateGalleryToken(repos.galleryTokenRepository))

	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", jwtOptional(), getCollectionByIDToken(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	collectionsGroup.GET("/user_get", jwtOptional(), getCollectionsByUserIDToken(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	collectionsGroup.POST("/create", mixpanelTrack("Collection Create", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), createCollectionToken(repos.collectionTokenRepository, repos.galleryTokenRepository))
	collectionsGroup.POST("/delete", mixpanelTrack("Collection Delete", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), deleteCollectionToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/info", mixpanelTrack("Collection Update Info", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateCollectionInfoToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/hidden", mixpanelTrack("Collection Update Hidden", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateCollectionHiddenToken(repos.collectionTokenRepository))
	collectionsGroup.POST("/update/nfts", mixpanelTrack("Collection Update NFTs", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateCollectionTokensToken(repos.collectionTokenRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", jwtOptional(), getTokens(repos.tokenRepository, ipfsClient, ethClient.EthClient))
	nftsGroup.GET("/user_get", jwtOptional(), getTokensForUser(repos.tokenRepository, ipfsClient, ethClient.EthClient))
	nftsGroup.POST("/update", mixpanelTrack("NFT Update", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateTokenByID(repos.tokenRepository))
	nftsGroup.GET("/unassigned/get", mixpanelTrack("NFT Unassigned Get", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), getUnassignedTokensForUser(repos.collectionTokenRepository, repos.tokenRepository, ipfsClient, ethClient.EthClient))
	nftsGroup.POST("/unassigned/refresh", mixpanelTrack("NFT Unassigned Refresh", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), refreshUnassignedTokensForUser(repos.collectionTokenRepository))

	parent.GET("/health", healthcheck())

}

func nftHandlersInit(parent *gin.RouterGroup, repos *repositories, ethClient *eth.Client, mp mixpanel.Mixpanel) {

	// AUTH

	authHandlersInitNFT(parent, repos, ethClient, mp)

	// GALLERIES

	galleriesGroup := parent.Group("/galleries")

	galleriesGroup.GET("/get", jwtOptional(), getGalleryByID(repos.galleryRepository))
	galleriesGroup.GET("/user_get", jwtOptional(), getGalleriesByUserID(repos.galleryRepository))
	galleriesGroup.POST("/update", mixpanelTrack("Gallery Update", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateGallery(repos.galleryRepository, repos.backupRepository))

	// COLLECTIONS

	collectionsGroup := parent.Group("/collections")

	collectionsGroup.GET("/get", jwtOptional(), getCollectionByID(repos.collectionRepository))
	collectionsGroup.GET("/user_get", jwtOptional(), getCollectionsByUserID(repos.collectionRepository))
	collectionsGroup.POST("/create", mixpanelTrack("Collection Create", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), createCollection(repos.collectionRepository, repos.galleryRepository))
	collectionsGroup.POST("/delete", mixpanelTrack("Collection Delete", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), deleteCollection(repos.collectionRepository))
	collectionsGroup.POST("/update/info", mixpanelTrack("Collection Update Info", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateCollectionInfo(repos.collectionRepository))
	collectionsGroup.POST("/update/hidden", mixpanelTrack("Collection Update Hidden", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateCollectionHidden(repos.collectionRepository))
	collectionsGroup.POST("/update/nfts", mixpanelTrack("Collection Update NFTs", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateCollectionNfts(repos.collectionRepository, repos.galleryRepository, repos.backupRepository))

	// NFTS

	nftsGroup := parent.Group("/nfts")

	nftsGroup.GET("/get", jwtOptional(), getNftByID(repos.nftRepository))
	nftsGroup.GET("/user_get", jwtOptional(), getNftsForUser(repos.nftRepository))
	nftsGroup.GET("/opensea/get", mixpanelTrack("NFT Opensea Get", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), getNftsFromOpensea(repos.nftRepository, repos.userRepository, repos.collectionRepository, repos.historyRepository))
	nftsGroup.POST("/opensea/refresh", mixpanelTrack("NFT Opensea Refresh", nil), rateLimited(), jwtRequired(repos.userRepository, ethClient, requiredNFTs), refreshOpenseaNFTs(repos.nftRepository, repos.userRepository))
	nftsGroup.POST("/update", mixpanelTrack("NFT Update", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), updateNftByID(repos.nftRepository))
	nftsGroup.GET("/unassigned/get", mixpanelTrack("NFT Unassigned Get", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), getUnassignedNftsForUser(repos.collectionRepository))
	nftsGroup.POST("/unassigned/refresh", mixpanelTrack("NFT Unassigned Refresh", nil), jwtRequired(repos.userRepository, ethClient, requiredNFTs), refreshUnassignedNftsForUser(repos.collectionRepository))

	parent.GET("/health", healthcheck())

}
