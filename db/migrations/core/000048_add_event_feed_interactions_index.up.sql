create index if not exists event_feed_interactions_idx on events (created_at) where action in ('CommentedOnFeedEvent', 'AdmiredFeedEvent') and feed_event_id is not null;
