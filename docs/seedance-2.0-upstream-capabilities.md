# Seedance 2.0 上游渠道字段能力矩阵

本文档记录 NewAPI 对外提供的 Seedance 官方协议字段，与各 Seedance 2.0 上游渠道实际能力之间的差异。

下游接口协议保持统一：

```text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{task_id}
```

## 1. 兼容原则

1. 下游始终使用 Seedance 官方字段，不需要了解上游供应商协议。
2. 下游模型名称与上游模型名称通过渠道模型映射隔离。
3. 上游支持的字段由对应渠道适配器转换并转发。
4. 上游不支持的官方字段仍允许下游传入，但适配器不会将其发送给上游。
5. 被忽略的字段不会导致 NewAPI 返回 `unsupported_parameter`。
6. 字段被接收只代表协议兼容，不代表目标上游能够实现该能力。
7. 视频结果直接使用上游返回的 URL，NewAPI 不下载、不转码、不转存视频文件。

能力状态定义：

| 状态 | 含义 |
| --- | --- |
| 支持 | 上游原生支持，适配器会转发 |
| 转换支持 | 上游支持该能力，适配器会转换字段名称或结构后转发 |
| 接收但忽略 | 下游可以传入，但不会发送给该上游，相关能力不会生效 |
| 条件支持 | 仅在满足额外输入条件时转发 |
| 响应支持 | 能从上游响应或 NewAPI 任务记录中返回 |
| 不返回 | 查询响应中不会提供该字段 |

## 2. MoyinAI 创建任务字段

MoyinAI 上游接口：

```text
POST {channel_base_url}/v1/videos
```

| 官方字段 | MoyinAI 字段 | 状态 | 处理说明 |
| --- | --- | --- | --- |
| `model` | `model` | 转换支持 | 下游模型名先完成渠道筛选，再替换为渠道模型映射中的上游模型名 |
| `content[].type=text` | `prompt` | 转换支持 | 多项有效文本使用换行符合并为一个提示词 |
| `content[].type=image_url` | `images[]` | 转换支持 | `first_frame`、`reference_image` 和未填写角色的图片作为通用图片参考发送 |
| `content[].role=last_frame` | - | 接收但忽略 | 尾帧图片不会加入 MoyinAI 的 `images` 数组 |
| `content[].type=video_url` | `videos[]` | 转换支持 | 参考视频 URL 数组 |
| `content[].type=audio_url` | `audios[]` | 条件支持 | 必须同时存在至少一项图片或视频参考 |
| `content[].type=draft_task` | - | 接收但忽略 | MoyinAI 不支持基于官方 Draft 任务 ID 生成正式视频 |
| `resolution` | `resolution` | 支持 | 原值转发；有效取值由 MoyinAI 模型配置决定 |
| `ratio` | `aspect_ratio` | 转换支持 | 字段改名后转发 |
| `duration` | `seconds` | 转换支持 | 整数秒转换为字符串后转发 |
| `callback_url` | - | 接收但忽略 | NewAPI 当前也不会为该字段主动发送任务状态回调 |
| `return_last_frame` | - | 接收但忽略 | 查询结果不会返回 `content.last_frame_url` |
| `service_tier` | - | 接收但忽略 | 不控制 MoyinAI 的服务等级 |
| `execution_expires_after` | - | 接收但忽略 | 不改变 MoyinAI 任务超时策略 |
| `generate_audio` | - | 接收但忽略 | 不控制 MoyinAI 是否生成同步音频 |
| `draft` | - | 接收但忽略 | 不启用样片模式 |
| `tools` | - | 接收但忽略 | 不启用联网搜索等官方工具 |
| `safety_identifier` | - | 接收但忽略 | 不发送给 MoyinAI |
| `priority` | - | 接收但忽略 | 不改变 MoyinAI 队列优先级 |
| `frames` | - | 接收但忽略 | 不按帧数控制时长；需要时应同时提供 `duration` |
| `seed` | - | 接收但忽略 | 不控制随机种子 |
| `camera_fixed` | - | 接收但忽略 | 不控制固定镜头能力 |
| `watermark` | - | 接收但忽略 | 不控制 MoyinAI 输出水印 |

