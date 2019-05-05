// Copyright 2019 asana Author. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package context provide the context utils
// Usage:
//
//	import "github.com/goasana/framework/context"
//
//	ctx := context.Context{HTTPRequest:req,ResponseWriter:rw}
//
//  more docs http://asana.me/docs/module/context.md
package context

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/goasana/framework/utils"
)

//commonly used mime-types
const (
	ApplicationHTML     = "application/xhtml+xml"
	ApplicationJSON     = "application/json"
	ApplicationJSONP    = "application/javascript"
	ApplicationXML      = "application/xml"
	ApplicationYAML     = "application/x-yaml"
	ApplicationProtoBuf = "application/x-protobuf"
	TextXML             = "text/xml"
	TextHTML            = "text/html"
	TextPlain           = "text/plain"
)

// NewContext return the Context with Request and Response
func NewContext() *Context {
	return &Context{
		Request:  NewRequest(),
		Response: NewResponse(),
	}
}

// Context Http request context struct including AsanaRequest, AsanaResponse, http.HTTPRequest and http.ResponseWriter.
// AsanaRequest and AsanaResponse provides some api to operate request and response more easily.
type Context struct {
	Request        *AsanaRequest
	Response       *AsanaResponse
	HTTPRequest    *http.Request
	ResponseWriter *Response
	_xsrfToken     string
}

// Reset init Context, AsanaRequest and AsanaResponse
func (ctx *Context) Reset(rw http.ResponseWriter, r *http.Request) {
	ctx.HTTPRequest = r
	if ctx.ResponseWriter == nil {
		ctx.ResponseWriter = &Response{}
	}
	ctx.ResponseWriter.reset(rw)
	ctx.Request.Reset(ctx)
	ctx.Response.Reset(ctx)
	ctx._xsrfToken = ""
}

// Redirect does redirection to localUrl with http header status code.
func (ctx *Context) Redirect(status int, localUrl string) {
	http.Redirect(ctx.ResponseWriter, ctx.HTTPRequest, localUrl, status)
}

// Abort stops this request.
// if asana.ErrorMaps exists, panic body.
func (ctx *Context) Abort(status int, body string) {
	ctx.Response.SetStatus(status)
	panic(body)
}

// WriteString Write string to response body.
// it sends response body.
func (ctx *Context) WriteString(content string) {
	_, _ = ctx.ResponseWriter.Write([]byte(content))
}

// GetCookie Get cookie from request by a given key.
// It's alias of AsanaRequest.Cookie.
func (ctx *Context) GetCookie(key string) string {
	return ctx.Request.Cookie(key)
}

// SetCookie Set cookie for response.
// It's alias of AsanaResponse.Cookie.
func (ctx *Context) SetCookie(name string, value string, others ...interface{}) {
	ctx.Response.Cookie(name, value, others...)
}

// GetSecureCookie Get secure cookie from request by a given key.
func (ctx *Context) GetSecureCookie(Secret, key string) (string, bool) {
	val := ctx.Request.Cookie(key)
	if val == "" {
		return "", false
	}

	parts := strings.SplitN(val, "|", 3)

	if len(parts) != 3 {
		return "", false
	}

	vs := parts[0]
	timestamp := parts[1]
	sig := parts[2]

	h := hmac.New(sha1.New, []byte(Secret))
	_, _ = fmt.Fprintf(h, "%s%s", vs, timestamp)

	if fmt.Sprintf("%02x", h.Sum(nil)) != sig {
		return "", false
	}
	res, _ := base64.URLEncoding.DecodeString(vs)
	return string(res), true
}

// SetSecureCookie Set Secure cookie for response.
func (ctx *Context) SetSecureCookie(Secret, name, value string, others ...interface{}) {
	vs := base64.URLEncoding.EncodeToString([]byte(value))
	timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	h := hmac.New(sha1.New, []byte(Secret))
	_, _ = fmt.Fprintf(h, "%s%s", vs, timestamp)
	sig := fmt.Sprintf("%02x", h.Sum(nil))
	cookie := strings.Join([]string{vs, timestamp, sig}, "|")
	ctx.Response.Cookie(name, cookie, others...)
}

// XSRFToken creates a xsrf token string and returns.
func (ctx *Context) XSRFToken(key string, expire int64) string {
	if ctx._xsrfToken == "" {
		token, ok := ctx.GetSecureCookie(key, "_xsrf")
		if !ok {
			token = string(utils.RandomCreateBytes(32))
			ctx.SetSecureCookie(key, "_xsrf", token, expire)
		}
		ctx._xsrfToken = token
	}
	return ctx._xsrfToken
}

// CheckXSRFCookie checks xsrf token in this request is valid or not.
// the token can provided in request header "X-Xsrftoken" and "X-CsrfToken"
// or in form field value named as "_xsrf".
func (ctx *Context) CheckXSRFCookie() bool {
	token := ctx.Request.Query("_xsrf")
	if token == "" {
		token = ctx.HTTPRequest.Header.Get("X-Xsrftoken")
	}
	if token == "" {
		token = ctx.HTTPRequest.Header.Get("X-Csrftoken")
	}
	if token == "" {
		ctx.Abort(403, "'_xsrf' argument missing from POST")
		return false
	}
	if ctx._xsrfToken != token {
		ctx.Abort(403, "XSRF cookie does not match POST argument")
		return false
	}
	return true
}

// RenderMethodResult renders the return value of a controller method to the output
func (ctx *Context) RenderMethodResult(result interface{}) {
	if result != nil {
		renderer, ok := result.(Renderer)
		if !ok {
			err, ok := result.(error)
			if ok {
				renderer = errorRenderer(err)
			} else {
				renderer = jsonRenderer(result)
			}
		}
		renderer.Render(ctx)
	}
}

//Response is a wrapper for the http.ResponseWriter
//started set to true if response was written to then don't execute other handler
type Response struct {
	http.ResponseWriter
	Started bool
	Status  int
	Elapsed time.Duration
}

func (r *Response) reset(rw http.ResponseWriter) {
	r.ResponseWriter = rw
	r.Status = 0
	r.Started = false
}

// Write writes the data to the connection as part of an HTTP reply,
// and sets `started` to true.
// started means the response has sent out.
func (r *Response) Write(p []byte) (int, error) {
	r.Started = true
	return r.ResponseWriter.Write(p)
}

// WriteHeader sends an HTTP response header with status code,
// and sets `started` to true.
func (r *Response) WriteHeader(code int) {
	if r.Status > 0 {
		//prevent multiple response.WriteHeader calls
		return
	}
	r.Status = code
	r.Started = true
	r.ResponseWriter.WriteHeader(code)
}

// Hijack hijacker for http
func (r *Response) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("webserver doesn't support hijacking")
	}
	return hj.Hijack()
}

// Flush http.Flusher
func (r *Response) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// CloseNotify http.CloseNotifier
func (r *Response) CloseNotify() <-chan bool {
	if cn, ok := r.ResponseWriter.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	return nil
}

// Pusher http.Pusher
func (r *Response) Pusher() (pusher http.Pusher) {
	if pusher, ok := r.ResponseWriter.(http.Pusher); ok {
		return pusher
	}
	return nil
}
