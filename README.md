# COD Status Bot

[![Go](https://github.com/bradselph/CODStatusBot/actions/workflows/go.yml/badge.svg)](https://github.com/bradselph/CODStatusBot/actions/workflows/go.yml)

## Introduction

COD Status Bot is a Discord bot designed to help you monitor your Activision accounts for shadowbans or permanent bans in Call of Duty games. The bot periodically checks the status of your accounts and notifies you of any changes. Now serving 300+ Discord servers, our bot has been optimized for performance and scalability.

## Features

- Monitor multiple Activision accounts
- Periodic automatic checks (customizable interval with your own API key)
- Manual status checks
- Account age verification
- Ban history logs
- Customizable notification preferences
- Anonymous feedback submission
- EZ-Captcha integration for continued compatibility with Activision
- SSO Cookie expiration tracking and notifications
- Toggle automatic checks on/off for individual accounts

## Getting Started

1. Invite the bot to your Discord server using the provided [Invite Link](https://discord.com/oauth2/authorize?client_id=1211857854324015124).
2. Once the bot joins your server, it will automatically register the necessary commands.
3. Set up your EZ-Captcha API key for full functionality and customized check intervals.
4. Use the `/addaccount` command to start monitoring your first account.

## EZ-Captcha Integration

The bot uses EZ-Captcha for solving CAPTCHAs, which maintains compatibility to check your accounts with
Activision. Users have two options:

1. Use the bot's default API key (limited use, shared among all users)
2. Get your own EZ-Captcha API key for unlimited use and customizable check intervals

### Getting Your Own EZ-Captcha API Key

1. Visit [EZ-Captcha Registration](https://dashboard.ez-captcha.com/#/register?inviteCode=uyNrRgWlEKy)
2. Complete the registration process
3. Once registered, you'll receive your API key
4. Use the `/setcaptchaservice` command to set your API key in the bot

By using your own API key, you can customize the check interval for your accounts and enjoy unlimited use of the service.

## Commands

### /addaccount

Add a new account to be monitored by the bot. You'll need to provide:
- Account Title: A name to identify the account
- SSO Cookie: The Single Sign-On cookie associated with your Activision account

### /removeaccount

Remove an account from being monitored by the bot. This will delete all associated data.

### /updateaccount

Update the SSO cookie for an existing account. Use this when your cookie expires or becomes invalid.

### /listaccounts

List all your monitored accounts, including their current status and notification preferences.

### /accountlogs

View the status change logs for a specific account or all accounts. This shows the last 10 status changes.

### /accountage

Check the age of a specific account. This displays the account's creation date and current age.

### /checknow

Immediately check the status of all your accounts or a specific account. This command is rate-limited for users without a personal API key.

### /setcheckinterval

Set your preferences for:
- Check Interval: How often the bot checks your accounts (in minutes)
- Notification Interval: How often you receive status updates (in hours)
- Notification Type: Choose between channel or DM notifications

### /setcaptchaservice

Set your personal EZ-Captcha API key for unlimited use and customizable check intervals.

### /helpapi

Display a detailed guide on how to use the bot and set up your API key.

### /helpcookie

Display a step-by-step guide on how to obtain your SSO cookie from the Activision website.

### /feedback

Send anonymous feedback or suggestions to the bot developer. This is sent directly to the developer's DMs.

### /togglecheck

Toggle automatic checks on/off for a monitored account. Useful for temporarily disabling checks on specific accounts.

## Notifications

The bot will send notifications:

- When there's a change in the ban status of an account
- Daily for each account, confirming that it's still being monitored
- If an SSO cookie becomes invalid or is about to expire
- 24 hours before an SSO cookie is set to expire

Notifications will be sent to the channel where the account was added or to your DMs, depending on your preference set with `/setcheckinterval`.

## SSO Cookie

The SSO (Single Sign-On) cookie is required to authenticate with Activision's services. To get the SSO cookie:

1. Log in to your Activision account on a web browser.
2. Open the browser's developer tools (usually F12 or right-click and select "Inspect").
3. Navigate to the Application or Storage tab.
4. Find the cookie named `ACT_SSO_COOKIE` associated with the Activision domain.
5. Copy the entire value of this cookie.

For a detailed guide, use the `/helpcookie` command.

## Rate Limiting

To prevent abuse and ensure fair usage:

- Users without a personal API key are subject to rate limits on the `/checknow` command.
- Global cooldowns are implemented for notifications to prevent spam.

## Database and Data Management

The bot uses a MySQL database to store account information and user settings. It includes:

- Secure storage of SSO cookies and user preferences
- Regular checks for expired cookies and account status changes
- Optimized queries and connection pooling for high-performance

## Support and Feedback

If you encounter any issues or have questions:

1. Use the `/feedback` command to contact the bot developer anonymously.
2. Join our [Support Server](https://discord.gg/your-support-server) for real-time assistance and updates.

## Privacy and Data Security

- The bot stores minimal data necessary for operation: account titles, SSO cookies, and status logs.
- Data is used solely for monitoring account status and providing notifications.
- No data is shared with third parties.
- Users can delete their data at any time using the `/removeaccount` command.
- We employ industry-standard security practices to protect your data.

## Recent Changes and Updates

- Optimized bot performance for 300+ Discord servers
- Implemented database connection pooling for improved scalability
- Added caching mechanisms to reduce API calls and improve response times
- Enhanced rate limiting to ensure fair usage across all servers
- Improved error handling and logging for better issue resolution
- Implemented performance monitoring to maintain high uptime and responsiveness

## Disclaimer

This bot is not affiliated with or endorsed by Activision. Use it at your own risk. The developers are not responsible for any consequences resulting from the use of this bot.

## Contributing

We welcome contributions to the COD Status Bot! If you'd like to contribute:

1. Fork the repository on GitHub.
2. Create a new branch for your feature or bug fix.
3. Commit your changes with clear, descriptive commit messages.
4. Push your branch and submit a pull request.

Please ensure your code adheres to the existing style and passes all tests. For major changes, please open an issue first to discuss what you would like to change.

## License

This project is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0). This means:

- You can use, modify, and distribute this software freely.
- If you modify the software and use it to provide a service over a network, you must make your modified source code available to users of that service.
- Any modifications or larger works must also be licensed under AGPL-3.0.

For more details, see the [LICENSE](LICENSE) file in the repository or visit [GNU AGPL-3.0](https://www.gnu.org/licenses/agpl-3.0.en.html).

## Open Source

This project is open source and available on GitHub. We believe in the power of community-driven development and welcome contributions from developers around the world. By making this bot open source, we aim to:

- Encourage collaboration and improvement of the bot's features.
- Provide transparency in how the bot operates.
- Enable the community to adapt the bot for their specific needs.

You can find the full source code, contribute to the project, or report issues at our [GitHub repository](https://github.com/bradselph/CODStatusBot).


Thank you for using COD Status Bot! We're committed to providing a reliable and efficient service to our growing community of users across 300+ Discord servers.