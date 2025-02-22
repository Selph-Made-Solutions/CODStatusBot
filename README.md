# COD Status Bot

[![Go](https://github.com/bradselph/CODStatusBot/actions/workflows/go.yml/badge.svg)](https://github.com/bradselph/CODStatusBot/actions/workflows/go.yml)
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fbradselph%2FCODStatusBot.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fbradselph%2FCODStatusBot?ref=badge_shield)

## Introduction

COD Status Bot is a Discord bot designed to help you monitor your Activision accounts for shadowbans or permanent bans in Call of Duty games. The bot periodically checks the status of your accounts and notifies you of any changes.

## Features

- Monitor multiple Activision accounts
- Periodic automatic checks (customizable interval with your own API key)
- Manual status checks with `/checknow`
- Account age verification and VIP status tracking
- Comprehensive ban history logs
- Customizable notification preferences (channel or DM)
- Multiple captcha service support (Capsolver, EZ-Captcha, 2Captcha)
- SSO Cookie expiration tracking and notifications
- Toggle automatic checks on/off for individual accounts
- Consolidated daily status updates
- Anonymous feedback submission

## Getting Started

1. Invite the bot to your Discord server using the provided [Invite Link](https://discord.com/oauth2/authorize?client_id=1211857854324015124)
2. Once the bot joins your server, it will automatically register the necessary commands
3. Set up your captcha service API key using `/setcaptchaservice` for full functionality
4. Use the `/addaccount` command to start monitoring your first account

## Captcha Service Integration

The bot supports multiple captcha services, with Capsolver being the recommended provider. Users have two options:

1. Use the bot's default API key (limited use, shared among all users)
2. Get your own API key for unlimited use and customizable check intervals

### Getting Your Own API Key

#### Capsolver (Recommended):
1. Visit [Capsolver Registration](https://dashboard.capsolver.com/passport/register?inviteCode=6YjROhACQnvP)
2. Complete registration and purchase credits (starting at $0.001 per solve)
3. Use `/setcaptchaservice` to configure your API key

#### Alternative Services:
- EZ-Captcha
- 2Captcha

Use `/setcaptchaservice` to select your preferred service and configure your API key.

## Commands

### Account Management
- `/addaccount` - Add a new account to monitor
- `/removeaccount` - Remove an account from monitoring
- `/updateaccount` - Update an account's SSO cookie
- `/listaccounts` - View all monitored accounts
- `/accountlogs` - View account status history
- `/accountage` - Check account age and VIP status
- `/togglecheck` - Enable/disable monitoring for an account

### Status Checking
- `/checknow` - Immediately check account status
- `/checkcaptchabalance` - View your captcha service balance

### Configuration
- `/setcheckinterval` - Configure check and notification intervals
- `/setnotifications` - Set notification preferences
- `/setcaptchaservice` - Configure captcha service settings

### Help and Support
- `/helpapi` - View detailed API setup guide
- `/helpcookie` - Get SSO cookie instructions
- `/feedback` - Send anonymous feedback

## Notifications

The bot sends notifications for:
- Status changes (permanent bans, temporary bans, shadowbans)
- Consolidated daily status updates
- Cookie expiration warnings
- Captcha balance alerts
- VIP status changes
- Account monitoring status updates

## Premium Features with Personal API Key

Users with their own API key enjoy:
- Unlimited status checks
- Faster check intervals
- Increased account monitoring slots
- Priority status updates
- Customizable notification settings

## Data Security and Privacy

- Minimal data storage: only essential account information and settings
- No sharing with third parties
- Data deletion available via `/removeaccount`
- Industry-standard security practices

## Recent Updates

- Added Capsolver as primary captcha service
- Implemented consolidated daily status updates
- Enhanced notification system with cooldown management
- Improved error handling and recovery
- Added VIP status tracking
- Optimized performance and reliability

## Contributing

We welcome contributions! To contribute:

1. Fork the repository
2. Create a feature branch
3. Submit a pull request

Please ensure your code follows existing patterns and includes appropriate tests.

## License

Licensed under GNU AGPL-3.0. See [LICENSE](LICENSE) file for details.

[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fbradselph%2FCODStatusBot.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fbradselph%2FCODStatusBot?ref=badge_large)