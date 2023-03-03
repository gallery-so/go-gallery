/* {% require_sudo %} */
CREATE INDEX IF NOT EXISTS user_id_event_code_last_updated ON user_events (USER_ID, EVENT_CODE, LAST_UPDATED DESC);

CREATE INDEX IF NOT EXISTS user_id_nft_id_event_code_last_updated ON nft_events (USER_ID, NFT_ID, EVENT_CODE, LAST_UPDATED DESC);

CREATE INDEX IF NOT EXISTS user_id_collection_id_event_code_last_updated ON collection_events (USER_ID, COLLECTION_ID, EVENT_CODE, LAST_UPDATED DESC);
