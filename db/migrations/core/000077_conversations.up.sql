create table if not exists conversations (
    id varchar(255) primary key,
    user_id varchar(255) references users(id),
    opening_prompt varchar not null,
    psuedo_tokens varchar not null,
    opening_state varchar not null,
    current_state varchar not null,
    messages jsonb,
    given_ids jsonb,
    helpful bool,
    used_tokens int not null default 0,
    deleted bool not null default false,
    created_at timestamptz not null default current_timestamp,
    last_updated timestamptz not null default current_timestamp
);