# API

Go 版本上传接口为 `POST /api/index`。旧 PHP 风格的 `/api/index.php` 当前没有兼容路由。

## 前置条件

- 在管理后台开启 API 上传。
- 使用 `config/api_key.json` 中的有效 token，或从旧 PHP `config/api_key.php` 自动迁移生成。

## 上传参数

| 参数 | 位置 | 必填 | 说明 |
| --- | --- | --- | --- |
| `image` | multipart file | 是 | 要上传的文件 |
| `token` | multipart form | 是 | API token |

## HTML 示例

```html
<form action="http://127.0.0.1:8080/api/index" method="post" enctype="multipart/form-data">
  <input type="file" name="image" accept="image/*" required>
  <input type="text" name="token" placeholder="输入 API token" required>
  <input type="submit" value="上传">
</form>
```

## curl 示例

```bash
curl -X POST http://127.0.0.1:8080/api/index \
  -F "image=@/path/to/your/file/example.jpg" \
  -F "token=your_token"
```

## Python 示例

```python
import requests

image_path = "/path/to/your/image.jpg"
token = "your_token"
url = "http://127.0.0.1:8080/api/index"

with open(image_path, "rb") as image:
    response = requests.post(
        url,
        files={"image": image},
        data={"token": token},
        timeout=30,
    )

print(response.status_code)
print(response.json())
```

## jQuery 示例

```javascript
var file = document.querySelector('input[type="file"]').files[0];
var token = $('input[name="token"]').val();

var formData = new FormData();
formData.append('image', file);
formData.append('token', token);

$.ajax({
  url: 'http://127.0.0.1:8080/api/index',
  type: 'POST',
  data: formData,
  processData: false,
  contentType: false,
  success: function(response) {
    console.log('上传成功', response);
  },
  error: function(xhr, status, error) {
    console.error('上传失败: ' + error);
  }
});
```

## 成功响应

```json
{
  "result": "success",
  "code": 200,
  "url": "http://127.0.0.1:8080/i/2026/06/02/example.jpg",
  "srcName": "example",
  "thumb": "http://127.0.0.1:8080/app/thumb?img=/i/2026/06/02/example.jpg",
  "del": "http://127.0.0.1:8080/app/del_hash?hash=...",
  "id": 0
}
```

## 响应字段

| 字段 | 说明 |
| --- | --- |
| `result` | `success` 或 `failed` |
| `code` | 状态码，见 [常见状态代码](./常见状态代码.md) |
| `url` | 原图访问地址 |
| `srcName` | 原始文件名去扩展名后的名称 |
| `thumb` | 缩略图地址 |
| `del` | 用户删除链接，取决于配置 |
| `webp_url` | WebP 转换启用且生成成功时返回 |
| `id` | API token 对应的 ID |

## 图片库 API v1

管理员在「图片库 → 租户与链接」创建账号、设置存储配额和公开链接格式。租户使用同一登录页，登录后进入自己的图片库。管理员可通过「查看账号」管理任意租户的目录和图片。

在图片库的「API 访问」生成令牌后，通过 `Authorization: Bearer <token>` 调用接口。令牌按账号授权，重新生成会替换之前的令牌。自动化工具可读取 `/api/v1/openapi.json` 或 `/api/v1/openapi.yaml`，再调用 `getCurrentUser` 验证身份。错误采用 RFC 9457 的 `application/problem+json`，可用 `code` 判断具体原因。配额不足会返回 HTTP 409 和 `quota_exceeded`，同时带上配额和用量。请求大小已知时也会返回 `requested_bytes`。

### 上传原图

```bash
BASE_URL=http://127.0.0.1:8080
# EASYIMAGE_TOKEN 由调用环境注入。

curl -fsS "$BASE_URL/api/v1/me" \
  -H "Authorization: Bearer $EASYIMAGE_TOKEN"

curl -fsS "$BASE_URL/api/v1/folders" \
  -H "Authorization: Bearer $EASYIMAGE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"path":"projects/summer"}'

curl -fsS "$BASE_URL/api/v1/images" \
  -H "Authorization: Bearer $EASYIMAGE_TOKEN" \
  -F 'file=@photo.png' \
  -F 'folder=projects/summer'
```

