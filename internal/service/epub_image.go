package service

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/zkep/my-geektime/internal/global"
	"golang.org/x/net/html"
	"go.uber.org/zap"
)

// EPUB 图片内嵌。
//
// 实测本机缓存命中率仅约 14%，多数图片需要联网下载，所以必须有护栏：
// 并发受限、单图超时、单图与整本总量上限，且任何一张图失败都不能让整本导出失败。
//
// 缓存 key 与 /v2/file/proxy 完全一致（{cache_prefix}/{md5(url)}），
// 因此这里回写的缓存天然幂等，不会污染或破坏已有缓存，反而顺带把网页端要用的图也缓存了。

const (
	// epubImageWorkers 并发下载数
	epubImageWorkers = 6
	// epubImageTimeout 单张图片的超时时间
	epubImageTimeout = 15 * time.Second
	// epubImageMaxBytes 单张图片大小上限
	epubImageMaxBytes = 10 << 20
	// epubImageTotalMaxBytes 整本电子书的图片总量上限，超出后停止下载
	epubImageTotalMaxBytes = 500 << 20
	// epubDefaultCachePrefix 与 config.yml site.proxy.cache_prefix 默认值保持一致
	epubDefaultCachePrefix = "resource"
)

// epubImageAsset 一张已内嵌（或放弃内嵌）的图片。
type epubImageAsset struct {
	URL       string
	// Name 是 epub 内的文件名，形如 ab12cd.jpg
	Name string
	// ManifestHref 相对 OEBPS 的路径，供 content.opf 使用
	ManifestHref string
	// TextHref 相对 OEBPS/text/ 的路径，供正文引用
	TextHref  string
	MediaType string
	Data      []byte
	Failed    bool
}

// epubImageCollector 收集、去重、下载并索引一本书用到的全部图片。
type epubImageCollector struct {
	mu     sync.Mutex
	assets map[string]*epubImageAsset
	order  []string
	// total 已计入的内嵌体积（压缩后）
	total int64
	// rawTotal 原始体积，仅用于日志展示压缩收益
	rawTotal int64
}

func newEpubImageCollector() *epubImageCollector {
	return &epubImageCollector{assets: make(map[string]*epubImageAsset)}
}

// add 登记一个待内嵌的图片 URL（仅登记，不下载）。
// 只处理代理白名单覆盖的域名——与网页端共用缓存的前提。
func (c *epubImageCollector) add(url string) {
	url = strings.TrimSpace(url)
	if url == "" || strings.HasPrefix(url, "data:") {
		return
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return
	}
	if !PorxyMatch(url) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.assets[url]; ok {
		return
	}
	c.assets[url] = &epubImageAsset{URL: url}
	c.order = append(c.order, url)
}

// addFromHTML 从 HTML 片段里收集 img@src 与 video@poster。
func (c *epubImageCollector) addFromHTML(raw string) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "img":
				c.add(epubAttr(n, "src"))
			case "video":
				c.add(epubAttr(n, "poster"))
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
}

// href 给正文转换器使用：把 URL 换成 epub 内相对路径。
func (c *epubImageCollector) href(url string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	asset, ok := c.assets[strings.TrimSpace(url)]
	if !ok || asset.Failed || asset.TextHref == "" {
		return "", false
	}
	return asset.TextHref, true
}

// asset 取回某个 URL 的图片资源（仅成功内嵌的才返回 true）。
func (c *epubImageCollector) asset(url string) (*epubImageAsset, bool) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.assets[url]
	if !ok || a.Failed || len(a.Data) == 0 || a.ManifestHref == "" {
		return nil, false
	}
	return a, true
}

// list 返回全部成功内嵌的图片，顺序与登记顺序一致。
func (c *epubImageCollector) list() []*epubImageAsset {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*epubImageAsset, 0, len(c.order))
	for _, u := range c.order {
		if a := c.assets[u]; a != nil && !a.Failed && len(a.Data) > 0 {
			out = append(out, a)
		}
	}
	return out
}

// fetch 并发拉取全部图片。缓存命中则直接读缓存，未命中则下载并回写缓存。
func (c *epubImageCollector) fetch(ctx context.Context) {
	urls := append([]string(nil), c.order...)
	if len(urls) == 0 {
		return
	}
	start := time.Now()
	sem := make(chan struct{}, epubImageWorkers)
	var wg sync.WaitGroup
	for _, url := range urls {
		wg.Add(1)
		sem <- struct{}{}
		go func(url string) {
			defer wg.Done()
			defer func() { <-sem }()
			c.fetchOne(ctx, url)
		}(url)
	}
	wg.Wait()

	embedded := len(c.list())
	c.mu.Lock()
	rawMB := c.rawTotal >> 20
	embedMB := c.total >> 20
	c.mu.Unlock()
	global.LOG.Info("epub images fetched",
		zap.Int("total", len(urls)),
		zap.Int("embedded", embedded),
		zap.Int("failed", len(urls)-embedded),
		zap.Int64("rawMB", rawMB),
		zap.Int64("embeddedMB", embedMB),
		zap.Duration("cost", time.Since(start)))
}

