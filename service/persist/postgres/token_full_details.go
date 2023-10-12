package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type TokenFullDetails struct {
	Token      db.Token
	Contract   db.Contract
	Definition db.TokenDefinition
	Media      db.TokenMedia
}

func (t TokenFullDetails) TokenIdentifiers() persist.TokenIdentifiers {
	return persist.NewTokenIdentifiers(t.Contract.Address, t.Token.TokenID, t.Token.Chain)
}

type TokenFullDetailsRepository struct {
	db                                *sql.DB
	queries                           *db.Queries
	getByTokenIdentifiersStmt         *sql.Stmt
	getByTokenIdentifiersPaginateStmt *sql.Stmt
	getContractByAddressStmt          *sql.Stmt
}

func NewTokenFullDetailsRepository(db *sql.DB, queries *db.Queries) *TokenFullDetailsRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	getByTokenIdentifiersStmt, err := db.PrepareContext(ctx, `
		SELECT tokens.ID,tokens.COLLECTORS_NOTE,token_medias.MEDIA as token_media,tokens.TOKEN_MEDIA_ID,tokens.TOKEN_TYPE,tokens.CHAIN,token_medias.NAME,token_medias.DESCRIPTION,tokens.TOKEN_ID,tokens.TOKEN_URI,tokens.QUANTITY,tokens.OWNER_USER_ID,tokens.OWNED_BY_WALLETS,tokens.OWNERSHIP_HISTORY,token_medias.METADATA as token_metadata,tokens.EXTERNAL_URL,tokens.BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,tokens.IS_USER_MARKED_SPAM,tokens.IS_PROVIDER_MARKED_SPAM,contracts.ID,contracts.DELETED,contracts.VERSION,contracts.CREATED_AT,contracts.LAST_UPDATED,contracts.NAME,contracts.SYMBOL,contracts.ADDRESS,contracts.CREATOR_ADDRESS,contracts.CHAIN,contracts.PROFILE_BANNER_URL,contracts.PROFILE_IMAGE_URL,contracts.BADGE_URL,contracts.DESCRIPTION,contracts.OWNER_ADDRESS,contracts.IS_PROVIDER_MARKED_SPAM,contracts.PARENT_ID,contracts.OVERRIDE_CREATOR_USER_ID
		FROM tokens
		LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID
		JOIN contracts ON tokens.contract = contracts.ID
		WHERE tokens.TOKEN_ID = $1 AND CONTRACT = $2 AND DISPLAYABLE AND tokens.DELETED = false AND contracts.deleted = false
		ORDER BY BLOCK_NUMBER DESC;
	`)
	checkNoErr(err)

	getByTokenIdentifiersPaginateStmt, err := db.PrepareContext(ctx, `
		SELECT tokens.ID,tokens.COLLECTORS_NOTE,token_medias.MEDIA as token_media,tokens.TOKEN_MEDIA_ID,tokens.TOKEN_TYPE,tokens.CHAIN,token_medias.NAME,token_medias.DESCRIPTION,tokens.TOKEN_ID,tokens.TOKEN_URI,tokens.QUANTITY,tokens.OWNER_USER_ID,tokens.OWNED_BY_WALLETS,tokens.OWNERSHIP_HISTORY,token_medias.METADATA as token_metadata,tokens.EXTERNAL_URL,tokens.BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,tokens.IS_USER_MARKED_SPAM,tokens.IS_PROVIDER_MARKED_SPAM,contracts.ID,contracts.DELETED,contracts.VERSION,contracts.CREATED_AT,contracts.LAST_UPDATED,contracts.NAME,contracts.SYMBOL,contracts.ADDRESS,contracts.CREATOR_ADDRESS,contracts.CHAIN,contracts.PROFILE_BANNER_URL,contracts.PROFILE_IMAGE_URL,contracts.BADGE_URL,contracts.DESCRIPTION,contracts.OWNER_ADDRESS,contracts.IS_PROVIDER_MARKED_SPAM,contracts.PARENT_ID,contracts.OVERRIDE_CREATOR_USER_ID
		FROM tokens
		LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID
		JOIN contracts ON tokens.contract = contracts.ID
		WHERE tokens.TOKEN_ID = $1 AND CONTRACT = $2 AND DISPLAYABLE AND tokens.DELETED = false AND contracts.deleted = false
		ORDER BY BLOCK_NUMBER DESC
		LIMIT $3 OFFSET $4;
	`)
	checkNoErr(err)

	getContractByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID FROM contracts WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	return &TokenFullDetailsRepository{
		db:                                db,
		queries:                           queries,
		getByTokenIdentifiersStmt:         getByTokenIdentifiersStmt,
		getByTokenIdentifiersPaginateStmt: getByTokenIdentifiersPaginateStmt,
		getContractByAddressStmt:          getContractByAddressStmt,
	}

}

