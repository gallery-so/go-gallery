select cron.schedule('@daily', 'refresh materialized view concurrently top_recommended_users with data');
