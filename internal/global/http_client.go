package global

import (
	"net"
	"net/http"
	"time"
)

// Transport 是本进程所有出站 HTTP 请求（上游接口 + 图片/静态资源回环）共用的连接池。
//
// 为什么必须显式调大：`http.DefaultTransport` 的 `MaxIdleConnsPerHost = 2`，
// 而本项目最典型的负载恰好是**同一 host 的极高并发** —— 渲染一章 PDF 要经
// `/v2/file/proxy` 拉 60~68 张图，课程级导出就是几千次同 host 请求。
// 用默认值时几乎每条连接用完都进不了空闲池、被直接关闭并重新建连，
// 是冷渲染慢的头号嫌疑（代码注释里早有记录）。
//
// ⚠️ 改这里等于改所有出站请求的行为，别顺手加 `Timeout` ——「是否给某个客户端
// 设置总超时」是各调用方自己的语义（见 authority.go 要 5 分钟、AI 流式则不能设）。
var Transport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          512,
	MaxIdleConnsPerHost:   128,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

var (
	// HttpClient 供上游接口与图片拉取使用。
	//
	// 刻意**不设** `Timeout`（与原先的 `http.DefaultClient` 行为一致，零值即不超时）：
	// 这是本次唯一的行为变化点 —— 只换连接池，不动超时语义。
	// 注意 `authority.Authority` 会在登录/刷新 Cookie 时把它替换成带 CookieJar 的实例
	//（仍然复用上面的 Transport）。
	HttpClient = &http.Client{Transport: Transport}
)

// NewLoopbackTransport 返回一个用于**本机回环**（127.0.0.1）的 Transport。
//
// 与 Transport 的区别只有两点，都是刻意的：
//  1. 不挂 ProxyFromEnvironment —— 目标是自己的 8090，不该受本机 HTTP 代理设置影响；
//  2. 不做 HTTP/2（回环是明文 http，用不上）。
//
// 典型用途：PDF 渲染时每章起的临时 HTTP server 把 `/v2/file/proxy` 转发回主进程。
func NewLoopbackTransport() *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   128,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
