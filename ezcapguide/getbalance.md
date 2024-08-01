Request API: `https://api.ez-captcha.com/getBalance`

Request format: `POST` `application/json`

### Request Parameters

| **Parameters** | **Type** | **Required** | **Description**                                               |
|----------------|----------|--------------|---------------------------------------------------------------|
| clientKey      | string   | true         | Account client key, which can be found in the personal center |

### Request Example

```json lines
{
    "clientKey": "cc9c18d3e263515c2c072b36a7125eecc078618f"
}
```

### Response Data

| **Parameters** | **Type** | **Description**                               |
|----------------|----------|-----------------------------------------------|
| errorId        | Integer  | Error message: 0: no error, 1: error          |
| balance        | Decimal  | Account balance (points) 1 USD = 10000 points |

### Response Example

```json lines
{
    "errorId": 0,
    "balance": 1071810
}
```