创建嵌套目录会同时创建缺失的父目录。上传时选择已有目录，省略 `folder` 上传到根目录。管理员可在上传、创建目录和查询列表时传入 `owner_id`，目标账号承担相应存储用量。

上传响应中的 `id` 用于查询、预生成和删除，`url` 是公开链接。链接形如 `https://images.example.com/media/{image_id}`，目录和文件名只在管理界面和认证 API 中显示。更改链接前缀后，服务继续响应原前缀。切换域名时，已发布的旧域名需要继续路由到该服务。

### 动态转换和预生成

在上传返回的 `url` 后追加 `?width=640&format=webp&quality=85`，即可在请求时转换。直接访问 `url` 会返回原始上传字节。

| 参数 | 说明 |
| --- | --- |
| `width`、`height` | 目标像素，范围 1–4096。仅填写一项时另一边按比例计算 |
| `scale` | 按原始尺寸等比缩放，范围 0.01–4。选择倍数时可同时设置输出格式和画质 |
| `fit` | `scale-down` 默认等比缩小，`contain` 允许等比放大，`cover` 需要宽高并居中裁剪填满 |
| `format` | `jpeg`、`png` 或 `webp`。省略时优先保留 JPEG 或 WebP 编码，其余可转换原图输出 PNG |
| `quality` | JPEG 和 WebP 的画质，范围 1–100，默认 85 |

要保存一个版本，使用上传返回的 `id`，在 `transform` 中传入相同参数。省略目标目录时，版本保存到原图目录。

```bash
curl -fsS "$BASE_URL/api/v1/folders" \
  -H "Authorization: Bearer $EASYIMAGE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"path":"exports/web"}'

curl -fsS "$BASE_URL/api/v1/images/$IMAGE_ID/variants" \
  -H "Authorization: Bearer $EASYIMAGE_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"folder":"exports/web","transform":{"scale":0.5,"format":"jpeg","quality":85}}'
```

- 每个预生成版本都有独立的 `id` 和 `url`。查询目标目录时传入 `include_variants=true` 即可列出这些版本。
- 配额统计原图和保存的版本。动态转换不计入配额。删除原图会同时删除其保存版本并释放用量。
- 转换接受 JPEG、PNG、静态 WebP、GIF 和 BMP，GIF 使用第一帧。转换源上限为 32 MiB 和 2,000 万像素，结果每边最多 4096 像素。
- WebP 输出使用 `cwebp`。Docker 镜像已经包含它。

### CDN 缓存

图片库的公开链接以图片 ID 结尾。Cloudflare 的默认静态规则按扩展名判断，需要把公开图片前缀设为可缓存。用默认前缀时，规则可匹配 `starts_with(http.request.uri.path, "/media/")`。启用仍在使用的历史前缀可以让已发布链接同样受益。[Cloudflare 默认缓存规则](https://developers.cloudflare.com/cache/concepts/default-cache-behavior/)

让 CDN 遵循源站 `Cache-Control`，并把查询字符串保留在缓存键中。不同的 `width`、`scale`、`format` 等参数可能返回不同图片。默认缓存期限为 300 秒。启用防盗链后，源站返回 `private, no-store`，以便每次请求都检查 Referer。[Cloudflare 缓存键](https://developers.cloudflare.com/cache/how-to/cache-keys/)

### 升级和存储

图片库使用 `admin/logs/workspace/` 下的本地文件存储，每个数据目录由一个服务实例写入。备份时保留该目录里的目录索引和图片文件。旧的图片访问路径、S3 存储和 `/api/index` 接口继续使用原配置。若站点启用了匿名上传，历史匿名入口仍按原配置工作。

若旧图片目录占用了 `/api/v1`，图片库 API 使用 `/api/workspace/v1`。图片库里的 OpenAPI 链接会指向实际接口地址。

旧图片目录与 `admin/logs/workspace/` 重叠时，图片库使用 `config/workspace/` 保存数据。

尺寸适配参数参考 [Cloudflare Images 转换文档](https://developers.cloudflare.com/images/optimization/features/)。接口描述和错误格式参考 [OpenAPI 3.1.1](https://spec.openapis.org/oas/v3.1.1.html) 与 [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)。
