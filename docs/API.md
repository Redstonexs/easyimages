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
