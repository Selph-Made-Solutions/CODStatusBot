# createTask method

The method is used to create a new captcha recognition task for a selected captcha task type. Returns the `id` of the task or an [error code](/api-docs/error-codes).

**API Endpoint:** `https://api.2captcha.com/createTask`  
**Method:** `POST`  
**Content-Type:** `application/json`

## Request properties

| Name         | Type     | Required | Description                                                                 |
|--------------|----------|----------|-----------------------------------------------------------------------------|
| **clientKey**| *String* | Yes      | Your [API key](/enterpage)                                                 |
| **task**     | *Object* | Yes      | Task object, see [Captcha task types](/api-docs)                          |
| languagePool | *String* | No       | Used to choose the workers for solving the captcha by their language. Applicable to image-based and text-based captchas. <br> Default: `en`. <br> `en` - English-speaking workers <br> `rn` - Russian-speaking workers. |
| callbackUrl  | *String* | No       | URL of your web [registered web server](/setting/pingback) used to receive and process the captcha resolution result |
| softId       | *Integer*| No       | The ID of your software registered in our [Software catalog](/software)   |

## Request example

```json
{
    "clientKey": "YOUR_API_KEY",
    "task": {
        "type": "HCaptchaTaskProxyless",
        "websiteURL": "https://2captcha.com/demo/hcaptcha",
        "websiteKey": "f7de0da3-3303-44e8-ab48-fa32ff8ccc7b"
    }
}
```

## Response example

```json
{
    "errorId": 0,
    "taskId": 72345678901
}
```