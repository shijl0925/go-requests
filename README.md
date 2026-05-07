# go-requests

A user-friendly Go HTTP client library inspired by Python's [requests](https://requests.readthedocs.io/) library. It wraps the standard `net/http` package to provide a simpler, more expressive API.

## Installation

```bash
go get github.com/shijl0925/go-requests
```

## Quick Start

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

## Features

- Simple package-level functions: `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`
- `Session` for persistent headers, cookies, and settings across requests
- URL query parameters (`Params`)
- Request headers (`Headers`)
- JSON request body (`JSON`, `JSONBody`)
- Form data body (`Data`)
- Multi-value query and form fields (`ParamValues`, `DataValues`)
- File uploads (`Files`)
- Streaming uploads (`Body`, streamed multipart `Files`)
- Cookie management (`Cookies`)
- Authentication: Basic Auth, Bearer Token, Digest Auth
- Request timeout (`Timeout`)
- Automatic retries for replayable requests (`Retry`)
- Redirect control (`AllowRedirects`, `MaxRedirects`)
- TLS verification control (`Verify`)
- Proxy support (`Proxies`)
- Transport and connection-pool configuration (`TransportConfig`)
- Custom HTTP clients and transports (`HTTPClient`, `RoundTripper`)
- Context support (`WithContext`)
- Common header shortcuts (`Header`, `UserAgent`, `Referer`, `Accept`, `ContentType`)
- Streaming responses (`Stream`)
- Response helpers: `.Text()`, `.JSON()`, `.JSONMap()`, `.JSONSlice()`, `.Content()`, `.SaveToFile()`, `.Ok()`, `.RaiseForStatus()`, `.IsRedirect()`

## Usage

### GET Request

```go
resp, err := requests.Get("https://httpbin.org/get",
    requests.Params{"page": "1", "limit": "10"},
    requests.Headers{"Accept": "application/json"},
)
```

Use `ParamValues` when the same query key needs multiple values:

```go
resp, err := requests.Get("https://httpbin.org/get",
    requests.ParamValues{"tag": {"go", "requests"}},
)
```

### POST with JSON Body

```go
resp, err := requests.Post("https://httpbin.org/post",
    requests.JSON{"name": "Alice", "age": 30},
)
```

### POST with a Struct

```go
type User struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

resp, err := requests.Post("https://httpbin.org/post",
    requests.JSONBody{Value: User{Name: "Bob", Age: 25}},
)
```

### POST Form Data

```go
resp, err := requests.Post("https://httpbin.org/post",
    requests.Data{"username": "admin", "password": "secret"},
)
```

Use `DataValues` when a form field needs repeated values:

```go
resp, err := requests.Post("https://httpbin.org/post",
    requests.DataValues{"tag": {"go", "requests"}},
)
```

### File Upload

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

Multipart file uploads stream from the provided reader instead of buffering the
entire file in memory.

### Streaming POST Upload

```go
f, _ := os.Open("large-video.mp4")
defer f.Close()

resp, err := requests.Post("https://example.com/upload",
    requests.Body{Reader: f},
    requests.Headers{"Content-Type": "video/mp4"},
)
```

### Streaming GET Download

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

### Authentication

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

Digest authentication currently supports MD5 with `qop=auth`. Servers that
require other algorithms (for example SHA-256 or MD5-sess) or `qop=auth-int`
return an explicit unsupported digest auth error.

### Cookies

```go
resp, err := requests.Get("https://httpbin.org/cookies",
    requests.Cookies{"session": "abc123"},
)
```

Package-level convenience functions such as `requests.Get` and `requests.Post`
share a default session, including its cookie jar. Use `requests.NewSession()`
for isolated state, `requests.DefaultSession()` to inspect or configure the
shared session, or `requests.ResetDefaultSession()` to clear it.

### Timeout

```go
resp, err := requests.Get("https://httpbin.org/delay/5",
    requests.Timeout(3*time.Second),
)
```

### Retry

```go
resp, err := requests.Get("https://api.example.com/resource",
    requests.Retry{
        MaxRetries: 3,
        Wait:       200 * time.Millisecond,
    },
)
```

By default, retries apply to replayable requests and retry status codes `429`,
`500`, `502`, `503`, and `504`. Requests with raw streaming bodies or file
uploads are not retried automatically because their bodies may not be reusable.

### Header Shortcuts

```go
resp, err := requests.Get("https://api.example.com/resource",
    requests.UserAgent("my-client/1.0"),
    requests.Accept("application/json"),
    requests.Referer("https://example.com"),
)
```

### Disable Redirects

```go
resp, err := requests.Get("https://httpbin.org/redirect/1",
    requests.AllowRedirects(false),
)
fmt.Println(resp.IsRedirect()) // true
```

Limit redirects for a single request:

```go
resp, err := requests.Get("https://httpbin.org/redirect/3",
    requests.MaxRedirects(1),
)
```

### Disable TLS Verification

```go
resp, err := requests.Get("https://self-signed.example.com",
    requests.Verify(false),
)
```

### Proxy

```go
resp, err := requests.Get("https://httpbin.org/get",
    requests.Proxies{"https": "http://proxy.example.com:8080"},
)
```

### Transport Configuration

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

You can also apply transport settings to a single request:

```go
resp, err := requests.Get("https://api.example.com/resource",
    requests.TransportConfig{MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second},
)
```

### Custom Client or RoundTripper

```go
client := &http.Client{Transport: customTransport}
resp, err := requests.Get("https://api.example.com/resource",
    requests.HTTPClient{Client: client},
)
```

For session-wide customization:

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

A `Session` maintains persistent headers and cookies across requests, and lets you set defaults once.

```go
s := requests.NewSession()
s.SetHeader("Authorization", "Bearer my-token")
s.SetTimeout(10 * time.Second)

// All requests from this session use the auth header and timeout.
resp1, _ := s.Get("https://api.example.com/users")
resp2, _ := s.Get("https://api.example.com/posts")
```

Session cookies are automatically persisted between requests:

```go
s := requests.NewSession()
s.Get("https://httpbin.org/cookies/set?token=abc")  // server sets cookie
resp, _ := s.Get("https://httpbin.org/cookies")     // cookie is sent automatically
```

### Response

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

## Testing

Run the local unit tests:

```bash
go test ./...
```

Run the integration tests against `https://httpbin.org`:

```bash
go test -tags=integration ./...
```

The integration suite is guarded by the `integration` build tag because it makes
real network requests. Set `HTTPBIN_BASE_URL` to point at a compatible httpbin
deployment if you do not want to use the public service:

```bash
HTTPBIN_BASE_URL=https://httpbin.org go test -tags=integration ./...
```