### 2.1 MoyinAI 实际请求示例

下游请求：

```json
{
  "model": "S-MOYIN",
  "content": [
    {
      "type": "text",
      "text": "人物从街角走向镜头"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://cdn.example.com/first.png"
      },
      "role": "first_frame"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://cdn.example.com/last.png"
      },
      "role": "last_frame"
    }
  ],
  "resolution": "720p",
  "ratio": "16:9",
  "duration": 10,
  "return_last_frame": true,
  "generate_audio": false,
  "priority": 5,
  "watermark": true
}
```

假设 `S-MOYIN` 映射到 MoyinAI 上游模型 `provider-seedance-2`，实际发送给 MoyinAI 的请求为：

```json
{
  "model": "provider-seedance-2",
  "prompt": "人物从街角走向镜头",
  "images": [
    "https://cdn.example.com/first.png"
  ],
  "seconds": "10",
  "aspect_ratio": "16:9",
  "resolution": "720p"
}
```

`last_frame`、`return_last_frame`、`generate_audio`、`priority` 和 `watermark` 均被接收，但不会出现在 MoyinAI 请求中。

## 3. MoyinAI 查询任务字段

MoyinAI 上游接口：

```text
GET {channel_base_url}/v1/videos/{upstream_task_id}
```

| 官方查询字段 | 状态 | 数据来源或处理说明 |
| --- | --- | --- |
| `id` | 响应支持 | 返回 NewAPI 公开任务 ID，不暴露上游任务 ID |
| `model` | 响应支持 | 返回下游请求使用的模型名称，例如 `S-MOYIN` |
| `status` | 转换支持 | 上游状态统一映射为 `queued`、`running`、`succeeded`、`failed` |
| `error.code` | 条件支持 | 上游提供错误码时返回 |
| `error.message` | 响应支持 | 任务失败时返回失败原因 |
| `created_at` | 响应支持 | NewAPI 任务创建时间 |
| `updated_at` | 响应支持 | NewAPI 任务最后更新时间 |
| `content.video_url` | 响应支持 | 读取上游 `video_url` 或 `result.video_url`，并保存到任务记录 |
| `content.last_frame_url` | 不返回 | MoyinAI 不提供尾帧 URL |
| `seed` | 不返回 | MoyinAI 查询响应未定义该字段 |
| `resolution` | 不返回 | 当前适配器不从 MoyinAI 查询响应生成该字段 |
| `ratio` | 条件支持 | 上游返回 `size` 时映射为 `ratio` |
| `duration` | 条件支持 | 上游返回可解析的 `seconds` 时映射为整数秒 |
| `frames` | 不返回 | MoyinAI 不提供帧数 |
| `framespersecond` | 不返回 | MoyinAI 不提供帧率 |
| `generate_audio` | 不返回 | MoyinAI 不提供该结果信息 |
| `tools` | 不返回 | MoyinAI 不提供官方工具使用信息 |
| `safety_identifier` | 不返回 | 该字段未发送给 MoyinAI |
| `priority` | 不返回 | 该字段未发送给 MoyinAI |
| `draft` | 不返回 | MoyinAI 不支持官方样片模式 |
| `draft_task_id` | 不返回 | MoyinAI 不支持官方样片任务 |
| `service_tier` | 不返回 | MoyinAI 不提供官方服务等级字段 |
| `execution_expires_after` | 不返回 | MoyinAI 不提供该字段 |
| `usage` | 不返回 | MoyinAI 当前文档未定义官方 token 用量结构 |

## 4. viraldance933m 创建任务字段

viraldance933m 上游接口：

```text
POST https://api.viralee.top/v1/videos
```

