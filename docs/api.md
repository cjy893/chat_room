# Socket API 文档

## 概述

这是一个实时聊天系统的API文档，提供用户注册、登录、房间管理、消息收发等功能。

Base URL: `/api/v1`

## 认证方式

大部分API都需要通过JWT Token进行认证。在需要认证的接口请求中，在HTTP头中加入：

```
Authorization: Bearer <token>
```

其中`<token>`是在登录或注册成功后返回的访问令牌。

## 状态码说明

| 状态码 | 说明           |
| ------ | -------------- |
| 200    | 请求成功       |
| 400    | 请求参数错误   |
| 401    | 未认证         |
| 403    | 禁止访问       |
| 404    | 资源不存在     |
| 409    | 资源冲突       |
| 500    | 服务器内部错误 |

## 数据格式

所有API响应遵循统一的数据格式：

```json
{
  "code": 200,
  "message": "success",
  "data": {}
}
```

---

## 接口列表

### 公共接口

#### 用户注册

**POST** `/api/v1/register`

##### 请求参数

| 参数名   | 类型   | 必填 | 说明     |
| -------- | ------ | ---- | -------- |
| username | string | 是   | 用户名   |
| password | string | 是   | 密码     |
| email    | string | 是   | 邮箱地址 |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "user_id": "用户ID",
    "username": "用户名",
    "token": "访问令牌"
  }
}
```

#### 用户登录

**POST** `/api/v1/login`

##### 请求参数

| 参数名    | 类型   | 必填 | 说明         |
| --------- | ------ | ---- | ------------ |
| username  | string | 是   | 用户名       |
| password  | string | 是   | 密码         |
| ip        | string | 否   | IP地址       |
| user_agent| string | 否   | 用户代理信息 |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "user_id": "用户ID",
    "username": "用户名",
    "token": "访问令牌"
  }
}
```

### 用户相关接口

需要认证的接口，需在请求头中携带有效的JWT Token。

#### 获取在线用户列表

**GET** `/api/v1/users` 或 `/api/v1/users/online`

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "user_id": "用户ID",
      "username": "用户名",
      "is_online": true,
      "last_seen": "最后在线时间"
    }
  ]
}
```

### 好友相关接口

需要认证的接口，需在请求头中携带有效的JWT Token。

#### 添加好友

**POST** `/api/v1/friends/add`

##### 请求参数

| 参数名   | 类型   | 必填 | 说明   |
| -------- | ------ | ---- | ------ |
| friend_id| string | 是   | 好友ID |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "message": "好友请求已发送"
  }
}
```

#### 删除好友

**POST** `/api/v1/friends/remove`

##### 请求参数

| 参数名   | 类型   | 必填 | 说明   |
| -------- | ------ | ---- | ------ |
| friend_id| string | 是   | 好友ID |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "message": "好友关系已解除"
  }
}
```

#### 获取好友列表

**GET** `/api/v1/friends`

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": "用户ID",
      "username": "用户名",
      "avatar": "头像URL",
      "status": "用户状态(online, offline, busy, away)",
      "last_seen": "最后在线时间"
    }
  ]
}
```

#### 获取好友请求列表

**GET** `/api/v1/friends/requests`

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": "好友关系ID",
      "user_id": "请求发起者ID",
      "friend_id": "请求接收者ID",
      "status": "状态(pending, accepted, blocked)",
      "created_at": "创建时间"
    }
  ]
}
```

#### 接受好友请求

**POST** `/api/v1/friends/accept`

##### 请求参数

| 参数名    | 类型   | 必填 | 说明     |
| --------- | ------ | ---- | -------- |
| request_id| string | 是   | 请求ID   |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "message": "好友请求已接受"
  }
}
```

#### 屏蔽用户

**POST** `/api/v1/friends/block`

##### 请求参数

| 参数名  | 类型   | 必填 | 说明   |
| ------- | ------ | ---- | ------ |
| user_id | string | 是   | 用户ID |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "message": "用户已屏蔽"
  }
}
```

### 文件相关接口

需要认证的接口，需在请求头中携带有效的JWT Token。

#### 上传文件

**POST** `/api/v1/upload`

##### 请求参数

| 参数名 | 类型 | 必填 | 说明     |
| ------ | ---- | ---- | -------- |
| file   | file | 是   | 上传文件 |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "url": "文件访问URL",
    "file_name": "原始文件名",
    "file_size": 文件大小(字节)
  }
}
```

### 聊天相关接口

#### 获取聊天记录

**GET** `/api/v1/rooms/{room_id}/messages`

##### 请求参数

