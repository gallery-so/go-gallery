-- Not sure if it matters in practical terms, but stagger these so we're not computing a bunch of stuff at the same time
select cron.schedule('5 * * * *', 'refresh materialized view concurrently user_relevance with data');
select cron.schedule('10 * * * *', 'refresh materialized view concurrently gallery_relevance with data');
select cron.schedule('15 * * * *', 'refresh materialized view concurrently contract_relevance with data');