| 官方字段 | viraldance933m 字段 | 状态 | 处理说明 |
| --- | --- | --- | --- |
| `model` | `model` | 转换支持 | 下游模型名由公共渠道路由处理，适配器不校验名称；上游固定发送 `viraldance933m`，不使用渠道模型映射覆盖 |
| `content[].type=text` | `prompt` | 转换支持 | 官方文本内容由公共协议层合并后发送，最长 2000 字符 |
| `content[].role=first_frame` | `start_image_url` | 转换支持 | 作为首帧图片 URL 发送 |
| `content[].role=last_frame` | `end_image_url` | 转换支持 | 作为尾帧图片 URL 发送 |
| `content[].role=reference_image` | `image_urls[]` | 转换支持 | 未填写角色的图片也按参考图处理，最多 9 张 |
| `content[].type=video_url` | `video_reference[]` | 转换支持 | 转换为 `{\"url\": \"...\"}` 数组，最多 3 条 |
| `content[].type=audio_url` | `audio_reference[]` | 条件支持 | 转换为 `{\"url\": \"...\"}` 数组，最多 3 条，且官方协议要求同时提供图片或视频参考 |
| `content[].type=draft_task` | - | 接收但忽略 | 上游未定义官方 Draft 任务引用 |
| `resolution` | `resolution` | 条件支持 | `720p` 会发送；其他官方取值被接收但不发送给该上游 |
| `ratio` | `aspect_ratio` | 条件支持 | `16:9`、`9:16`、`1:1` 会发送；其他官方取值被接收但不发送 |
| `duration` | `duration` | 转换支持 | 以数值发送，允许 4-15 秒 |
| `callback_url` | - | 接收但忽略 | NewAPI 不向该上游注册回调 |
| `return_last_frame` | - | 接收但忽略 | 不要求上游返回尾帧 URL |
| `service_tier` | - | 接收但忽略 | 不控制上游服务等级 |
| `execution_expires_after` | - | 接收但忽略 | 不改变上游任务超时策略 |
| `generate_audio` | - | 接收但忽略 | 不控制上游是否生成同步音频 |
| `draft` | - | 接收但忽略 | 不启用样片模式 |
| `tools` | - | 接收但忽略 | 不发送官方工具配置 |
| `safety_identifier` | - | 接收但忽略 | 不发送给上游 |
| `priority` | - | 接收但忽略 | 不改变上游队列优先级 |
| `frames` | - | 接收但忽略 | 不按帧数控制时长 |
| `seed` | - | 接收但忽略 | 不控制随机种子 |
| `camera_fixed` | - | 接收但忽略 | 不控制固定镜头能力 |
| `watermark` | - | 接收但忽略 | 不控制输出水印 |

## 5. viraldance933m 查询任务字段

viraldance933m 上游接口：

```text
GET https://api.viralee.top/v1/videos/{upstream_task_id}
```

| 官方查询字段 | 状态 | 数据来源或处理说明 |
| --- | --- | --- |
| `id` | 响应支持 | 返回 NewAPI 公开任务 ID，不暴露上游任务 ID |
| `model` | 响应支持 | 固定返回下游模型名 `viraldance933m` |
| `status` | 转换支持 | `submitted`/`queued` 映射为 `queued`，`processing`/`in_progress` 映射为 `running` |
| `error.code` | 条件支持 | 上游提供错误码时返回 |
| `error.message` | 响应支持 | 固定返回通用失败信息，不透传上游错误文案 |
| `created_at` | 响应支持 | NewAPI 任务创建时间 |
| `updated_at` | 响应支持 | NewAPI 任务最后更新时间 |
| `content.video_url` | 响应支持 | 读取上游 `metadata.url`，保存到任务记录后直接返回，不下载视频 |
| 其他结果扩展字段 | 不返回 | 上游查询协议未提供可可靠映射的数据 |

上游仅返回 `completed` 但没有 `metadata.url` 时，任务继续轮询。非终态即使上游报告进度 100%，NewAPI 也会保存为 99%，避免公共轮询器提前停止。

## 6. viraldance933x 创建任务字段

viraldance933x 上游接口：

```text
POST https://api.viralee.top/v1/videos
```

