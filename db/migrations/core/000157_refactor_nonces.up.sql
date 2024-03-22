alter table nonces rename to legacy_nonces;

create table nonces (
  id varchar(255) primary key,
  value text not null,
  created_at timestamptz not null default now(),
  consumed bool not null default false
);

create unique index nonces_value_idx on nonces (value);