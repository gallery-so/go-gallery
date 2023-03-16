-- Spam scores for newly-created users. Contains all newly created users,
-- but users with score 0 can typically be ignored since they're not likely to
-- be spam.
create table if not exists spam_user_scores (
    user_id varchar(255) primary key references users(id),
    score int not null,
    decided_is_spam bool,
    decided_at timestamptz,
    deleted bool not null,
    created_at timestamptz not null
);

create index spam_user_scores_created_at_idx on spam_user_scores(created_at);