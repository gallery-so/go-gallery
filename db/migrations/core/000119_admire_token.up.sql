ALTER TABLE admires ADD COLUMN IF NOT EXISTS token_id varchar(255) references token(id);
