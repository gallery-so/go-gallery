package mongodb

const (
	tokenColName = "tokens"
)

// TokenMongoRepository is a repository that stores tokens in a MongoDB database
type TokenMongoRepository struct {
	mp *storage
}

// NewTokenMongoRepository creates a new instance of the collection mongo repository
func NewTokenMongoRepository(mgoClient *mongo.Client) *TokenMongoRepository {
	return &TokenMongoRepository{
		mp: newStorage(mgoClient, 0, galleryDBName, tokenColName),
	}
}

// CreateBulk is a helper function to create multiple nfts in one call and returns
// the ids of each nft created
func (t *TokenMongoRepository) CreateBulk(pCtx context.Context, pERC721s []*persist.Token) ([]persist.DBID, error) {

	nfts := make([]interface{}, len(pERC721s))

	for i, v := range pERC721s {
		nfts[i] = v
	}

	ids, err := t.mp.insertMany(pCtx, nfts)

	if err != nil {
		return nil, err
	}
	return ids, nil
}

// Create inserts a token into the database
func (t *TokenMongoRepository) Create(pCtx context.Context, pERC721 *persist.Token) (persist.DBID, error) {

	return t.mp.insert(pCtx, pERC721)
}

// GetByWallet gets tokens for a given wallet address
func (t *TokenMongoRepository) GetByWallet(pCtx context.Context, pAddress string) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	if pPageNumber > 0 && pMaxCount > 0 {
		opts.SetSkip(int64((pPageNumber - 1) * pMaxCount))
		opts.SetLimit(int64(pMaxCount))
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"owner_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByUserID gets ERC721 tokens for a given userID
func (t *TokenMongoRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"owner_user_id": pUserID}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByContract gets ERC721 tokens for a given contract
func (t *TokenMongoRepository) GetByContract(pCtx context.Context, pAddress string) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	opts.SetSort(bson.M{"last_updated": -1})

	if pPageNumber > 0 && pMaxCount > 0 {
		opts.SetSkip(int64((pPageNumber - 1) * pMaxCount))
		opts.SetLimit(int64(pMaxCount))
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"contract_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByNFTIdentifiers gets tokens for a given contract address and token ID
func (t *TokenMongoRepository) GetByNFTIdentifiers(pCtx context.Context, pTokenID string, pAddress string) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"token_id": pTokenID, "contract_address": strings.ToLower(pAddress)}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetByID gets tokens for a given DB ID
func (t *TokenMongoRepository) GetByID(pCtx context.Context, pID persist.DBID) ([]*persist.Token, error) {
	opts := options.Find()
	if deadline, ok := pCtx.Deadline(); ok {
		dur := time.Until(deadline)
		opts.SetMaxTime(dur)
	}

	result := []*persist.Token{}

	err := t.mp.find(pCtx, bson.M{"_id": pID}, &result, opts)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// BulkUpsert will create a bulk operation on the database to upsert many tokens for a given wallet address
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func (t *TokenMongoRepository) BulkUpsert(pCtx context.Context, pERC721s []*persist.Token) error {

	wg := &sync.WaitGroup{}
	mu := &sync.Mutex{}
	errs := []error{}
	wg.Add(len(pERC721s))

	for _, v := range pERC721s {

		go func(token *persist.Token) {
			defer wg.Done()
			_, err := t.mp.upsert(pCtx, bson.M{"token_id": token.TokenID, "contract_address": strings.ToLower(token.ContractAddress), "owner_address": token.OwnerAddress}, token)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(v)
	}
	wg.Wait()

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Upsert will upsert a token into the database
// This function's primary purpose is to be used when syncing a user's tokens from an external provider
func (t *TokenMongoRepository) Upsert(pCtx context.Context, pToken *persist.Token) error {

	_, err := t.mp.upsert(pCtx, bson.M{"token_id": pToken.TokenID, "contract_address": strings.ToLower(pToken.ContractAddress), "owner_address": pToken.OwnerAddress}, pToken)
	return err
}

// UpdateByID will update a given token by its DB ID and owner user ID
func (t *TokenMongoRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {

	user, err := UserGetByID(pCtx, pUserID, pRuntime)
	if err != nil {
		return err
	}

	return t.mp.update(pCtx, bson.M{"_id": pID, "owner_address": bson.M{"$in": user.Addresses}}, pUpdate)

}

// SniffMediaType will attempt to detect the media type for a given array of bytes
func SniffMediaType(buf []byte) MediaType {
	contentType := http.DetectContentType(buf[:512])
	spl := strings.Split(contentType, "/")

	switch spl[0] {
	case "image":
		switch spl[1] {
		case "svg":
			return MediaTypeSVG
		case "gif":
			return MediaTypeGIF
		default:
			return MediaTypeImage
		}
	case "video":
		return MediaTypeVideo
	case "audio":
		return MediaTypeAudio
	case "text":
		return MediaTypeText
	default:
		return MediaTypeUnknown
	}

}
