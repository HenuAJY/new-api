# 文件上传 API 使用说明

本文面向接入 NewAPI 的下游开发者，说明如何申请临时上传地址并将文件直接上传到对象存储。

## 1. 接口概览

文件上传分为两个请求：

1. 调用 NewAPI 获取临时上传地址。
2. 使用临时地址将文件直接 `PUT` 到对象存储。

文件内容不会经过 NewAPI。NewAPI Key 只用于第一个请求，不能用于对象存储上传请求。

| 项目 | 内容 |
| --- | --- |
| 预签名接口 | `POST /v1/upload/presign` |
| 鉴权方式 | `Authorization: Bearer <NewAPI Key>` |
| 上传方法 | 使用响应中的 `method`，当前为 `PUT` |
| 上传地址 | 使用响应中的 `url` |
| 上传请求头 | 使用响应中的 `headers` |
| 上传有效期 | 使用响应中的 `expires_at`，Unix 秒 |

将 `https://your-new-api.example.com` 替换为实际 NewAPI 服务地址。

## 2. 获取上传地址

### 2.1 请求

```http
POST https://your-new-api.example.com/v1/upload/presign HTTP/1.1
Authorization: Bearer sk-your-new-api-key
Content-Type: application/json

{
  "filename": "photo.png",
  "content_type": "image/png"
}
```

### 2.2 请求参数

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `filename` | string | 是 | 原始文件名，最大 255 字节 |
| `content_type` | string | 否 | 文件 MIME 类型，例如 `image/png`；省略时使用 `application/octet-stream` |

`filename` 只用于保留安全的扩展名，服务端不会直接使用它作为对象路径。对象路径由服务端生成，调用方不能指定或覆盖对象键。

### 2.3 成功响应

HTTP 状态码为 `200`：

```json
{
  "url": "https://your-bucket.tos-cn-beijing.volces.com/uploads/123/2026/07/22/uuid.png?X-Tos-Signature=...",
  "public_url": "https://your-bucket.tos-cn-beijing.volces.com/uploads/123/2026/07/22/uuid.png",
  "method": "PUT",
  "object_key": "uploads/123/2026/07/22/uuid.png",
  "headers": {
    "Content-Type": "image/png"
  },
  "expires_at": 1784682900
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `url` | string | 临时上传 URL，必须原样使用，不要修改查询参数 |
| `public_url` | string | 对象固定地址；只有对象存储配置为公共读时才能直接访问 |
| `method` | string | 上传 HTTP 方法，当前为 `PUT` |
| `object_key` | string | 对象存储中的对象键，建议在业务数据库中保存 |
| `headers` | object | 上传时必须携带的签名请求头 |
| `expires_at` | integer | `url` 过期时间，Unix 秒 |

`url` 是临时敏感信息，不要长期写入日志或公开分享。`expires_at` 只表示上传 URL 的有效期，不代表文件会在该时间自动删除。

## 3. 上传文件

获取响应后，使用 `method`、`url` 和 `headers` 上传文件。

### 3.1 curl

```bash
curl -X PUT "$UPLOAD_URL" \
  -H "Content-Type: image/png" \
  --data-binary @photo.png
