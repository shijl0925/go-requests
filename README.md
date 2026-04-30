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
- File uploads (`Files`)
- Cookie management (`Cookies`)
- Authentication: Basic Auth, Bearer Token, Digest Auth
- Request timeout (`Timeout`)
- Redirect control (`AllowRedirects`)
- TLS verification control (`Verify`)
- Proxy support (`Proxies`)
- Context support (`WithContext`)
- Response helpers: `.Text()`, `.JSON()`, `.Content()`, `.Ok()`, `.RaiseForStatus()`, `.IsRedirect()`

## Usage

### GET Request

```go
resp, err := requests.Get("https://httpbin.org/get",
    requests.Params{"page": "1", "limit": "10"},
    requests.Headers{"Accept": "application/json"},
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

### Cookies

```go
resp, err := requests.Get("https://httpbin.org/cookies",
    requests.Cookies{"session": "abc123"},
)
```

### Timeout

```go
resp, err := requests.Get("https://httpbin.org/delay/5",
    requests.Timeout(3*time.Second),
)
```

### Disable Redirects

```go
resp, err := requests.Get("https://httpbin.org/redirect/1",
    requests.AllowRedirects(false),
)
fmt.Println(resp.IsRedirect()) // true
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

### Context

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

resp, err := requests.Get("https://httpbin.org/get",
    requests.WithContext{Ctx: ctx},
)
```

### Session

A `Session` maintains persistent headers and cookies across requests, and lets you set defaults once.

```go
s := requests.NewSession()
s.Headers.Set("Authorization", "Bearer my-token")
s.Timeout = 10 * time.Second

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

resp.Text()        // body as string
resp.Content()     // body as []byte

var data map[string]interface{}
resp.JSON(&data)   // unmarshal body into data

resp.Ok()          // true if status < 400
resp.IsRedirect()  // true if status is 3xx

// Return an error for 4xx/5xx responses
if err := resp.RaiseForStatus(); err != nil {
    log.Fatal(err)
}
```