// GetByID gets a token by its DBID
func (t *TokenFullDetailsRepository) GetByID(ctx context.Context, tokenID persist.DBID) (TokenFullDetails, error) {
	r, err := t.queries.GetTokenFullDetailsByTokenDbid(ctx, tokenID)
	if err != nil {
		return TokenFullDetails{}, err
	}
	return TokenFullDetails{
		Token:      r.Token,
		Contract:   r.Contract,
		Definition: r.TokenDefinition,
		Media:      r.TokenMedia,
	}, nil
}

// GetByUserID gets all tokens for a user
func (t *TokenFullDetailsRepository) GetByUserID(ctx context.Context, userID persist.DBID) ([]TokenFullDetails, error) {
	r, err := t.queries.GetTokenFullDetailsByUserId(ctx, userID)
	if err != nil {
		return nil, err
	}
	tokens := util.MapWithoutError(r, func(r db.GetTokenFullDetailsByUserIdRow) TokenFullDetails {
		return TokenFullDetails{
			Token:      r.Token,
			Contract:   r.Contract,
			Definition: r.TokenDefinition,
			Media:      r.TokenMedia,
		}
	})
	return tokens, nil
}

// GetByTokenIdentifiers gets a token by its token ID and contract address and chain
// XXX: May not need this function
func (t *TokenFullDetailsRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pChain persist.Chain, limit int64, page int64) ([]persist.TokenGallery, error) {

	var contractID persist.DBID
	err := t.getContractByAddressStmt.QueryRowContext(pCtx, pContractAddress, pChain).Scan(&contractID)
	if err != nil {
		return nil, err
	}

	var rows *sql.Rows
	if limit > 0 {
		rows, err = t.getByTokenIdentifiersPaginateStmt.QueryContext(pCtx, pTokenID, contractID, limit, page*limit)
	} else {
		rows, err = t.getByTokenIdentifiersStmt.QueryContext(pCtx, pTokenID, contractID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]persist.TokenGallery, 0, 10)
	for rows.Next() {
		token := persist.TokenGallery{}
		contract := persist.ContractGallery{}
		if err := rows.Scan(&token.ID, &token.CollectorsNote, &token.TokenMedia, &token.TokenMediaID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam, &contract.ID, &contract.Deleted, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Name, &contract.Symbol, &contract.Address, &contract.CreatorAddress, &contract.Chain, &contract.ProfileBannerURL, &contract.ProfileImageURL, &contract.BadgeURL, &contract.Description, &contract.OwnerAddress, &contract.IsProviderMarkedSpam, &contract.ParentID, &contract.OverrideCreatorUserID); err != nil {
			return nil, err
		}
		token.Contract = contract
		tokens = append(tokens, token)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, persist.ErrTokenGalleryNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress, Chain: pChain}
	}

	return tokens, nil
}

// GetByContractID gets all tokens for a contract
func (t *TokenFullDetailsRepository) GetByContractID(ctx context.Context, contractID persist.DBID) ([]TokenFullDetails, error) {
	r, err := t.queries.GetTokenFullDetailsByContractId(ctx, contractID)
	if err != nil {
		return nil, err
	}
	tokens := util.MapWithoutError(r, func(r db.GetTokenFullDetailsByContractIdRow) TokenFullDetails {
		return TokenFullDetails{
			Token:      r.Token,
			Contract:   r.Contract,
			Definition: r.TokenDefinition,
			Media:      r.TokenMedia,
		}
	})
	return tokens, nil
}

type TokenUpsertParams struct {
	SetCreatorFields bool
	SetHolderFields  bool

	// If OptionalDelete is nil, no delete will be performed
	OptionalDelete *TokenUpsertDeletionParams
}

type TokenUpsertDeletionParams struct {
	DeleteCreatorStatus bool
	DeleteHolderStatus  bool
	OnlyFromUserID      *persist.DBID
	OnlyFromContracts   []persist.DBID
	OnlyFromChains      []persist.Chain
}

func (d *TokenUpsertDeletionParams) ToParams(upsertTime time.Time) db.DeleteTokensBeforeTimestampParams {
	userID := ""
	if d.OnlyFromUserID != nil {
		userID = d.OnlyFromUserID.String()
	}

	chains := util.MapWithoutError(d.OnlyFromChains, func(c persist.Chain) int32 { return int32(c) })
	return db.DeleteTokensBeforeTimestampParams{
		RemoveCreatorStatus: d.DeleteCreatorStatus,
		RemoveHolderStatus:  d.DeleteHolderStatus,
		OnlyFromUserID:      sql.NullString{String: userID, Valid: d.OnlyFromUserID != nil},
		OnlyFromChains:      chains,
		OnlyFromContractIds: util.StringersToStrings(d.OnlyFromContracts),
		Timestamp:           upsertTime,
	}
}

