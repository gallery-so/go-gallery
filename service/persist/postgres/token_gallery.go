package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

// TokenGalleryRepository represents a postgres repository for tokens
type TokenGalleryRepository struct {
	db                      *sql.DB
	queries                 *db.Queries
	getByID                 *sql.Stmt
	getByUserIDStmt         *sql.Stmt
	getByUserIDPaginateStmt *sql.Stmt
	getByContractIDStmt     *sql.Stmt

	getByTokenIdentifiersStmt         *sql.Stmt
	getByTokenIdentifiersPaginateStmt *sql.Stmt

	updateInfoStmt                         *sql.Stmt
	updateInfoByTokenIdentifiersUnsafeStmt *sql.Stmt

	getContractByAddressStmt                  *sql.Stmt
	setTokensAsUserMarkedSpamStmt             *sql.Stmt
	checkOwnTokensStmt                        *sql.Stmt
	deleteTokensOfContractBeforeTimeStampStmt *sql.Stmt
}

var errTokensNotOwnedByUser = errors.New("not all tokens are owned by user")

// NewTokenGalleryRepository creates a new TokenRepository
// TODO joins on addresses
func NewTokenGalleryRepository(db *sql.DB, queries *db.Queries) *TokenGalleryRepository {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	getByIDStmt, err := db.PrepareContext(ctx, `
		SELECT tokens.ID,tokens.COLLECTORS_NOTE,token_medias.MEDIA as token_media,tokens.TOKEN_MEDIA_ID,tokens.TOKEN_TYPE,tokens.CHAIN,token_medias.NAME,token_medias.DESCRIPTION,tokens.TOKEN_ID,tokens.TOKEN_URI,tokens.QUANTITY,tokens.OWNER_USER_ID,tokens.OWNED_BY_WALLETS,tokens.OWNERSHIP_HISTORY,token_medias.METADATA as token_metadata,tokens.EXTERNAL_URL,tokens.BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,tokens.IS_USER_MARKED_SPAM,tokens.IS_PROVIDER_MARKED_SPAM,contracts.ID,contracts.DELETED,contracts.VERSION,contracts.CREATED_AT,contracts.LAST_UPDATED,contracts.NAME,contracts.SYMBOL,contracts.ADDRESS,contracts.CREATOR_ADDRESS,contracts.CHAIN,contracts.PROFILE_BANNER_URL,contracts.PROFILE_IMAGE_URL,contracts.BADGE_URL,contracts.DESCRIPTION,contracts.OWNER_ADDRESS,contracts.IS_PROVIDER_MARKED_SPAM,contracts.PARENT_ID,contracts.OVERRIDE_CREATOR_USER_ID
		FROM tokens
		LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID
		JOIN contracts ON tokens.contract = contracts.ID
		WHERE tokens.ID = $1 AND tokens.DISPLAYABLE AND tokens.DELETED = false AND contracts.deleted = false;
	`)
	checkNoErr(err)

	getByUserIDStmt, err := db.PrepareContext(ctx, `
		SELECT tokens.ID,tokens.COLLECTORS_NOTE,token_medias.MEDIA,TOKEN_MEDIA_ID,TOKEN_TYPE,tokens.CHAIN,token_medias.NAME,token_medias.DESCRIPTION,tokens.TOKEN_ID,TOKEN_URI,QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,token_medias.METADATA as token_metadata,tokens.EXTERNAL_URL,tokens.BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,tokens.IS_USER_MARKED_SPAM,tokens.IS_PROVIDER_MARKED_SPAM,contracts.ID,contracts.DELETED,contracts.VERSION,contracts.CREATED_AT,contracts.LAST_UPDATED,contracts.NAME,contracts.SYMBOL,contracts.ADDRESS,contracts.CREATOR_ADDRESS,contracts.CHAIN,contracts.PROFILE_BANNER_URL,contracts.PROFILE_IMAGE_URL,contracts.BADGE_URL,contracts.DESCRIPTION,contracts.OWNER_ADDRESS,contracts.IS_PROVIDER_MARKED_SPAM,contracts.PARENT_ID,contracts.OVERRIDE_CREATOR_USER_ID
		FROM tokens
		LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID
		JOIN contracts ON tokens.contract = contracts.ID
		WHERE OWNER_USER_ID = $1 AND tokens.DISPLAYABLE AND tokens.DELETED = false AND contracts.deleted = false
		ORDER BY BLOCK_NUMBER DESC;
	`)
	checkNoErr(err)

	getByUserIDPaginateStmt, err := db.PrepareContext(ctx, `
		SELECT tokens.ID,tokens.COLLECTORS_NOTE,token_medias.MEDIA as token_media,tokens.TOKEN_MEDIA_ID,tokens.TOKEN_TYPE,tokens.CHAIN,token_medias.NAME,token_medias.DESCRIPTION,tokens.TOKEN_ID,tokens.TOKEN_URI,tokens.QUANTITY,tokens.OWNER_USER_ID,tokens.OWNED_BY_WALLETS,tokens.OWNERSHIP_HISTORY,token_medias.METADATA as token_metadata,tokens.EXTERNAL_URL,tokens.BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,tokens.IS_USER_MARKED_SPAM,tokens.IS_PROVIDER_MARKED_SPAM,contracts.ID,contracts.DELETED,contracts.VERSION,contracts.CREATED_AT,contracts.LAST_UPDATED,contracts.NAME,contracts.SYMBOL,contracts.ADDRESS,contracts.CREATOR_ADDRESS,contracts.CHAIN,contracts.PROFILE_BANNER_URL,contracts.PROFILE_IMAGE_URL,contracts.BADGE_URL,contracts.DESCRIPTION,contracts.OWNER_ADDRESS,contracts.IS_PROVIDER_MARKED_SPAM,contracts.PARENT_ID,contracts.OVERRIDE_CREATOR_USER_ID
		FROM tokens
		LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID
		JOIN contracts ON tokens.contract = contracts.ID
		WHERE OWNER_USER_ID = $1 AND tokens.DISPLAYABLE AND tokens.DELETED = false AND contracts.deleted = false
		ORDER BY BLOCK_NUMBER DESC
		LIMIT $2 OFFSET $3;
	`)
	checkNoErr(err)

	getByContractIDStmt, err := db.PrepareContext(ctx, `
		SELECT tokens.ID,tokens.COLLECTORS_NOTE,token_medias.MEDIA,tokens.TOKEN_MEDIA_ID,tokens.TOKEN_TYPE,tokens.CHAIN,token_medias.NAME,token_medias.DESCRIPTION,tokens.TOKEN_ID,tokens.TOKEN_URI,tokens.QUANTITY,OWNER_USER_ID,OWNED_BY_WALLETS,OWNERSHIP_HISTORY,token_medias.METADATA as token_metadata,tokens.EXTERNAL_URL,tokens.BLOCK_NUMBER,tokens.VERSION,tokens.CREATED_AT,tokens.LAST_UPDATED,tokens.IS_USER_MARKED_SPAM,tokens.IS_PROVIDER_MARKED_SPAM,contracts.ID,contracts.DELETED,contracts.VERSION,contracts.CREATED_AT,contracts.LAST_UPDATED,contracts.NAME,contracts.SYMBOL,contracts.ADDRESS,contracts.CREATOR_ADDRESS,contracts.CHAIN,contracts.PROFILE_BANNER_URL,contracts.PROFILE_IMAGE_URL,contracts.BADGE_URL,contracts.DESCRIPTION,contracts.OWNER_ADDRESS,contracts.IS_PROVIDER_MARKED_SPAM,contracts.PARENT_ID,contracts.OVERRIDE_CREATOR_USER_ID
		FROM tokens
		LEFT JOIN token_medias ON token_medias.ID = tokens.TOKEN_MEDIA_ID
		JOIN contracts ON tokens.contract = contracts.ID
		WHERE tokens.CONTRACT = $1 AND tokens.DISPLAYABLE AND tokens.DELETED = false AND contracts.deleted = false
		ORDER BY BLOCK_NUMBER DESC;
	`)
	checkNoErr(err)

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

	updateInfoStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE ID = $3 AND OWNER_USER_ID = $4;`)
	checkNoErr(err)

	updateInfoByTokenIdentifiersUnsafeStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET COLLECTORS_NOTE = $1, LAST_UPDATED = $2 WHERE TOKEN_ID = $3 AND CONTRACT = $4 AND DELETED = false;`)
	checkNoErr(err)

	getContractByAddressStmt, err := db.PrepareContext(ctx, `SELECT ID FROM contracts WHERE ADDRESS = $1 AND CHAIN = $2 AND DELETED = false;`)
	checkNoErr(err)

	setTokensAsUserMarkedSpamStmt, err := db.PrepareContext(ctx, `UPDATE tokens SET is_user_marked_spam = $1, LAST_UPDATED = now() WHERE OWNER_USER_ID = $2 AND ID = ANY($3) AND DELETED = false;`)
	checkNoErr(err)

	checkOwnTokensStmt, err := db.PrepareContext(ctx, `SELECT COUNT(*) = $1 FROM tokens WHERE OWNER_USER_ID = $2 AND ID = ANY($3);`)
	checkNoErr(err)

	deleteTokensOfContractBeforeTimeStampStmt, err := db.PrepareContext(ctx, `update tokens set owned_by_wallets = '{}' where contract = $1 and last_synced < $2 and deleted = false;`)
	checkNoErr(err)

	return &TokenGalleryRepository{
		db:                                db,
		queries:                           queries,
		getByID:                           getByIDStmt,
		getByUserIDStmt:                   getByUserIDStmt,
		getByUserIDPaginateStmt:           getByUserIDPaginateStmt,
		getByContractIDStmt:               getByContractIDStmt,
		getByTokenIdentifiersStmt:         getByTokenIdentifiersStmt,
		getByTokenIdentifiersPaginateStmt: getByTokenIdentifiersPaginateStmt,
		updateInfoStmt:                    updateInfoStmt,

		updateInfoByTokenIdentifiersUnsafeStmt: updateInfoByTokenIdentifiersUnsafeStmt,

		getContractByAddressStmt:      getContractByAddressStmt,
		setTokensAsUserMarkedSpamStmt: setTokensAsUserMarkedSpamStmt,
		checkOwnTokensStmt:            checkOwnTokensStmt,

		deleteTokensOfContractBeforeTimeStampStmt: deleteTokensOfContractBeforeTimeStampStmt,
	}

}

