/* The migration will fail if there are null `actor_ids`. These rows should be set to some sensible default beforehand. */
alter table events alter column actor_id set not null;