```

其中 `UPLOAD_URL` 是响应中的 `url`。如果响应的 `headers` 包含其他字段，也必须一并发送。

### 3.2 Node.js

Node.js 18 及以上版本可以使用项目提供的客户端：

```bash
NEW_API_BASE_URL=https://your-new-api.example.com \
NEW_API_KEY=sk-your-new-api-key \
node scripts/tos-upload-client.mjs --file ./photo.png
```

PowerShell：

```powershell
$env:NEW_API_BASE_URL = "https://your-new-api.example.com"
$env:NEW_API_KEY = "sk-your-new-api-key"
node scripts/tos-upload-client.mjs --file ".\photo.png"
```

如需指定媒体类型：

```bash
node scripts/tos-upload-client.mjs ./file.bin --content-type application/octet-stream
```

### 3.3 浏览器

浏览器上传时，先由后端或前端服务调用预签名接口，再使用返回的 URL 上传：

```javascript
const presignResponse = await fetch('/v1/upload/presign', {
  method: 'POST',
  headers: {
    Authorization: `Bearer ${newApiKey}`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify({
    filename: file.name,
    content_type: file.type || 'application/octet-stream',
  }),
})

if (!presignResponse.ok) {
  throw new Error('获取上传地址失败')
}

const presign = await presignResponse.json()
const uploadResponse = await fetch(presign.url, {
  method: presign.method,
  headers: presign.headers,
  body: file,
})

if (!uploadResponse.ok) {
  throw new Error(`文件上传失败：${uploadResponse.status}`)
}

console.log({
  objectKey: presign.object_key,
  publicUrl: presign.public_url,
})
```

浏览器直传还要求 TOS Bucket 配置允许当前前端域名的 `PUT` CORS 请求，并允许上传时使用的请求头，例如 `Content-Type`。

## 4. 上传成功后的文件地址

上传成功后，业务侧至少保存 `object_key`。`public_url` 是否能直接访问取决于 TOS 权限：

- Bucket 或对象允许公共读：可以直接使用 `public_url`。
- Bucket 为私有：访问 `public_url` 会返回 `403`，需要由业务后端另外生成 `GET` 预签名下载 URL。

不要根据客户端文件名自行拼接对象地址，应该使用接口返回的 `object_key` 或 `public_url`。

## 5. 错误处理

### 5.1 NewAPI 响应错误

```json
{
  "error": {
    "message": "upload service is not configured",
    "type": "new_api_error",
    "param": "",
    "code": "upload_service_unavailable"
  }
}
```

| 状态码 | 错误码或现象 | 处理建议 |
| --- | --- | --- |
| `400` | `invalid_request_body` | 检查 JSON 格式和 `Content-Type` |
| `400` | `invalid_upload_request` | 检查 `filename` 和 `content_type` |
| `401` | 令牌无效或未提供 | 检查 `Authorization` 是否为有效 NewAPI Key |
| `403` | 用户、令牌或 IP 无权限 | 检查令牌状态、IP 限制和用户状态 |
| `429` | 触发上传限流 | 默认每个 API Key 60 秒最多申请 100 次，等待后重新申请上传地址 |
| `503` | `upload_service_unavailable` | 联系 NewAPI 管理员检查 TOS 配置 |
| `500` | `upload_presign_failed` | 稍后重试并检查服务端日志 |

### 5.2 TOS 上传响应错误

| 状态码或现象 | 常见原因 | 处理建议 |
| --- | --- | --- |
| `403 SignatureDoesNotMatch` | 修改了 `url` 或签名请求头不一致 | 原样使用 `url` 和 `headers`，尤其是 `Content-Type` |
| `403 AccessDenied` | URL 过期或 TOS 权限不足 | 重新申请 URL，并检查 Bucket/IAM 权限 |
| `404 NoSuchBucket` | Bucket 或区域配置错误 | 联系 NewAPI 管理员检查 TOS 配置 |
| 网络超时 | 文件较大或网络不稳定 | 重新申请 URL 后重试上传 |
| 上传成功但 `public_url` 返回 `403` | 文件不是公共读 | 使用后端生成的临时下载 URL |

预签名 URL 过期后不可复用。重试时必须重新调用 `/v1/upload/presign`。

## 6. 调用频率限制

预签名接口默认按 API Key 对应的令牌 ID进行限流：

- 每个 API Key 默认每 60 秒最多申请 100 个预签名 URL。
- 不同 API Key 分别计算，不会因为共用同一个 OpenResty、NAT 或代理 IP 而共享额度。
- 实际文件从客户端直传 TOS 的 `PUT` 请求不计入此限制。
- 只有调用 `POST /v1/upload/presign` 才会消耗额度。

管理员可以通过环境变量调整：

```env
UPLOAD_RATE_LIMIT_ENABLE=true
UPLOAD_RATE_LIMIT=100
UPLOAD_RATE_LIMIT_DURATION=60
```

关闭限流：

```env
UPLOAD_RATE_LIMIT_ENABLE=false
```

修改环境变量后需要重启或重新创建 NewAPI 服务。触发限制时接口返回 `429 Too Many Requests`，客户端应等待一段时间后重新申请 URL，不要在短时间内无限重试。

## 7. 安全要求

- NewAPI Key 只发送给 NewAPI，不要放入 TOS `PUT` 请求。
- 不要把 `TOS_ACCESS_KEY` 或 `TOS_SECRET_KEY` 放到客户端代码中。
- 不要修改预签名 URL 的查询参数。
- 生产环境调用 NewAPI 应使用 HTTPS。
- 不要把完整的预签名 URL 写入长期日志。
- 私有文件不要依赖 `public_url`，应保存 `object_key` 并通过后端授权下载。

## 8. 最小接入示例

```bash
# 1. 获取预签名信息
curl -s https://your-new-api.example.com/v1/upload/presign \
  -H "Authorization: Bearer sk-your-new-api-key" \
  -H "Content-Type: application/json" \
  -d '{"filename":"photo.png","content_type":"image/png"}'

# 2. 使用上一步返回的 url 和 headers 上传文件
curl -X PUT "<response.url>" \
  -H "Content-Type: image/png" \
  --data-binary @photo.png

# 3. 业务侧保存 response.object_key；公共读对象可使用 response.public_url
```