| 参数名  | 类型   | 必填 | 默认值 | 说明     |
| ------- | ------ | ---- | ------ | -------- |
| room_id | string | 是   | -      | 房间ID   |
| page    | string | 否   | 1      | 页码     |
| limit   | string | 否   | 50     | 每页条数 |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": "消息ID",
      "sender_id": "发送者ID",
      "content": "消息内容",
      "created_at": "创建时间"
    }
  ]
}
```

#### 搜索消息

**GET** `/api/v1/rooms/{room_id}/messages/search`

##### 请求参数

| 参数名   | 类型   | 必填 | 说明   |
| -------- | ------ | ---- | ------ |
| room_id  | string | 是   | 房间ID |
| keyword  | string | 是   | 关键词 |
| limit    | string | 否   | 限制数 |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": "消息ID",
      "sender_id": "发送者ID",
      "content": "消息内容",
      "created_at": "创建时间"
    }
  ]
}
```

#### 获取未读消息数

**GET** `/api/v1/rooms/{room_id}/unread`

##### 请求参数

| 参数名  | 类型   | 必填 | 说明   |
| ------- | ------ | ---- | ------ |
| room_id | string | 是   | 房间ID |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "room_id": "房间ID",
    "unread_count": 未读消息数
  }
}
```

#### 标记消息为已读

**POST** `/api/v1/rooms/{room_id}/read`

##### 请求参数

| 参数名     | 类型     | 必填 | 说明     |
| ---------- | -------- | ---- | -------- |
| room_id    | string   | 是   | 房间ID   |
| message_ids| []string | 是   | 消息ID数组 |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": null
}
```

#### 撤回消息

**DELETE** `/api/v1/messages/{message_id}`

##### 请求参数

| 参数名     | 类型   | 必填 | 说明   |
| ---------- | ------ | ---- | ------ |
| message_id | string | 是   | 消息ID |
| reason     | string | 否   | 撤回原因 |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": null
}
```

### 房间相关接口

#### 创建聊天室

**POST** `/api/v1/rooms`

##### 请求参数

| 参数名      | 类型     | 必填 | 说明         |
| ----------- | -------- | ---- | ------------ |
| name        | string   | 是   | 房间名称     |
| description | string   | 否   | 房间描述     |
| type        | string   | 是   | 房间类型     |
| is_public   | boolean  | 否   | 是否公开     |
| member_ids  | []string | 否   | 成员ID列表   |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "id": "房间ID",
    "name": "房间名称",
    "description": "房间描述",
    "type": "房间类型",
    "creator_id": "创建者ID",
    "is_public": true,
    "members": [
      {
        "id": "成员关系ID",
        "room_id": "房间ID",
        "user_id": "用户ID",
        "role": "角色(owner, admin, member)",
        "joined_at": "加入时间"
      }
    ]
  }
}
```

#### 加入房间

**POST** `/api/v1/rooms/{room_id}/join`

##### 请求参数

| 参数名  | 类型   | 必填 | 说明   |
| ------- | ------ | ---- | ------ |
| room_id | string | 是   | 房间ID |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "message": "成功加入房间"
  }
}
```

#### 离开房间

**POST** `/api/v1/rooms/{room_id}/leave`

##### 请求参数

| 参数名  | 类型   | 必填 | 说明   |
| ------- | ------ | ---- | ------ |
| room_id | string | 是   | 房间ID |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "message": "成功离开房间"
  }
}
```

#### 获取房间成员

**GET** `/api/v1/rooms/{room_id}/members`

##### 请求参数

| 参数名  | 类型   | 必填 | 说明   |
| ------- | ------ | ---- | ------ |
| room_id | string | 是   | 房间ID |

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": "成员关系ID",
      "room_id": "房间ID",
      "user_id": "用户ID",
      "role": "角色(owner, admin, member)",
      "joined_at": "加入时间"
    }
  ]
}
```

#### 获取用户房间列表

**GET** `/api/v1/user/rooms`

##### 响应结果

```json
{
  "code": 200,
  "message": "success",
  "data": [
    {
      "id": "房间ID",
      "name": "房间名称",
      "description": "房间描述",
      "type": "房间类型",
      "creator_id": "创建者ID",
      "is_public": true,
      "members": [
        {
          "id": "成员关系ID",
          "room_id": "房间ID",
          "user_id": "用户ID",
          "role": "角色(owner, admin, member)",
          "joined_at": "加入时间"
        }
      ]
    }
  ]
}
```

### WebSocket 接口

#### 建立WebSocket连接

**GET** `/ws`

##### 查询参数

| 参数名   | 类型   | 必填              | 说明                                  |
| -------- | ------ | ----------------- | ------------------------------------- |
| user_id  | string | 否(可通过JWT获取) | 用户ID                                |
| username | string | 否(可通过JWT获取) | 用户名                                |

或者使用JWT认证，在请求头中添加：
```
Authorization: Bearer <token>
```

#### 建立房间WebSocket连接

**GET** `/ws/rooms/{room_id}`

##### 请求参数

| 参数名  | 类型   | 必填 | 说明     |
| ------- | ------ | ---- | -------- |
| room_id | string | 是   | 房间ID   |

##### 认证方式

必须在请求头中携带有效的JWT Token：
```
Authorization: Bearer <token>
```

建立连接后会自动加入指定房间。