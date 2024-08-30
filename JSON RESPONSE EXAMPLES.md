# Ban Check Endpoint Examples

* No ban Example:

```json
{
  "error": "",
  "success": "boolan",
  "canAppeal": boolan,
  "bans": []
}
````

* Shadowban Example:

```json
{
  "error": "",
  "success": "boolan",
  "canAppeal": boolan,
  "bans": [
    {
      "enforcement": "UNDER_REVIEW",
      "canAppeal": boolan,
      "title": "COD:MW3"
    },
    {
      "enforcement": "UNDER_REVIEW",
      "canAppeal": boolan,
      "title": "COD:MW II"
    }
  ]
}
```

* Permaban Example:

```json
{
  "error": "",
  "success": "boolan",
  "canAppeal": boolan,
  "bans": [
    {
      "enforcement": "PERMANENT",
      "canAppeal": boolan,
      "title": "COD:MW3"
    },
    {
      "enforcement": "PERMANENT",
      "canAppeal": boolan,
      "title": "COD:MW II"
    }
  ]
}
```

* Permaban/Open Appeal Example:

```json
{
  "error": "",
  "success": "boolan",
  "canAppeal": boolan,
  "bans": [
    {
      "enforcement": "PERMANENT",
      "canAppeal": boolan,
      "bar": {
        "CaseNumber": "",
        "Status": "Open"
      },
      "title": "COD:MW3"
    },
    {
      "enforcement": "PERMANENT",
      "canAppeal": boolan,
      "bar": {
        "CaseNumber": "",
        "Status": "Open"
      },
      "title": "COD:MW II"
    }
  ]
}
```

* Permaban/Denied Appeal Example:

```json
{
  "error": "",
  "success": "boolan",
  "canAppeal": boolan,
  "bans": [
    {
      "enforcement": "PERMANENT",
      "canAppeal": boolan,
      "bar": {
        "CaseNumber": "",
        "Status": "Closed"
      },
      "title": "COD:MW3"
    },
    {
      "enforcement": "PERMANENT",
      "canAppeal": boolan,
      "bar": {
        "CaseNumber": "",
        "Status": "Closed"
      },
      "title": "COD:MW II"
    }
  ]
}
```

# Profile Endpoint Examples

* Profile Example: `?&accts=false` (Latest update)

```json
{
  "username": "",
  "email": "",
  "created": "ISO8601 Timestamp",
  "accounts": [],
  "tfaEnabled": boolan
}
```

* Profile Example: `?&accts=true` (Latest update)

```json
{
  "username": "",
  "email": "",
  "created": "ISO8601 Timestamp",
  "accounts": [
    {
      "username": "",
      "provider": "battle"
    },
    {
      "username": "",
      "provider": "steam"
    },
    {
      "username": "",
      "provider": "uno"
    },
    {
      "username": "",
      "provider": "xbl"
    },
    {
      "username": "",
      "provider": "psn"
    }
  ],
  "tfaEnabled": boolan
}
```

* UserInfo Endpoint Example:

```json
{
  "status": null,
  "exceptionMessageList": [],
  "errors": {},
  "exceptionMessageCode": null,
  "userInfo": {
    "userName": "",
    "friendCount": 0,
    "notificationCount": 0,
    "isAuthenticated": boolan,
    "profilImageUrl": null,
    "phoneNumber": "",
    "countryCode": "",
    "postalCode": "",
    "isElite": null,
    "isGraceLogin": boolan,
    "graceLoginCount": 0,
    "userNameEmpty": boolan,
    "jiveUserName": ""
  },
  "identities": [
    {
      "provider": "amazon",
      "username": null,
      "tokens": null,
      "authorized": boolan,
      "created": "",
      "updated": "",
      "accountID": null,
      "secondaryAccountID": null
    },
    {
      "provider": "facebook",
      "username": null,
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": null,
      "secondaryAccountID": null
    },
    {
      "provider": "gopenid",
      "username": null,
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": null,
      "secondaryAccountID": null
    },
    {
      "provider": "na",
      "username": null,
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": null,
      "secondaryAccountID": null
    },
    {
      "provider": "psn",
      "username": "",
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": null,
      "secondaryAccountID": null
    },
    {
      "provider": "steam",
      "username": "",
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": null,
      "secondaryAccountID": null
    },
    {
      "provider": "twitch",
      "username": "",
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": null,
      "secondaryAccountID": null
    },
    {
      "provider": "twitter",
      "username": "",
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": null,
      "secondaryAccountID": null
    },
    {
      "provider": "uno",
      "username": "",
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": "",
      "secondaryAccountID": null
    },
    {
      "provider": "xbl",
      "username": "",
      "tokens": null,
      "authorized": boolan,
      "created": "2020-01-01T13:00:00Z",
      "updated": "2020-01-01T13:00:00Z",
      "accountID": null,
      "secondaryAccountID": null
    }
  ],
  "sessionID": null,
  "accountPercentageCompletion": 100,
  "facebookLinked": boolan,
  "twitterLinked": boolan,
  "youtubeLinked": boolan,
  "twitchLinked": boolan,
  "amazonLinked": boolan,
  "emailValidated": boolan,
  "gamerAccountLinked": boolan,
  "codPreferences": {
    "news_and_community_updates": boolan,
    "news_and_community_updates_sms": boolan,
    "in_game_events": boolan,
    "in_game_events_sms": boolan,
    "gameplay_help_and_tips": boolan,
    "gameplay_help_and_tips_sms": boolan,
    "esports": boolan,
    "esports_sms": boolan,
    "sales_and_promotions": boolan,
    "sales_and_promotions_sms": boolan
  },
  "playerSupportPreferences": {
    "service_and_support": boolan,
    "service_and_support_sms": boolan
  }
}
```