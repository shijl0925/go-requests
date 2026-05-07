# go-requests

<a href="README.md">English</a> | <a href="README-zh.md">中文</a>

go-requests 是一个用户友好的 Go HTTP 客户端库，灵感来自 Python 的 [requests](https://requests.readthedocs.io/) 库。它封装了标准库 `net/http`，提供更简单、更具表现力的 API。

## 安装

```bash
go get github.com/shijl0925/go-requests
```

## 快速开始

```go
package main

import (
    "fmt"
    "github.com/shijl0925/go-requests"
)

func main() {
    resp, err := requests.Get("https://httpbin.org/get",
        requests.Params{"key": "value"},
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(resp.StatusCode) // 200
    fmt.Println(resp.Text())     // response body as string
}
```

## 功能特性

- 简单的包级函数：`Get`、`Post`、`Put`、`Patch`、`Delete`、`Head`、`Options`
- 使用 `Session` 在多个请求之间持久化请求头、Cookie 和设置
- URL 查询参数（`Params`）
- 请求头（`Headers`）
- JSON 请求体（`JSON`、`JSONBody`）
- 表单数据请求体（`Data`）
- 多值查询和表单字段（`ParamValues`、`DataValues`）
- 文件上传（`Files`）
- 流式上传（`Body`、流式 multipart `Files`）
- Cookie 管理（`Cookies`）
- 身份认证：Basic Auth、Bearer Token、Digest Auth
- 请求超时（`Timeout`）
- 对可重放请求自动重试（`Retry`）
- 重定向控制（`AllowRedirects`、`MaxRedirects`）
- TLS 验证控制（`Verify`）
- 代理支持（`Proxies`）
- 传输层和连接池配置（`TransportConfig`）
- 自定义 HTTP 客户端和传输器（`HTTPClient`、`RoundTripper`）
- Context 支持（`WithContext`）
- 常用请求头快捷方式（`Header`、`UserAgent`、`Referer`、`Accept`、`ContentType`）
- 流式响应（`Stream`）
- 响应辅助方法：`.Text()`、`.JSON()`、`.JSONMap()`、`.JSONSlice()`、`.Content()`、`.SaveToFile()`、`.Ok()`、`.RaiseForStatus()`、`.IsRedirect()`

## 使用方法

### GET 请求

```go
resp, err := requests.Get("https://httpbin.org/get",
    requests.Params{"page": "1", "limit": "10"},
    requests.Headers{"Accept": "application/json"},
)
```

当同一个查询 key 需要多个值时，使用 `ParamValues`：

```go
resp, err := requests.Get("https://httpbin.org/get",
    requests.ParamValues{"tag": {"go", "requests"}},
)
```

### 使用 JSON 请求体发送 POST

```go
resp, err := requests.Post("https://httpbin.org/post",
    requests.JSON{"name": "Alice", "age": 30},
)
```

### 使用结构体发送 POST

```go
type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

resp, err := requests.Post("https://httpbin.org/post",
    requests.JSONBody{Value: User{Name: "Bob", Age: 25}},
)
```

### POST 表单数据

```go
resp, err := requests.Post("https://httpbin.org/post",
    requests.Data{"username": "admin", "password": "secret"},
)
```

当表单字段需要重复值时，使用 `DataValues`：

```go
resp, err := requests.Post("https://httpbin.org/post",
    requests.DataValues{"tag": {"go", "requests"}},
)
```

### 文件上传

```go
f, _ := os.Open("report.pdf")
defer f.Close()

resp, err := requests.Post("https://httpbin.org/post",
    requests.Files{
        "file": requests.FileField{
            FileName: "report.pdf",
            Content:  f,
        },
    },
)
```

Multipart 文件上传会从提供的 reader 中流式读取，而不是将整个文件缓存在内存中。

### 流式 POST 上传

```go
f, _ := os.Open("large-video.mp4")
defer f.Close()

resp, err := requests.Post("https://example.com/upload",
    requests.Body{Reader: f},
    requests.Headers{"Content-Type": "video/mp4"},
)
```

### 流式 GET 下载

```go
resp, err := requests.Get("https://example.com/large-file",
    requests.Stream(true),
)
if err != nil {
    panic(err)
}
defer resp.Close()

_, err = io.Copy(dst, resp.Body())
```

### 身份认证

```go
// Basic Auth
resp, err := requests.Get("https://httpbin.org/basic-auth/user/pass",
    requests.Auth{Provider: requests.BasicAuth{Username: "user", Password: "pass"}},
)

// Bearer Token
resp, err := requests.Get("https://api.example.com/resource",
    requests.Auth{Provider: requests.TokenAuth{Token: "my-token"}},
)

// Digest Auth (automatic challenge-response)
resp, err := requests.Get("https://httpbin.org/digest-auth/auth/user/pass",
    requests.Auth{Provider: requests.DigestAuth{Username: "user", Password: "pass"}},
)
```

Digest 认证目前支持 MD5 和 `qop=auth`。如果服务器要求其他算法（例如 SHA-256 或 MD5-sess）或 `qop=auth-int`，会返回明确的不支持 Digest Auth 错误。

### Cookies

```go
resp, err := requests.Get("https://httpbin.org/cookies",
    requests.Cookies{"session": "abc123"},
)
```

包级便捷函数（例如 `requests.Get` 和 `requests.Post`）共享一个默认 session，包括其 cookie jar。使用 `requests.NewSession()` 获取隔离状态，使用 `requests.DefaultSession()` 查看或配置共享 session，或使用 `requests.ResetDefaultSession()` 清除它。

### 超时

```go
resp, err := requests.Get("https://httpbin.org/delay/5",
    requests.Timeout(3*time.Second),
)
```

### 重试

```go
resp, err := requests.Get("https://api.example.com/resource",
    requests.Retry{
        MaxRetries: 3,
        Wait:       200 * time.Millisecond,
    },
)
```

默认情况下，重试仅适用于可重放请求，并会重试状态码 `429`、`500`、`502`、`503` 和 `504`。带有原始流式请求体或文件上传的请求不会自动重试，因为它们的请求体可能无法复用。

### 请求头快捷方式

```go
resp, err := requests.Get("https://api.example.com/resource",
    requests.UserAgent("my-client/1.0"),
    requests.Accept("application/json"),
    requests.Referer("https://example.com"),
)
```

### 禁用重定向

```go
resp, err := requests.Get("https://httpbin.org/redirect/1",
    requests.AllowRedirects(false),
)
fmt.Println(resp.IsRedirect()) // true
```

限制单个请求的重定向次数：

```go
resp, err := requests.Get("https://httpbin.org/redirect/3",
    requests.MaxRedirects(1),
)
```

### 禁用 TLS 验证

```go
resp, err := requests.Get("https://self-signed.example.com",
    requests.Verify(false),
)
```

### 代理

```go
resp, err := requests.Get("https://httpbin.org/get",
    requests.Proxies{"https": "http://proxy.example.com:8080"},
)
```

### 传输层配置

```go
s := requests.NewSession()
s.SetVerify(true)
s.SetProxies(map[string]string{"https": "http://proxy.example.com:8080"})
s.SetTransportConfig(requests.TransportConfig{
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 10,
    IdleConnTimeout:     90 * time.Second,
})
```

你也可以将传输层设置应用到单个请求：

```go
resp, err := requests.Get("https://api.example.com/resource",
    requests.TransportConfig{MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second},
)
```

### 自定义 Client 或 RoundTripper

```go
client := &http.Client{Transport: customTransport}
resp, err := requests.Get("https://api.example.com/resource",
    requests.HTTPClient{Client: client},
)
```

用于 session 级别的自定义：

```go
s := requests.NewSession()
s.SetClient(client)
s.SetRoundTripper(customTransport)
```

### Context

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

resp, err := requests.Get("https://httpbin.org/get",
    requests.WithContext(ctx),
)
```

### Session

`Session` 会在多个请求之间维护持久请求头和 Cookie，并允许你一次性设置默认值。

```go
s := requests.NewSession()
s.SetHeader("Authorization", "Bearer my-token")
s.SetTimeout(10 * time.Second)

// All requests from this session use the auth header and timeout.
resp1, _ := s.Get("https://api.example.com/users")
resp2, _ := s.Get("https://api.example.com/posts")
```

Session Cookie 会在请求之间自动持久化：

```go
s := requests.NewSession()
s.Get("https://httpbin.org/cookies/set?token=abc")  // server sets cookie
resp, _ := s.Get("https://httpbin.org/cookies")     // cookie is sent automatically
```

### 响应

```go
resp, _ := requests.Get("https://httpbin.org/json")

resp.StatusCode    // 200
resp.Status        // "200 OK"
resp.Headers       // http.Header
resp.Cookies       // []*http.Cookie
resp.URL           // *url.URL (final URL after redirects)
resp.Elapsed       // time spent receiving the response

resp.Text()        // body as string
resp.Content()     // body as []byte

var data map[string]interface{}
resp.JSON(&data)   // unmarshal body into data
data, _ = resp.JSONMap()

// Save response content to a file
err := resp.SaveToFile("response.json")

resp.Ok()          // true if status < 400
resp.IsRedirect()  // true if status is 3xx

// Return an error for 4xx/5xx responses
if err := resp.RaiseForStatus(); err != nil {
    log.Fatal(err)
}
```

## 测试

运行本地单元测试：

```bash
go test ./...
```

运行针对 `https://httpbin.org` 的集成测试：

```bash
go test -tags=integration ./...
```

集成测试套件由 `integration` build tag 保护，因为它会发起真实网络请求。如果你不想使用公共服务，可以将 `HTTPBIN_BASE_URL` 设置为兼容的 httpbin 部署地址：

```bash
HTTPBIN_BASE_URL=https://httpbin.org go test -tags=integration ./...
```
