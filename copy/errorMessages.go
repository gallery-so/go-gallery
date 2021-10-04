package copy

const (
	// InvalidAuthHeader is an error message when the authorization header format is invalid
	// e.g. Authorization ALQojskmoqksdqomq (no Bearer)
	InvalidAuthHeader = "invalid authorization header format"
	// NftIDQueryNotProvided is an error message when the nft id is not provided in request query values
	NftIDQueryNotProvided = "NFT id not provided in query values"
	// CouldNotFindDocument is a mongo error when no document is found from query
	CouldNotFindDocument = "could not find document"
)
