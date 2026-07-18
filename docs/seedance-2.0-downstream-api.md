# Seedance 2.0 视频生成 API 对接文档

本文档面向调用 Seedance 2.0 视频生成服务的客户端。接口采用异步任务模式：先创建任务获得任务 ID，再通过任务 ID 轮询查询生成状态和视频地址。

> 请求中的 `model` 必须使用服务提供方分配的模型名称。示例中的 `YOUR_MODEL_NAME` 需要替换为实际可用的模型名称。

## 1. 基础信息

### 1.1 Base URL

请将下列地址替换为实际部署地址：

```text
https://api.example.com
```

### 1.2 鉴权

所有接口均使用 API Key，通过 Bearer Token 传递：

```http
Authorization: Bearer YOUR_API_KEY
```

### 1.3 Content-Type

创建任务时使用：

```http
Content-Type: application/json
```

### 1.4 接口列表

| 功能 | 方法 | 路径 |
| --- | --- | --- |
| 创建视频任务 | `POST` | `/api/v3/contents/generations/tasks` |
| 查询视频任务 | `GET` | `/api/v3/contents/generations/tasks/{task_id}` |

## 2. 创建视频任务

```http
POST /api/v3/contents/generations/tasks
```

### 2.1 请求字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 服务提供方分配的模型名称 |
| `content` | array | 是 | 提示词和多媒体参考内容；必须至少包含一项有效的文本内容 |
| `resolution` | string | 否 | 期望的视频分辨率，例如 `720p`；实际可用值以模型能力为准 |
| `ratio` | string | 否 | 期望的宽高比，例如 `16:9`、`9:16`、`1:1` |
| `duration` | integer | 否 | 期望的视频时长，单位为秒；Seedance 2.0 通常使用 4-15 秒，最终以模型能力为准 |

### 2.2 content 内容类型

| `type` | 数据字段 | 说明 |
| --- | --- | --- |
| `text` | `text` | 视频生成提示词；不能为空。多项文本会按顺序用换行符合并 |
| `image_url` | `image_url.url` | 参考图片 URL |
| `video_url` | `video_url.url` | 参考视频 URL |
| `audio_url` | `audio_url.url` | 参考音频 URL |
| `draft_task` | `draft_task.id` | 样片任务 ID；仅在对应模型支持时生效 |

多媒体 URL 必须能够通过公网直接访问。

`role` 字段可以按官方协议传入。具体角色是否生效以所使用模型的能力为准。

音频参考不能单独使用。当请求包含 `audio_url` 时，还必须至少提供一项 `image_url` 或 `video_url`。

### 2.3 文生视频示例

```bash
curl -X POST "https://api.example.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_MODEL_NAME",
    "content": [
      {
        "type": "text",
        "text": "一只橘猫在雨后的街道上缓慢行走，镜头低机位跟拍，路面倒映霓虹灯光"
      }
    ],
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 10
  }'
```

### 2.4 图片参考示例

```bash
curl -X POST "https://api.example.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_MODEL_NAME",
    "content": [
      {
        "type": "text",
        "text": "保持参考图中的主体外观，让人物自然转身看向镜头，微风吹动衣角"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://cdn.example.com/reference.png"
        },
        "role": "reference_image"
      }
    ],
    "resolution": "720p",
    "ratio": "9:16",
    "duration": 8
  }'
```

### 2.5 多媒体参考示例

```bash
curl -X POST "https://api.example.com/api/v3/contents/generations/tasks" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "YOUR_MODEL_NAME",
    "content": [
      {
        "type": "text",
        "text": "参考图片中的主体、参考视频中的动作节奏和参考音频的节拍，生成一段竖屏短片"
      },
      {
        "type": "image_url",
        "image_url": {
          "url": "https://cdn.example.com/subject.png"
        }
      },
      {
        "type": "video_url",
        "video_url": {
          "url": "https://cdn.example.com/motion.mp4"
        }
      },
      {
        "type": "audio_url",
        "audio_url": {
          "url": "https://cdn.example.com/rhythm.mp3"
        }
      }
    ],
    "resolution": "720p",
    "ratio": "9:16",
    "duration": 10
  }'
```

### 2.6 创建成功响应

HTTP 状态码：`200 OK`

```json
{
  "id": "task_01JXYZEXAMPLE"
}
```

`id` 是创建成功后返回的任务 ID。查询任务时必须使用该 ID。

## 3. 查询视频任务

```http
GET /api/v3/contents/generations/tasks/{task_id}
```

### 3.1 请求示例

```bash
curl -X GET "https://api.example.com/api/v3/contents/generations/tasks/task_01JXYZEXAMPLE" \
  -H "Authorization: Bearer YOUR_API_KEY"
```

任务只能使用创建该任务时所属账户的 API Key 查询。

### 3.2 任务状态

| 状态 | 说明 | 下游处理建议 |
| --- | --- | --- |
| `queued` | 任务已进入队列 | 继续轮询 |
| `running` | 视频正在生成 | 继续轮询 |
| `succeeded` | 生成成功 | 读取 `content.video_url` |
| `failed` | 生成失败 | 读取 `error.code` 和 `error.message` |