// fetchOne 获取单张图片：先查共享缓存，未命中再下载并回写，最后按需压缩出内嵌副本。
func (c *epubImageCollector) fetchOne(ctx context.Context, url string) {
	key := path.Join(epubCachePrefix(), epubMD5(url))
	data, mediaType, ok := c.readCache(key, url)
	if !ok {
		data, mediaType, ok = c.download(ctx, url)
		if !ok {
			c.markFailed(url)
			return
		}
		// 缓存里写的永远是**原始字节**，key 与 /v2/file/proxy 完全一致，保持幂等。
		// 压缩结果绝不能写回这里——否则网页端显示的原图也会被换成缩略图。
		c.writeCache(key, url, data)
	}
	// 只有内嵌进 epub 的那一份做瘦身（等比缩放 + 转 JPEG）。
	// 原始高清截图会让整本电子书膨胀到数百 MB，多数阅读器打不开。
	embedded, embeddedType := epubShrinkImage(data, mediaType)
	if len(embedded) == 0 {
		embedded, embeddedType = data, mediaType
	}
	ext := epubExtByMediaType(embeddedType)
	if ext == "" {
		c.markFailed(url)
		return
	}
	// 按最终内嵌体积计账。缓存命中同样要计入，否则整本体积上限形同虚设。
	if err := c.reserve(len(data), len(embedded)); err != nil {
		global.LOG.Warn("epub image skipped", zap.String("url", url), zap.Error(err))
		c.markFailed(url)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if asset, exist := c.assets[url]; exist {
		asset.Name = epubMD5(url) + "." + ext
		asset.ManifestHref = "images/" + asset.Name
		asset.TextHref = "../images/" + asset.Name
		asset.MediaType = embeddedType
		asset.Data = embedded
		asset.Failed = false
	}
}

// readCache 读取共享图片缓存。
// 缓存文件没有 Content-Type，只能按内容魔数嗅探。
func (c *epubImageCollector) readCache(key, url string) ([]byte, string, bool) {
	if !global.CONF.Site.Proxy.Cache {
		return nil, "", false
	}
	rc, stat, err := global.Storage.Get(key)
	if err != nil {
		return nil, "", false
	}
	defer func() { _ = rc.Close() }()
	if stat != nil && (stat.Size() <= 0 || stat.Size() > epubImageMaxBytes) {
		return nil, "", false
	}
	data, err := io.ReadAll(io.LimitReader(rc, epubImageMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > epubImageMaxBytes {
		return nil, "", false
	}
	mediaType, ok := epubSniffImage(data, "")
	if !ok {
		// 缓存里可能是错误页或非图片内容，当作未命中重新下载
		return nil, "", false
	}
	return data, mediaType, true
}

// download 联网拉取单张图片（带 Referer，行为对齐 /v2/file/proxy）。
func (c *epubImageCollector) download(ctx context.Context, url string) ([]byte, string, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, epubImageTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false
	}
	req.Header.Set("Referer", url)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, epubImageMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > epubImageMaxBytes {
		return nil, "", false
	}
	mediaType, ok := epubSniffImage(data, resp.Header.Get("Content-Type"))
	if !ok {
		return nil, "", false
	}
	return data, mediaType, true
}

// writeCache 回写共享图片缓存。
// 仅在 site.proxy.cache 打开时写入，key 与 /v2/file/proxy 完全一致，幂等。
func (c *epubImageCollector) writeCache(key, url string, data []byte) {
	if !global.CONF.Site.Proxy.Cache {
		return
	}
	if _, err := global.Storage.Put(key, io.NopCloser(bytes.NewReader(data))); err != nil {
		global.LOG.Warn("epub image cache put failed",
			zap.String("url", url), zap.String("key", key), zap.Error(err))
	}
}

// reserve 累加已占用体积，超过整本上限则拒绝。
// embedSize 是压缩后的内嵌体积（真正计入 epub 的部分），rawSize 仅作统计。
func (c *epubImageCollector) reserve(rawSize, embedSize int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total+int64(embedSize) > epubImageTotalMaxBytes {
		return fmt.Errorf("epub image total size exceeds limit %d MB", epubImageTotalMaxBytes>>20)
	}
	c.total += int64(embedSize)
	c.rawTotal += int64(rawSize)
	return nil
}

func (c *epubImageCollector) markFailed(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if asset, ok := c.assets[url]; ok {
		asset.Failed = true
		asset.Data = nil
	}
}

// epubCachePrefix 图片缓存前缀，空值时回落到默认值。
func epubCachePrefix() string {
	if v := strings.TrimSpace(global.CONF.Site.Proxy.CachePrefix); v != "" {
		return v
	}
	return epubDefaultCachePrefix
}

// epubMD5 与 /v2/file/proxy 的缓存 key 保持完全一致。
func epubMD5(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// epubSniffImage 按内容魔数识别图片类型，魔数不识别时才退回 Content-Type。
// 反过来（先信 Content-Type）会把 200 的错误页当成图片写进 epub。
func epubSniffImage(data []byte, contentType string) (string, bool) {
	switch {
	case len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg", true
	case len(data) > 8 && bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", true
	case len(data) > 6 && (bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a"))):
		return "image/gif", true
	case len(data) > 12 && bytes.HasPrefix(data, []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", true
	case len(data) > 2 && data[0] == 'B' && data[1] == 'M':
		return "image/bmp", true
	}
	// SVG 是文本格式，没有魔数，只能看开头
	headLen := len(data)
	if headLen > 512 {
		headLen = 512
	}
	head := strings.TrimSpace(string(data[:headLen]))
	if strings.HasPrefix(head, "<svg") || (strings.HasPrefix(head, "<?xml") && strings.Contains(head, "<svg")) {
		return "image/svg+xml", true
	}
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/jpeg", "image/jpg", "image/pjpeg":
		return "image/jpeg", true
	case "image/png", "image/x-png":
		return "image/png", true
	case "image/gif":
		return "image/gif", true
	case "image/webp":
		return "image/webp", true
	case "image/svg+xml":
		return "image/svg+xml", true
	}
	return "", false
}

// epubExtByMediaType 由 media type 推导文件扩展名。
func epubExtByMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/bmp":
		return "bmp"
	case "image/svg+xml":
		return "svg"
	default:
		return ""
	}
}
