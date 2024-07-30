Request host: `https://api.ez-captcha.com`

Request api: `https://api.ez-captcha.com/createTask`

Request format: `POST` `application/json`

### Task Type

| Parameters                        | Description                                                                                   | Price                                              |
|-----------------------------------|-----------------------------------------------------------------------------------------------|----------------------------------------------------|
| ReCaptchaV2TaskProxyless           | reCaptcha V2 solution, using server built-in proxy                                           | ![6 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=6+POINTS&colour=Blue)  |
| ReCaptchaV2TaskProxylessS9         | reCaptcha V2 High-scoring solutions, use the server's built-in proxy and make the returned token score at least 0.9 | ![12 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=12+POINTS&colour=Blue) |
| RecaptchaV2EnterpriseTaskProxyless | reCaptcha V2 Enterprise solution, using server built-in proxy                                 | ![18 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=18+POINTS&colour=Purple) |
| ReCaptchaV2STaskProxyless          | reCaptcha V2 with **s** parameter solution, using server built-in proxy                        | ![15 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=15+POINTS&colour=Blue)  |
| RecaptchaV2SEnterpriseTaskProxyless| reCaptcha V2 Enterprise with **s** parameter solution, using server built-in proxy              | ![30 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=30+POINTS&colour=Purple) |
| ReCaptchaV3TaskProxyless           | reCaptcha V3 solution, using server built-in proxy                                            | ![8 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=8+POINTS&colour=Blue)   |
| ReCaptchaV3TaskProxylessS9         | reCaptcha V3 High-scoring solutions, use the server's built-in proxy and make the returned token score at least 0.9 | ![15 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=15+POINTS&colour=Blue)  |
| ReCaptchaV3EnterpriseTaskProxyless | reCaptcha V3 Enterprise solution, using server built-in proxy                                  | ![18 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=18+POINTS&colour=Purple) |
| FuncaptchaTaskProxyless            | Funcaptcha solution, using server built-in proxy                                              | ![18 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=18+POINTS&colour=Blue)  |

### Request Parameters

Take reCaptcha as example

| Parameters      | Type   | Required | Description                                                                                                           |
|-----------------|--------|----------|-----------------------------------------------------------------------------------------------------------------------|
| clientKey       | string | true     | Account client key, which can be found in the personal center                                                          |
| task            | object | true     | The task parameter object, the details are the following items in this table                                           |
| type            | string | true     | Task type, such as ReCaptchaV2TaskProxyless                                                                            |
| websiteURL      | string | true     | Website URL using ReCaptcha, usually a fixed value                                                                     |
| websiteKey      | string | true     | ReCaptcha site key, a fixed value                                                                                      |
| isInvisible     | Bool   | false    | For the reCaptcha V2 type, if it is an invisible type of reCaptchaV2, you need to add this parameter and set it to true. If this parameter is not filled, it defaults to false. For the reCaptcha V3 type, this parameter defaults to true |
| pageAction      | string | false    | This parameter is only for reCaptcha V3 and generally needs to be filled in. If it is not filled in, an empty parameter will be used by default, which will greatly affect the token accuracy |
| s               | string | false    | Only used to solve reCaptcha V2 with s parameter, including V2 and V2 Enterprise, you need to specify the type as ReCaptchaV2STaskProxyless or RecaptchaV2SEnterpriseTaskProxyless, find this parameter from the data packet returned from the website you are using and set it. |

### Request Example

ReCaptcha V3

```
POST https://api.ez-captcha.com/createTask

Content-Type: application/json
{
"clientKey":"yourapiKey",
"task": {
"type":"ReCaptchaV3TaskProxyless",
"websiteURL":"https://recaptcha-demo.appspot.com/recaptcha-v3-request-scores.php",
"websiteKey":"6LdyC2cUAAAAACGuDKpXeDorzUDWXmdqeg-xy696",
"pageAction": "examples/v3scores",
"isInvisible": true
}
}
```

### Response Data

| Parameters          | Type    | Description                                                                                 |
|---------------------|---------|---------------------------------------------------------------------------------------------|
| errorId             | Integer | Error message: 0 - no error, 1 - error                                                     |
| errorCode           | string  | Error Code                                                                                  |
| errorDescription    | string  | Detailed error description                                                                  |
| taskId              | string  | Created task ID, use the [getTaskResult](https://ezcaptcha.atlassian.net/wiki/spaces/IS/pages/7045230/getTaskResult+Task+Result) interface to get the token result |

### Response Example

```
{
"errorId": 0,
"errorCode": "",
"errorDescription": "",
"taskId": "2376919c-1863-11ec-a012-94e6f7355a0b" // Please save this ID for the next step
}
```