alter table tokens drop column chain__deprecated;
alter table tokens drop column token_id__deprecated;
alter table tokens drop column name__deprecated;
alter table tokens drop column description__deprecated;
alter table tokens drop column token_type__deprecated;
alter table tokens drop column ownership_history__deprecated;
alter table tokens drop column external_url__deprecated;
alter table tokens drop column is_provider_marked_spam__deprecated;
alter table tokens drop column token_uri__deprecated;
alter table tokens drop column fallback_media__deprecated;
alter table tokens drop column token_media_id__deprecated;
alter table token_medias drop column name__deprecated;
alter table token_medias drop column description__deprecated;
alter table token_medias drop column metadata__deprecated;
alter table token_medias drop column contract_id__deprecated;
alter table token_medias drop column token_id__deprecated;
alter table token_medias drop column chain__deprecated;
drop table if exists tokens_backup;
alter index if exists tokens_with_token_definition_fk_pkey rename to tokens_pkey;
alter index if exists tokens_with_token_definition__owner_user_id_token_definitio_idx rename to tokens_owner_user_id_token_definition_id_idx;
alter index if exists tokens_with_token_definition__owner_user_id_is_creator_toke_idx rename to tokens_owner_user_id_is_creator_token_idx;
alter index if exists tokens_with_token_definition__owner_user_id_is_holder_token_idx rename to tokens_owner_user_id_is_holder_token_idx;
alter index if exists tokens_with_token_definition_fk_owner_user_id_displayable_idx rename to tokens_owner_user_id_displayable_idx;
alter index if exists tokens_with_token_definition_fk_owner_user_id_contract_idx rename to tokens_owner_user_id_contract_id_idx;
alter index if exists tokens_with_token_definition_fk_owned_by_wallets_idx rename to tokens_owned_by_wallets_idx;
alter index if exists tokens_with_token_definition_fk_last_updated_idx rename to tokens_last_updated_idx;
alter index if exists tokens_with_token_definition_f_contract_token_definition_id_idx rename to tokens_contract_id_token_definition_id_idx;
