create table if not exists push_notification_tokens (
    id varchar(255) primary key,
    user_id varchar(255) not null references users(id),
    push_token varchar(255) not null,
    created_at timestamptz not null,
    deleted bool not null
);

create index if not exists push_notification_tokens_user_id_idx on push_notification_tokens (user_id) where deleted = false;
create unique index if not exists push_notification_tokens_push_token_idx on push_notification_tokens (push_token) where deleted = false;

create table if not exists push_notification_tickets (
    id varchar(255) primary key,
    push_token_id varchar(255) not null references push_notification_tokens(id),
    ticket_id varchar(255) not null,
    created_at timestamptz not null,
    check_after timestamptz not null,
    num_check_attempts int not null,
    deleted bool not null
);

create index if not exists push_notification_tickets_created_at_idx on push_notification_tickets (created_at) where deleted = false;
create index if not exists push_notification_tickets_check_after_idx on push_notification_tickets (check_after) where deleted = false;
