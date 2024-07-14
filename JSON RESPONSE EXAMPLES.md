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
