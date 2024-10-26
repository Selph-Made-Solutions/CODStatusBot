# getTaskResult method

Returns the result for a task.  
The result format depends on task type and described in the task specification.

**API Endpoint:** `https://api.2captcha.com/getTaskResult`  
**Method:** `POST`  
**Content-Type:** `application/json`

## Request properties

| Name        | Type      | Required | Description                       |
|-------------|-----------|----------|-----------------------------------|
| **clientKey** | *String* | **Yes**  | Your [API key](https://2captcha.com/enterpage) |
| **taskId**    | *Integer* | **Yes**  | The id of your task              |

## Request example

```json
{
   "clientKey": "YOUR_API_KEY", 
   "taskId": 74372499131
}
```

## Response examples

### In progress

When the task is not complete yet, you receive the following response. Wait at least 5 seconds and repeat the request.

```json
{
    "errorId": 0,
    "status": "processing"
}
```

### Task could not be completed

If workers were unable to complete the task you get the response containing the [error id](https://2captcha.com/api-docs/error-codes).

```json
{
    "errorId": 12,
    "errorCode": "ERROR_CAPTCHA_UNSOLVABLE",
    "errorDescription": "Workers could not solve the Captcha"
}
```

### Task completed

When the task is completed you receive the solution according to the task type format and some common task data like the timestamps, price, IP that submitted the request.

```json
{
    "errorId": 0,
    "status": "ready",
    "solution": {},
    "cost": "0.00299",
    "ip": "1.2.3.4",
    "createTime": 1692863536,
    "endTime": 1692863556,
    "solveCount": 1
}
```

#### Response specification

| Property     | Type      | Description                                                                            |
|--------------|-----------|----------------------------------------------------------------------------------------|
| errorId      | *Integer* | The [error id](https://2captcha.com/api-docs/error-codes) for cases when the task cannot be completed |
| status       | *String*  | **ready** - the task is completed successfully <br> **processing** - still processing your task, please repeat the request again in 5-10 seconds |
| solution     | *Object*  | An object containing the solution for your task. The format can be found in task type specification |
| cost         | *String*  | The task price charged from your balance                                              |
| ip           | *String*  | The IP address that submitted the task request                                         |
| createTime   | *Integer* | Timestamp indicating the moment the task was submitted                                 |
| endTime      | *Integer* | Timestamp indicating the moment the task was completed                                 |
| solveCount   | *Integer* | The number of workers that attempted to complete your task                             |