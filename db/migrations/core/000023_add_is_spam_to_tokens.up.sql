ALTER TABLE tokens ADD COLUMN IF NOT EXISTS is_user_marked_spam bool;
<<<<<<< HEAD
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS is_provider_marked_spam bool;
=======
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS is_provider_marked_spam bool;
>>>>>>> ef25202 (Add user marked spam)
