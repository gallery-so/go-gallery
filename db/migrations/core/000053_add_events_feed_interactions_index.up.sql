create index if not exists events_feed_interactions_idx on events (created_at) where action in ('CommentedOnFeedEvent', 'AdmiredFeedEvent') and feed_event_id is not null;