func (t *TokenFullDetailsRepository) UpsertTokens(ctx context.Context, tokens []db.Token, definitions []db.TokenDefinition, setCreatorFields bool, setHolderFields bool) (time.Time, []TokenFullDetails, error) {
	tokens = t.excludeZeroQuantityTokens(ctx, tokens)

	// If we're not upserting anything, we still need to return the current database time
	// since it may be used by the caller and is assumed valid if err == nil
	if len(tokens) == 0 {
		currentTime, err := t.queries.GetCurrentTime(ctx)
		if err != nil {
			return time.Time{}, nil, err
		}
		return currentTime, []TokenFullDetails{}, nil
	}

	params := db.UpsertTokensParams{
		SetCreatorFields:    setCreatorFields,
		SetHolderFields:     setHolderFields,
		TokenOwnedByWallets: []string{},
	}

	var errors []error

	for i := range definitions {
		d := &definitions[i]
		params.DefinitionDbid = append(params.DefinitionDbid, persist.GenerateID().String())
		params.DefinitionName = append(params.DefinitionName, d.Name.String)
		params.DefinitionDescription = append(params.DefinitionDescription, d.Description.String)
		params.DefinitionTokenType = append(params.DefinitionTokenType, d.TokenType.String())
		params.DefinitionTokenID = append(params.DefinitionTokenID, d.TokenID.String())
		params.DefinitionExternalUrl = append(params.DefinitionExternalUrl, d.ExternalUrl.String)
		params.DefinitionChain = append(params.DefinitionChain, int32(d.Chain))
		params.DefinitionIsProviderMarkedSpam = append(params.DefinitionIsProviderMarkedSpam, d.IsProviderMarkedSpam)
		appendJSONB(&params.DefinitionFallbackMedia, d.FallbackMedia, &errors)
		params.DefinitionContractID = append(params.DefinitionContractID, d.ContractID.String())
		// Defer error checking until now to keep the code above from being
		// littered with multiline "if" statements
		if len(errors) > 0 {
			return time.Time{}, nil, errors[0]
		}
	}

	for i := range tokens {
		t := &tokens[i]
		params.TokenDbid = append(params.TokenDbid, persist.GenerateID().String())
		params.TokenVersion = append(params.TokenVersion, t.Version.Int32)
		params.TokenCollectorsNote = append(params.TokenCollectorsNote, t.CollectorsNote.String)
		params.TokenTokenID = append(params.TokenTokenID, t.TokenID.String())
		params.TokenQuantity = append(params.TokenQuantity, t.Quantity.String())
		params.TokenBlockNumber = append(params.TokenBlockNumber, t.BlockNumber.Int64)
		params.TokenOwnerUserID = append(params.TokenOwnerUserID, t.OwnerUserID.String())
		appendDBIDList(&params.TokenOwnedByWallets, t.OwnedByWallets, &params.TokenOwnedByWalletsStartIdx, &params.TokenOwnedByWalletsEndIdx)
		params.TokenChain = append(params.TokenChain, int32(t.Chain))
		params.TokenContractID = append(params.TokenContractID, t.ContractID.String())
		params.TokenIsCreatorToken = append(params.TokenIsCreatorToken, t.IsCreatorToken)
		// Defer error checking until now to keep the code above from being
		// littered with multiline "if" statements
		if len(errors) > 0 {
			return time.Time{}, nil, errors[0]
		}
	}

	upserted, err := t.queries.UpsertTokens(ctx, params)
	if err != nil {
		return time.Time{}, nil, err
	}

	upsertedTokens := util.MapWithoutError(upserted, func(r db.UpsertTokensRow) TokenFullDetails {
		return TokenFullDetails{
			Token:      r.Token,
			Contract:   r.Contract,
			Definition: r.TokenDefinition,
			Media:      r.TokenMedia,
		}
	})

	return upserted[0].Token.LastSynced, upsertedTokens, nil
}

func (t *TokenFullDetailsRepository) excludeZeroQuantityTokens(ctx context.Context, tokens []db.Token) []db.Token {
	return util.Filter(tokens, func(t db.Token) bool {
		if t.Quantity == "" || t.Quantity == "0" {
			logger.For(ctx).Warnf("Token(chain=%d, contractID=%s, tokenID=%s) has 0 quantity", t.Chain, t.ContractID, t.TokenID)
			return false
		}
		return true
	}, false)
}
