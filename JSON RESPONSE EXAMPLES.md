# Ban Check Endpoint Examples

* No ban Example:

```json
{"error":"","success":"true","canAppeal":false,"bans":[]}
````

* Shadowban Example:
```json
{"error":"","success":"true","canAppeal":false,"bans":[{"enforcement":"UNDER_REVIEW","canAppeal":false,"title":"COD:MW3"},{"enforcement":"UNDER_REVIEW","canAppeal":false,"title":"COD:MW II"}]}
```

* Permaban Example:
```json
{"error":"","success":"true","canAppeal":true,"bans":[{"enforcement":"PERMANENT","canAppeal":true,"title":"COD:MW3"},{"enforcement":"PERMANENT","canAppeal":true,"title":"COD:MW II"}]}
```

* Permaban/Open Appeal Example:

```json
{"error":"","success":"true","canAppeal":false,"bans":[{"enforcement":"PERMANENT","canAppeal":false,"bar":{"CaseNumber":"","Status":"Open"},"title":"COD:MW3"},{"enforcement":"PERMANENT","canAppeal":false,"bar":{"CaseNumber":"","Status":"Open"},"title":"COD:MW II"}]}
```

* Permaban/Denied Appeal Example:
```json
{"error":"","success":"true","canAppeal":false,"bans":[{"enforcement":"PERMANENT","canAppeal":false,"bar":{"CaseNumber":"","Status":"Closed"},"title":"COD:MW3"},{"enforcement":"PERMANENT","canAppeal":false,"bar":{"CaseNumber":"","Status":"Closed"},"title":"COD:MW II"}]}
```

# Profile Endpoint Examples

* Profile Example: `?&accts=false`

```json
{"username":"","email":"","created":"","accounts":[]}
```

* Profile Example: `?&accts=true`

```json
{"username":"","email":"","created":"","accounts":[{"username":"","provider":"battle"},{"username":"","provider":"steam"},{"username":"","provider":"uno"},{"username":"","provider":"xbl"},{"username": "","provider": "psn"}]}
```

* UserInfo Endpoint Example:
```json
{"status": null, "exceptionMessageList": [], "errors": {}, "exceptionMessageCode": null, "userInfo": {"userName": "", "friendCount": 0, "notificationCount": 0, "isAuthenticated": true, "profilImageUrl": null, "phoneNumber": "", "countryCode": "", "postalCode": "", "isElite": null, "isGraceLogin": false, "graceLoginCount": 0, "userNameEmpty": false, "jiveUserName": ""}, "identities": [{"provider": "amazon", "username": null, "tokens": null, "authorized": true, "created": "", "updated": "", "accountID": null, "secondaryAccountID": null}, {"provider": "facebook", "username": null, "tokens": null, "authorized": true, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": null, "secondaryAccountID": null}, {"provider": "gopenid", "username": null, "tokens": null, "authorized": false, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": null, "secondaryAccountID": null}, {"provider": "na", "username": null, "tokens": null, "authorized": true, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": null, "secondaryAccountID": null}, {"provider": "psn", "username": "", "tokens": null, "authorized": true, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": null, "secondaryAccountID": null}, {"provider": "steam", "username": "", "tokens": null, "authorized": true, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": null, "secondaryAccountID": null}, {"provider": "twitch", "username": "", "tokens": null, "authorized": true, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": null, "secondaryAccountID": null}, {"provider": "twitter", "username": "", "tokens": null, "authorized": true, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": null, "secondaryAccountID": null}, {"provider": "uno", "username": "", "tokens": null, "authorized": true, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": "", "secondaryAccountID": null}, {"provider": "xbl", "username": "", "tokens": null, "authorized": true, "created": "2020-01-01T13:00:00Z", "updated": "2020-01-01T13:00:00Z", "accountID": null, "secondaryAccountID": null}], "sessionID": null, "accountPercentageCompletion": 100, "facebookLinked": true, "twitterLinked": true, "youtubeLinked": false, "twitchLinked": true, "amazonLinked": true, "emailValidated": true, "gamerAccountLinked": true, "codPreferences": {"news_and_community_updates": false, "news_and_community_updates_sms": true, "in_game_events": false, "in_game_events_sms": true, "gameplay_help_and_tips": true, "gameplay_help_and_tips_sms": true, "esports": false, "esports_sms": true, "sales_and_promotions": false, "sales_and_promotions_sms": true}, "playerSupportPreferences": {"service_and_support": true, "service_and_support_sms": true}}
```