Request host: `https://api.ez-captcha.com`

Request API: `https://api.ez-captcha.com/getTaskResult`

Request format: `POST application/json`

Result token can be queried within 5 minutes after each task is created

### Request Parameters

| Parameters | Type   | Required | Description                                                   |
|------------|--------|----------|---------------------------------------------------------------|
| clientKey  | string | true     | Account client key, which can be found in the personal center |
| taskId     | string | true     | The task ID created by the [createTask](createtask.md) method |

### Request Example

```http request
POST https://api.ez-captcha.com/getTaskResult

Content-Type: application/json

{
"clientKey": "YOUR_API_KEY",
"taskId": "TASKID OF CREATETASK" // ID created by createTask method
}
```

### Response Data

| Parameters       | Type    | Description                                                                                                           |
|------------------|---------|-----------------------------------------------------------------------------------------------------------------------|
| errorId          | Integer | Error message: 0 - no error, 1 - error                                                                                |
| errorCode        | string  | Error Code                                                                                                            |
| errorDescription | string  | Detailed error description                                                                                            |
| status           | String  | **processing** - task is in progress <br> **ready** - task is complete, the result is found in the solution parameter |
| solution         | Object  | Task results, different types of task results will be different.                                                      |

### Response Example

```json lines
{
"errorId": 0,
"errorCode": null,
"errorDescription": null,
"solution": {
"gRecaptchaResponse": "03AGdBq25SxXT-pmSeBXjzScW-EiocHwwpwqtk1QXlJnGnUJCZrgjwLLdt7cb0..."
},
"status": "ready"
}
```