ALTER TABLE admires ADD COLUMN IF NOT EXISTS token_id varchar(255) references token(id);
CREATE UNIQUE index admire_actor_id_token_id_idx ON admires(actor_id, token_id) WHERE deleted = false;
