# NewAPI TOS 上传客户端

该客户端需要 Node.js 18 或更高版本，不依赖第三方 npm 包。它先向 NewAPI 请求临时上传 URL，再把文件流式上传到 TOS。NewAPI Key 只会发送给 NewAPI，不会发送给 TOS。

## 使用环境变量

PowerShell：

```powershell
$env:NEW_API_BASE_URL = "http://localhost:3000"
$env:NEW_API_KEY = "sk-your-new-api-key"
node scripts/tos-upload-client.mjs --file "C:\path\to\photo.png"
```

Bash：

```bash
export NEW_API_BASE_URL="https://new-api.example.com"
export NEW_API_KEY="sk-your-new-api-key"
node scripts/tos-upload-client.mjs --file ./photo.png
```

也可以使用 Node.js 的 env 文件支持：

```env
NEW_API_BASE_URL=https://new-api.example.com
NEW_API_KEY=sk-your-new-api-key
```

```bash
node --env-file=.env.client scripts/tos-upload-client.mjs --file ./photo.png
```

上传成功后输出：

```json
{
  "objectKey": "uploads/123/2026/07/21/uuid.png",
  "publicUrl": "https://my-bucket.tos-cn-beijing.volces.com/uploads/123/2026/07/21/uuid.png",
  "size": 102400,
  "contentType": "image/png",
  "expiresAt": 1784590000
}
```

`publicUrl` 是对象的固定公网地址。Bucket 或对象必须配置为公共读，否则访问该地址时 TOS 会返回 403。

可通过 `--content-type` 覆盖自动识别的媒体类型，通过 `--timeout-ms` 调整大文件上传超时：

```bash
node scripts/tos-upload-client.mjs ./video.mp4 --content-type video/mp4 --timeout-ms 1800000
```

运行本地模拟测试：

```bash
node --test scripts/tos-upload-client.test.mjs
```