// GetByID gets a token by its DBID
func (t *TokenGalleryRepository) GetByID(pCtx context.Context, tokenID persist.DBID) (persist.TokenGallery, error) {
	token := persist.TokenGallery{}
	contract := persist.ContractGallery{}
	err := t.getByID.QueryRowContext(pCtx, tokenID).Scan(&token.ID, &token.CollectorsNote, &token.TokenMedia, &token.TokenMediaID, &token.TokenType, &token.Chain, &token.Name, &token.Description, &token.TokenID, &token.TokenURI, &token.Quantity, &token.OwnerUserID, pq.Array(&token.OwnedByWallets), pq.Array(&token.OwnershipHistory), &token.TokenMetadata, &token.ExternalURL, &token.BlockNumber, &token.Version, &token.CreationTime, &token.LastUpdated, &token.IsUserMarkedSpam, &token.IsProviderMarkedSpam, &contract.ID, &contract.Deleted, &contract.Version, &contract.CreationTime, &contract.LastUpdated, &contract.Name, &contract.Symbol, &contract.Address, &contract.CreatorAddress, &contract.Chain, &contract.ProfileBannerURL, &contract.ProfileImageURL, &contract.BadgeURL, &contract.Description, &contract.OwnerAddress, &contract.IsProviderMarkedSpam, &contract.ParentID, &contract.OverrideCreatorUserID)

	if err != nil {
		return persist.TokenGallery{}, err
	}

	token.Contract = contract
	return token, nil
}

