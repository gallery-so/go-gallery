create index concurrently if not exists base_tokens_created_at_simplehash_kafka_key on base.tokens (created_at, simplehash_kafka_key);
