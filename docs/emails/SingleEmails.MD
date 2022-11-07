### Single Emails

Single emails are sent directly from the SendGrid app, but for the SendGrid app to know who to send emails to, every time a user verifies their email, we also add them to our `contacts` on SendGrid. As user's update their subscription statuses we have to make sure we also add them to an [unsubscribe group](https://docs.sendgrid.com/ui/sending-email/unsubscribe-groups) so that the single emails will not be sent to them if not necesssary.
