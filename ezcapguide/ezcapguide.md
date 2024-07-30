### API Basic Instructions

- [Get Balance](getbalance.md)
- [Create Task](createtask.md)
- [Task Result](gettask.md)
- [Error code list](errorcode.md)



### Task (Token)

- [ReCaptcha V2](https://ezcaptcha.atlassian.net/wiki/wiki/spaces/IS/pages/7045286/ReCaptcha+V2)



Here's the HTML content converted to Markdown:

```markdown
## Create a task through the createTask method, and then get the result through the getTaskResult method

---

**Note:**  
Create a task through the [createTask](https://ezcaptcha.atlassian.net/wiki/spaces/IS/pages/7045215/createTask+Create+Task) method, and then get the result through the [getTaskResult](https://ezcaptcha.atlassian.net/wiki/spaces/IS/pages/7045230/getTaskResult+Task+Result) method.

---

**Info:**  
If you obtain an invalid token, please contact us. It will usually work normally after we optimize it.

---

- View [Online Code Examples](https://apifox.com/apidoc/shared-7ec798b7-6564-46b5-ad58-b2044f4ebfaf/api-114325511) now!

---

### Task Type

For the solution of ReCaptcha V2, the task types we provide are as follows:

| Task Type                      | Description                                                                                                  | Price      | Price (USD) |
|--------------------------------|--------------------------------------------------------------------------------------------------------------|------------|-------------|
| ReCaptchaV2TaskProxyless       | reCaptcha V2 solution, using server built-in proxy                                                          | ![6 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=6+POINTS&colour=Blue) | $0.6/1k   |
| ReCaptchaV2TaskProxylessS9     | reCaptcha V2 High-scoring solutions, use the server's built-in proxy and make the returned token score at least 0.9 | ![12 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=12+POINTS&colour=Blue) | $1.2/1k   |
| ReCaptchaV2STaskProxyless      | reCaptcha V2 with **s** parameter solution, using server built-in proxy                                      | ![6 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=6+POINTS&colour=Blue) | $0.6/1k   |
| ReCaptchaV2EnterpriseTaskProxyless | reCaptcha V2 Enterprise solution, using server built-in proxy                                              | ![12 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=12+POINTS&colour=Purple) | $1.2/1k   |
| ReCaptchaV2SEnterpriseTaskProxyless | reCaptcha V2 Enterprise with **s** parameter solution, using server built-in proxy                           | ![12 POINTS](https://ezcaptcha.atlassian.net/wiki/plugins/servlet/status-macro/placeholder?title=12+POINTS&colour=Purple) | $1.2/1k   |

---

## Create Task

Create a task through the createTask method

- **Request host:** `https://api.ez-captcha.com`
- **Request API:** `https://api.ez-captcha.com/createTask`
- **Request format:** `POST` `application/json`

---

### Parameter Structure

| Parameter    | Type    | Required | Description                                                                                                 |
|--------------|---------|----------|-------------------------------------------------------------------------------------------------------------|
| clientKey    | string  | true     | Your client key                                                                                             |
| type         | string  | true     | Task type, such as ReCaptchaV2TaskProxyless                                                                 |
| websiteURL   | string  | true     | ReCaptcha website URL, usually a fixed value                                                                |
| websiteKey   | string  | true     | ReCaptcha site key, usually a fixed value                                                                   |
| isInvisible  | Bool    | false    | For the reCaptcha V2 type, if it is an invisible type of reCaptchaV2, you need to add this parameter and set it to true. If this parameter is not filled, it defaults to false. |
| sa           | string  | false    | For reCaptchaV2 (including v2 enterprise), if **”sa“** appears in the query parameter of the **anchor** request, you will need to set it to the value of sa query parameter, which is usually a string.  *Some websites may strictly verify this value* |
| s            | string  | false    | Only used to solve reCaptcha V2 with s parameter, including V2 and V2 Enterprise, you need to specify the type as ReCaptchaV2STaskProxyless or RecaptchaV2SEnterpriseTaskProxyless, find this parameter from the data packet returned from the website you are using and set it. |

---

### Request Example

```plaintext
POST https://api.ez-captcha.com/createTask

Content-Type: application/json

{
    "clientKey": "cc9c18d3e263515c2c072b36a7125eecc078618f",
    "task": {
        "websiteURL": "https://www.google.com/recaptcha/api2/demo",
        "websiteKey": "6Le-wvkSAAAAAPBMRTvw0Q4Muexq9bi0DJwx_mJ-",
        "type": "ReCaptchaV2TaskProxyless",
        "isInvisible": false // reCaptcha V2 of invisible type sets this value to true
    }
}
```

---

### Response Example

```plaintext
{
    "errorId": 0,
    "errorCode": "",
    "errorDescription": "",
    "taskId": "61138bb6-19fb-11ec-a9c8-0242ac110006" // Please save this ID for next step
}
```

---




## Get Result

Use the  method to get the recognition result
[gettask.md](gettask.md)
- **Request host:** `https://api.ez-captcha.com`
- **Request API:** `https://api.ez-captcha.com/getTaskResult`
- **Request format:** `POST` `application/json`

Depending on system health, you will get results in 10s to 80s interval with 120s timeout

---

### Request Example

```plaintext
POST https://api.ez-captcha.com/getTaskResult

Content-Type: application/json
 
{
    "clientKey":"YOUR_API_KEY",
    "taskId": "TASKID OF CREATETASK" //ID created by createTask method
}
```

---

### Response Data

| Parameter           | Type    | Description                                                                                          |
|---------------------|---------|------------------------------------------------------------------------------------------------------|
| errorId             | Integer | Error message: 0 - no error, 1 - error                                                              |
| errorCode           | string  | Error code, [errorcode](errorcode.md) to view all error list |
| errorDescription    | string  | Detailed error description                                                                          |
| status              | String  | **processing** - task is in progress, please try again in 3 seconds <br> **ready** - task is complete, find the result in the solution parameter |
| solution            | Object  | The recognition result will be different for different types of captcha. For reCaptcha, the result is gRecaptchaResponse in the object. |
| solution.gRecaptchaResponse | string | Recognition result: response value, used for POST or simulated submission to the target website. The validity period is generally 120s, it is recommended to use within 60s |

---

### Response Example

```plaintext
{
    "errorId": 0,
    "errorCode": null,
    "errorDescription": null,
    "solution": {
        "gRecaptchaResponse": "03AGdBq25SxXT-pmSeBXjzScW-EiocHwwpwqtk1QXlJnGnU......"
    },
    "status": "ready"
}
```

---

### Response Description

- **Successful recognition:** when errorId is equal to 0 and status is equal to ready, the result is in the solution.
- **Identifying:** When errorId is 0 and status is processing, please try again after 3 seconds.
- **An error occurred:** when the errorId is greater than 0, please understand the error information according to the errorDescription [All error descriptions](errorcode.md)