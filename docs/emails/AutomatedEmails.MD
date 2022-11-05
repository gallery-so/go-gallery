### Sending Automated emails

Automated emails are triggered on a scheduled timer that is set with a [GCP scheduled task](https://cloud.google.com/scheduler/docs/schedule-run-cron-job).

When a task is triggered, the task will shoot a request to the email service letting it know to pull all verified emails
that are not unsubscribed from all emails or the email type that is being sent, for example, notifications. It will then
iteratively and concurrently send emails to each of the addresses using a [SendGrid Template](https://docs.sendgrid.com/for-developers/sending-email/using-handlebars#handlebarjs-reference) and [dynamic template data](https://docs.sendgrid.com/ui/sending-email/how-to-send-an-email-with-dynamic-templates).
