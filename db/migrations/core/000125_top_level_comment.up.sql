alter table comments add column top_level_comment_id varchar(255) references comments(id);

create index top_level_comment_id_idx on comments(top_level_comment_id);

WITH RECURSIVE comment_thread AS (
    SELECT 
        id, 
        reply_to, 
        id as top_level_comment_id
    FROM comments
    WHERE reply_to IS NULL

    UNION ALL

    SELECT 
        c.id, 
        c.reply_to, 
        ct.top_level_comment_id
    FROM comments c
    INNER JOIN comment_thread ct ON c.reply_to = ct.id
)
UPDATE comments
SET top_level_comment_id = ct.top_level_comment_id
FROM comment_thread ct
WHERE comments.id = ct.id AND comments.reply_to IS NOT NULL;