| 官方字段 | viraldance933x 字段 | 状态 | 处理说明 |
| --- | --- | --- | --- |
| `model` | `model` | 转换支持 | 下游模型名由公共渠道路由处理，适配器不校验名称；优先发送渠道配置的 `UpstreamModelName`，未配置时回退到下游模型名 |
| `content[].type=text` | `prompt` | 转换支持 | 官方文本内容由公共协议层合并后发送，最长 5000 字符 |
| `content[].role=first_frame` | `start_image_url` | 转换支持 | 作为首帧图片 URL 发送 |
| `content[].role=last_frame` | `end_image_url` | 转换支持 | 作为尾帧图片 URL 发送 |
| `content[].role=reference_image` | `image_urls[]` | 转换支持 | 未填写角色的图片也按参考图处理，最多 9 张 |
| `content[].type=video_url` | `video_reference[]` | 转换支持 | 转换为 `{\"url\": \"...\"}` 数组，最多 3 条 |
| `content[].type=audio_url` | `audio_reference[]` | 条件支持 | 转换为 `{\"url\": \"...\"}` 数组，最多 3 条，且官方协议要求同时提供图片或视频参考 |
| `content[].type=draft_task` | - | 接收但忽略 | 上游未定义官方 Draft 任务引用 |
| `resolution` | `resolution` | 条件支持 | `720p` 会发送；其他官方取值被接收但不发送给该上游 |
| `ratio` | `aspect_ratio` | 条件支持 | 支持 `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`9:21` |
| `duration` | `duration` | 转换支持 | 以数值发送，允许 4-14 秒；未传时按默认 5 秒计费 |
| 其他官方扩展字段 | - | 接收但忽略 | 不向该上游转发 |

该渠道按秒计费。模型价格应配置为每秒单价，任务预扣额度使用实际 `duration`；未传 `duration` 时使用上游默认值 5 秒。

## 7. viraldance933x 查询任务字段

viraldance933x 上游接口：

```text
GET https://api.viralee.top/v1/videos/{upstream_task_id}
```

| 官方查询字段 | 状态 | 数据来源或处理说明 |
| --- | --- | --- |
| `id` | 响应支持 | 返回 NewAPI 公开任务 ID，不暴露上游任务 ID |
| `model` | 响应支持 | 返回下游请求使用的模型名称 |
| `status` | 转换支持 | `submitted`/`queued` 映射为 `queued`，`in_progress` 映射为 `running` |
| `error.code` | 条件支持 | 上游提供错误码时返回 |
| `error.message` | 响应支持 | 固定返回通用失败信息，不透传上游错误文案 |
| `created_at` | 响应支持 | NewAPI 任务创建时间 |
| `updated_at` | 响应支持 | NewAPI 任务最后更新时间 |
| `content.video_url` | 响应支持 | 读取上游顶层 `video_url`，保存到任务记录后直接返回，不下载视频 |
| 其他结果扩展字段 | 不返回 | 上游查询协议未提供可可靠映射的数据 |

上游仅返回 `completed` 但没有 `video_url` 时，任务继续轮询。非终态即使上游报告进度 100%，NewAPI 也会保存为 99%。

## 8. 渠道新增维护规范

每接入一个新的 Seedance 2.0 上游渠道，应在本文档新增两张表：创建任务字段和查询任务字段。

渠道适配器需要遵守以下规则：

1. 不在公共中间件中硬编码供应商模型名。
2. 使用 `OriginModelName` 保存下游模型名，使用 `UpstreamModelName` 调用上游。
3. 只向上游发送该渠道文档明确支持的字段。
4. 不因某个渠道不支持官方扩展字段而拒绝下游请求。
5. 对语义不等价的字段不得强行映射。例如尾帧不能当作普通参考图发送。
6. 查询成功必须同时满足上游成功状态和非空视频 URL。
7. 上游视频 URL 保存到任务记录并直接返回，不在服务器下载视频。
8. 新增渠道时为字段过滤、模型映射、状态映射和 URL 提取添加确定性测试。

## 9. 当前渠道总览

| 渠道 ID | 渠道 | 下游模型名示例 | 上游模型名来源 | 文本 | 图片 | 视频 | 音频 | 尾帧 | 官方扩展字段 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `9801` | MoyinAI | `S-MOYIN` | 渠道模型映射 | 支持 | 通用参考 | 支持 | 条件支持 | 不支持 | 接收但不转发 |
| `9802` | viraldance933m | `viraldance933m` | 适配器固定 | 支持 | 参考图/首帧 | 支持 | 条件支持 | 支持尾帧输入 | 接收但按能力转发 |
| `9803` | viraldance933x | 渠道模型映射 | `UpstreamModelName` | 支持 | 参考图/首帧 | 支持 | 条件支持 | 支持尾帧输入 | 接收但按能力转发 |
| `9804` | viraldance933v | 渠道模型映射 | `UpstreamModelName` | 支持 | 参考图/首帧 | 支持 | 条件支持 | 支持尾帧输入 | 接收但按能力转发 |
| `9805` | viraldance431-fast | 渠道模型映射 | `UpstreamModelName` | 支持 | 参考图/首帧 | 支持 | 条件支持 | 支持尾帧输入 | 接收但按能力转发 |
| `9806` | gravitex-sd | 渠道模型映射 | `UpstreamModelName` | 支持 | 参考图/首帧 | 支持 | 条件支持 | 支持尾帧输入 | 接收但按能力转发 |

## 10. viraldance933v 字段说明

viraldance933v 上游接口：

```text
POST https://api.viralee.top/v1/videos
GET  https://api.viralee.top/v1/videos/{upstream_task_id}
```

| 官方字段 | viraldance933v 字段 | 状态 | 处理说明 |
| --- | --- | --- | --- |
| `model` | `model` | 转换支持 | 适配器不校验或固定模型名，优先使用渠道 `UpstreamModelName`，否则回退到 `OriginModelName` |
| `content[].type=text` | `prompt` | 转换支持 | 文本合并后发送，最长 5000 字符 |
| `content[].role=first_frame` | `start_image_url` | 转换支持 | 首帧图片 |
| `content[].role=last_frame` | `end_image_url` | 转换支持 | 尾帧图片 |
| `content[].type=image_url` | `image_urls[]` | 转换支持 | 参考图片最多 9 张 |
| `content[].type=video_url` | `video_reference[]` | 转换支持 | 参考视频最多 3 条 |
| `content[].type=audio_url` | `audio_reference[]` | 条件支持 | 参考音频最多 3 条 |
| `duration` | `duration` | 转换支持 | 上游允许 1-15 秒；未传时使用上游默认 5 秒 |
| `ratio` / `resolution` | - | 接收但忽略 | 该渠道固定输出 9:16、720p |
| `enable_face_mask` | - | 接收但忽略 | 该字段不是下游官方 Seedance 协议字段，不从下游转发 |
| 其他官方扩展字段 | - | 接收但忽略 | 不发送给上游 |

查询成功时优先读取上游顶层 `url`，没有时读取 `metadata.url`。只有成功状态且视频 URL 非空才结束轮询；非终态进度为 100% 时保存为 99%。视频 URL 直接保存到任务记录并返回，不下载视频文件。

## 11. viraldance431-fast 字段说明

viraldance431-fast 上游接口：

```text
POST https://api.viralee.top/v1/videos
GET  https://api.viralee.top/v1/videos/{upstream_task_id}
```

该渠道按次计费，生成失败由现有任务结算流程处理退款。

| 官方字段 | viraldance431-fast 字段 | 状态 | 处理说明 |
| --- | --- | --- | --- |
| `model` | `model` | 转换支持 | 适配器不校验或固定模型名，优先使用渠道 `UpstreamModelName`，否则回退到 `OriginModelName` |
| `content[].type=text` | `prompt` | 转换支持 | 文本合并后发送，最长 5000 字符 |
| `content[].role=first_frame` | `start_image_url` | 转换支持 | 首帧图片 |
| `content[].role=last_frame` | `end_image_url` | 转换支持 | 尾帧图片 |
| `content[].type=image_url` | `image_urls[]` | 转换支持 | 普通参考图片最多 4 张 |
| `content[].type=video_url` | `video_reference[]` | 转换支持 | 参考视频最多 3 条 |
| `content[].type=audio_url` | `audio_reference[]` | 条件支持 | 参考音频最多 1 条；官方入口仍要求音频搭配图片或视频素材 |
| `duration` | `duration` | 转换支持 | 上游允许 4-15 秒；未传时由上游使用默认 5 秒 |
| `ratio` | `aspect_ratio` | 转换支持 | 支持 `16:9`、`9:16`、`1:1` |
| `resolution` | `resolution` | 转换支持 | 仅向上游发送 `720p` |
| 其他官方扩展字段 | - | 接收但忽略 | 不发送给上游 |

查询完成时优先读取上游顶层 `url`，并兼容读取 `video_url` 和 `metadata.url`。只有成功状态且视频 URL 非空才停止轮询；非终态进度为 100% 时保存为 99%。视频 URL 直接保存到任务记录并返回，不下载视频文件。

## 12. gravitex-sd 字段说明

gravitex-sd 上游接口：

```text
POST https://api.gravitex.ai/v1/video/generations
GET  https://api.gravitex.ai/v1/video/generations/{upstream_task_id}
```

该渠道按秒计费。模型价格应配置为每秒单价，任务费用按 `duration` 的秒数计算；未传 `duration` 或传 `-1` 自动时长时，因上游响应不返回实际成片时长，按其文档默认 5 秒计费。

| 官方字段 | gravitex-sd 字段 | 状态 | 处理说明 |
| --- | --- | --- | --- |
| `model` | `model` | 转换支持 | 不校验或固定模型名；优先使用渠道 `UpstreamModelName`，否则回退到 `OriginModelName`。上游文档列出 `seedance-2-0` 和 `seedance-2-0-fast` |
| `content[].type=text` | `content[].type=text` | 直接支持 | 最多 1 条，最长 5000 字符；没有 `content` 时可使用 `prompt` |
| `content[].role=first_frame` | 同名字段 | 直接支持 | 最多 1 张；未指定角色的单图也由上游作为首帧处理 |
| `content[].role=last_frame` | 同名字段 | 直接支持 | 最多 1 张且必须搭配首帧 |
| `content[].role=reference_image` | 同名字段 | 直接支持 | 最多 9 张，与首帧模式互斥 |
| `content[].type=video_url` | 同名字段 | 直接支持 | 参考视频最多 3 条 |
| `content[].type=audio_url` | 同名字段 | 条件支持 | 参考音频最多 3 条，必须搭配图片或视频 |
| `duration` | `duration` | 直接支持 | 允许 `-1` 自动时长或 4-15 秒 |
| `resolution` | `resolution` | 直接支持 | 支持 `480p`、`720p`、`1080p`；fast 模型的 1080p 限制交由上游按实际模型判断 |
| `ratio` | `ratio` | 直接支持 | 支持 `16:9`、`9:16`、`1:1`、`4:3`、`3:4`、`21:9`、`adaptive` |
| `generate_audio` | `generate_audio` | 直接支持 | 显式 `false` 会保留并发送 |
| `watermark` | `watermark` | 直接支持 | 显式 `false` 会保留并发送 |
| `seed` | `seed` | 直接支持 | 显式 `0` 或 `-1` 会保留并发送 |
| `draft_task` 及其他扩展字段 | - | 接收但忽略 | 不发送给上游 |

查询完成时兼容上游 `{code, message, data}` 包装结构，优先读取 `data.video_url`，并兼容读取 `data.url`、`data.metadata.video_url`、`data.metadata.url` 和 `data.metadata.content.video_url`。只有成功状态且视频 URL 非空才停止轮询；非终态进度为 100% 时保存为 99%。视频 URL 直接保存到任务记录并返回，不下载视频文件。上游错误状态码可以返回，但错误消息统一替换为公共安全文案。

### 12.1 Gravitex 素材库内部托管

下游不需要调用 Gravitex 素材库接口，也不能传 Base64、Data URI、本地路径或 `asset://` 地址。下游在官方 `content` 中传入可被公网访问的 `http://` 或 `https://` 图片、视频、音频 URL 后，适配器会在提交视频任务前：

1. 按渠道 API Key 和上游地址查找或创建一个托管的 `aigc` 素材组；
2. 调用 `POST /v1/assets` 上传 URL，并按内容类型传递 `Image`、`Video` 或 `Audio`；
3. 轮询素材状态直到 `active`，将内容 URL 替换为上游返回的 `asset://...` 地址；
4. 使用内存缓存复用已创建的素材组和已激活素材，视频生成请求本身仍只发送上游需要的字段。

素材上传或处理失败时，任务不会提交到视频生成接口，向下游返回统一的素材准备失败信息；NewAPI 不下载或保存素材和视频文件本体。