// GetByUserID gets all tokens for a user
func (t *TokenGalleryRepository) GetByUserID(pCtx context.Context, pUserID persist.DBID, limit int64, page int64) ([]persist.TokenGallery, error) {
	var rows *sql.Rows
	var err error
	if limit > 0 {
		rows, err = t.getByUserIDPaginateStmt.QueryContext(pCtx, pUserID, limit, page*limit)
	} else {
		rows, err = t.getByUserIDStmt.QueryContext(pCtx, pUserID)
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

	return tokens, nil

}

// GetByTokenIdentifiers gets a token by its token ID and contract address and chain
func (t *TokenGalleryRepository) GetByTokenIdentifiers(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pChain persist.Chain, limit int64, page int64) ([]persist.TokenGallery, error) {

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
func (t *TokenGalleryRepository) GetByContractID(pCtx context.Context, pContractID persist.DBID) ([]persist.TokenGallery, error) {
	rows, err := t.getByContractIDStmt.QueryContext(pCtx, pContractID)
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

func (t *TokenGalleryRepository) UpsertTokens(ctx context.Context, tokens []persist.TokenGallery, definitions []db.TokenDefinition, setCreatorFields bool, setHolderFields bool) (time.Time, []persist.TokenGallery, error) {
	tokens, err := t.excludeZeroQuantityTokens(ctx, tokens)
	if err != nil {
		return time.Time{}, nil, err
	}

	// If we're not upserting anything, we still need to return the current database time
	// since it may be used by the caller and is assumed valid if err == nil
	if len(tokens) == 0 {
		currentTime, err := t.queries.GetCurrentTime(ctx)
		if err != nil {
			return time.Time{}, nil, err
		}
		return currentTime, []persist.TokenGallery{}, nil
	}

	params := db.UpsertTokensParams{
		SetCreatorFields: setCreatorFields,
		SetHolderFields:  setHolderFields,
		OwnedByWallets:   []string{},
	}

	var errors []error

	for i := range tokens {
		t := &tokens[i]
		params.ID = append(params.ID, persist.GenerateID().String())
		params.Version = append(params.Version, t.Version.Int32())
		params.Name = append(params.Name, t.Name.String())
		params.Description = append(params.Description, t.Description.String())
		params.CollectorsNote = append(params.CollectorsNote, t.CollectorsNote.String())
		params.TokenType = append(params.TokenType, t.TokenType.String())
		params.TokenID = append(params.TokenID, t.TokenID.String())
		params.Quantity = append(params.Quantity, t.Quantity.String())
		InsertHelpers.AppendAddressAtBlock(&params.OwnershipHistory, t.OwnershipHistory, &params.OwnershipHistoryStartIdx, &params.OwnershipHistoryEndIdx, &errors)
		appendJSONB(&params.FallbackMedia, t.FallbackMedia, &errors)
		params.ExternalUrl = append(params.ExternalUrl, t.ExternalURL.String())
		params.BlockNumber = append(params.BlockNumber, t.BlockNumber.BigInt().Int64())
		params.OwnerUserID = append(params.OwnerUserID, t.OwnerUserID.String())
		InsertHelpers.AppendWalletList(&params.OwnedByWallets, t.OwnedByWallets, &params.OwnedByWalletsStartIdx, &params.OwnedByWalletsEndIdx)
		params.Chain = append(params.Chain, int32(t.Chain))
		params.Contract = append(params.Contract, t.Contract.ID.String())
		appendBool(&params.IsProviderMarkedSpam, t.IsProviderMarkedSpam, &errors)
		params.TokenUri = append(params.TokenUri, "")
		params.IsCreatorToken = append(params.IsCreatorToken, t.IsCreatorToken)

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

	// Update tokens with the existing data if the token already exists.
	for i := range tokens {
		t := &tokens[i]
		(*t).ID = upserted[i].ID
		(*t).CreationTime = upserted[i].CreatedAt
		(*t).LastUpdated = upserted[i].LastUpdated
		(*t).LastSynced = upserted[i].LastSynced
	}

	return upserted[0].LastSynced, tokens, nil
}

func (t *TokenGalleryRepository) excludeZeroQuantityTokens(pCtx context.Context, pTokens []persist.TokenGallery) ([]persist.TokenGallery, error) {
	newTokens := make([]persist.TokenGallery, 0, len(pTokens))
	for _, token := range pTokens {
		if token.Quantity == "" || token.Quantity == "0" {
			logger.For(pCtx).Warnf("Token %s has 0 quantity", token.Name)
			continue
		}
		newTokens = append(newTokens, token)
	}
	return newTokens, nil
}

// UpdateByID updates a token by its ID
func (t *TokenGalleryRepository) UpdateByID(pCtx context.Context, pID persist.DBID, pUserID persist.DBID, pUpdate interface{}) error {
	var res sql.Result
	var err error

	switch errTyp := pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		res, err = t.updateInfoStmt.ExecContext(pCtx, errTyp.CollectorsNote, errTyp.LastUpdated, pID, pUserID)

	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return persist.ErrTokenNotFoundByID{ID: pID}
	}
	return nil
}

// UpdateByTokenIdentifiersUnsafe updates a token by its token identifiers without checking if it is owned by any given user
func (t *TokenGalleryRepository) UpdateByTokenIdentifiersUnsafe(pCtx context.Context, pTokenID persist.TokenID, pContractAddress persist.Address, pChain persist.Chain, pUpdate interface{}) error {

	var contractID persist.DBID
	if err := t.getContractByAddressStmt.QueryRowContext(pCtx, pContractAddress, pChain).Scan(&contractID); err != nil {
		return err
	}

	var res sql.Result
	var err error
	switch errTyp := pUpdate.(type) {
	case persist.TokenUpdateInfoInput:
		res, err = t.updateInfoByTokenIdentifiersUnsafeStmt.ExecContext(pCtx, errTyp.CollectorsNote, errTyp.LastUpdated, pTokenID, contractID)
	default:
		return fmt.Errorf("unsupported update type: %T", pUpdate)
	}
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return persist.ErrTokenGalleryNotFoundByIdentifiers{TokenID: pTokenID, ContractAddress: pContractAddress, Chain: pChain}
	}
	return nil
}

// FlagTokensAsUserMarkedSpam marks tokens as spam by the user.
func (t *TokenGalleryRepository) FlagTokensAsUserMarkedSpam(ctx context.Context, ownerUserID persist.DBID, tokens []persist.DBID, isSpam bool) error {
	_, err := t.setTokensAsUserMarkedSpamStmt.ExecContext(ctx, isSpam, ownerUserID, tokens)
	return err
}

// TokensAreOwnedByUser checks if all tokens are owned by the provided user.
func (t *TokenGalleryRepository) TokensAreOwnedByUser(ctx context.Context, userID persist.DBID, tokens []persist.DBID) error {
	var owned bool

	err := t.checkOwnTokensStmt.QueryRowContext(ctx, len(tokens), userID, tokens).Scan(&owned)
	if err != nil {
		return err
	}

	if !owned {
		return errTokensNotOwnedByUser
	}

	return nil
}