建议每隔 5-10 秒查询一次，避免高频轮询。只有 `succeeded` 和 `failed` 是终态。

### 3.3 排队中响应

```json
{
  "id": "task_01JXYZEXAMPLE",
  "model": "YOUR_MODEL_NAME",
  "status": "queued",
  "error": null,
  "created_at": 1784300000,
  "updated_at": 1784300000,
  "content": {},
  "ratio": "9:16",
  "duration": 10
}
```

### 3.4 生成中响应

```json
{
  "id": "task_01JXYZEXAMPLE",
  "model": "YOUR_MODEL_NAME",
  "status": "running",
  "error": null,
  "created_at": 1784300000,
  "updated_at": 1784300030,
  "content": {},
  "ratio": "9:16",
  "duration": 10
}
```

### 3.5 生成成功响应

```json
{
  "id": "task_01JXYZEXAMPLE",
  "model": "YOUR_MODEL_NAME",
  "status": "succeeded",
  "error": null,
  "created_at": 1784300000,
  "updated_at": 1784300180,
  "content": {
    "video_url": "https://cdn.example.com/output/video.mp4"
  },
  "ratio": "9:16",
  "duration": 10
}
```

视频 URL 可能存在有效期限制。任务成功后应及时下载或转存。

### 3.6 生成失败响应

```json
{
  "id": "task_01JXYZEXAMPLE",
  "model": "YOUR_MODEL_NAME",
  "status": "failed",
  "error": {
    "code": "generation_failed",
    "message": "generation failed"
  },
  "created_at": 1784300000,
  "updated_at": 1784300060,
  "content": {},
  "ratio": "9:16",
  "duration": 10
}
```

### 3.7 查询响应字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | string | 视频生成任务 ID |
| `model` | string | 创建任务时使用的模型名称 |
| `status` | string | `queued`、`running`、`succeeded` 或 `failed` |
| `error` | object/null | 失败信息；非失败状态为 `null` |
| `error.code` | string | 任务错误码；部分错误可能不提供该字段 |
| `error.message` | string | 失败原因 |
| `created_at` | integer | 任务创建时间，Unix 时间戳，单位秒 |
| `updated_at` | integer | 任务最后更新时间，Unix 时间戳，单位秒 |
| `content.video_url` | string | 生成成功后返回的视频 URL |
| `resolution` | string | 实际分辨率；无可用数据时省略 |
| `ratio` | string | 实际宽高比；无可用数据时省略 |
| `duration` | integer | 实际视频时长，单位秒；无可用数据时省略 |

## 4. 官方字段兼容说明

下游可以按 Seedance 官方创建任务协议传入以下字段：

- `model`
- `content`
- `callback_url`
- `return_last_frame`
- `service_tier`
- `execution_expires_after`
- `generate_audio`
- `draft`
- `tools`
- `safety_identifier`
- `priority`
- `resolution`
- `ratio`
- `duration`
- `frames`
- `seed`
- `camera_fixed`
- `watermark`

`content` 支持官方定义的 `text`、`image_url`、`video_url`、`audio_url` 和 `draft_task` 类型，以及对应的 `role` 字段。

接口会接收上述官方字段，不会仅因为当前模型不支持某个扩展字段而返回参数不支持错误。

字段能够正常传入不代表对应能力一定生效。实际生成能力以所使用模型为准。例如模型不支持尾帧输出时，即使传入 `return_last_frame: true`，查询结果也不会包含 `last_frame_url`。

## 5. 错误响应

请求级错误使用以下结构：

```json
{
  "code": "invalid_request",
  "message": "image_url.url is required",
  "data": null
}
```

常见错误：

| HTTP 状态码 | `code` 示例 | 说明 |
| --- | --- | --- |
| `400` | `invalid_request` | JSON 格式错误、缺少文本、媒体 URL 为空，或音频缺少图片/视频参考 |
| `400` | `missing_model` | 未提供 `model` |
| `400` | `unsupported_content_type` | `content[].type` 不受支持 |
| `400` | `task_not_exist` | 任务不存在，或当前用户无权查询该任务 |
| `401` | - | API Key 缺失或无效 |
| `402` | - | 账户余额或额度不足 |
| `429` | - | 请求频率或并发超过限制 |
| `500` | - | 服务内部错误 |
| `502` | `invalid_response` | 视频生成服务返回了无法识别的响应 |

调用方应同时判断 HTTP 状态码和响应体，不要只根据 `code` 字段判断请求是否成功。

## 6. 推荐调用流程

1. 获取 Base URL、API Key 和已分配的模型名称。
2. 调用创建接口并保存响应中的 `id`。
3. 每隔 5-10 秒调用查询接口。
4. 收到 `queued` 或 `running` 时继续轮询。
5. 收到 `succeeded` 时读取并及时保存 `content.video_url`。
6. 收到 `failed` 时停止轮询并记录 `error`。
7. 对网络错误、`429` 和可恢复的 `5xx` 使用指数退避重试；不要用同一个请求无限创建新任务。
