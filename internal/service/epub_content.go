package service

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// EPUB 正文与评论的 HTML → XHTML 转换。
//
// 设计要点：
//  1. 正文是不可控的第三方 HTML，必须先容错解析再清洗，不能做正则替换；
//  2. x/net/html 的渲染器会把 HTML 实体解码成真实字符（因此 &nbsp; 不会变成
//     XML 未定义实体），并为 void 元素输出自闭合写法，天然接近合法 XHTML；
//  3. 最终仍逐篇做 XML 校验，不通过则整体降级为纯文本，
//     避免其中一个坏标签毁掉整本电子书。

// epubDroppedElements 会被整块删除的元素：阅读器不支持脚本/表单，
// 留着既无意义又可能破坏 XHTML 校验。
var epubDroppedElements = map[string]bool{
	"script":   true,
	"style":    true,
	"iframe":   true,
	"noscript": true,
	"template": true,
	"form":     true,
	"button":   true,
	"input":    true,
	"select":   true,
	"textarea": true,
	"frame":    true,
	"frameset": true,
	"object":   true,
	"embed":    true,
	"applet":   true,
}

// epubMediaElements 无法内嵌的媒体元素，降级为静态图 + 提示文案。
var epubMediaElements = map[string]bool{
	"video": true,
	"audio": true,
}

// epubImageDroppedAttrs 在重写 src 后必须丢弃的属性。
// srcset/sizes 里是原始外链，留在 epub 内会指向不存在的相对路径。
var epubImageDroppedAttrs = map[string]bool{
	"srcset":         true,
	"sizes":          true,
	"loading":        true,
	"decoding":       true,
	"fetchpriority":  true,
	"crossorigin":    true,
	"referrerpolicy": true,
	"data-src":       true,
	"longdesc":       true,
}

// epubXMLNameRe XML 合法名称：以字母/下划线开头，后接字母数字与 _.-
// HTML 里常见的 @click、:class 这类属性名在 XML 中非法，必须剔除。
var epubXMLNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// epubConverter 把 HTML 片段转换为 XHTML 片段。
type epubConverter struct {
	// resolveImage 把图片 URL 换成 epub 内部相对路径，返回 false 表示未内嵌。
	resolveImage func(src string) (string, bool)
	// stripImage 为 true 时直接删除 img（评论区用它避免头像裂图）。
	stripImage bool
	// mediaHint 音视频元素的降级提示，空则使用默认文案。
	mediaHint string
}

// newEpubConverter 创建带图片内嵌能力的转换器（用于正文）。
func newEpubConverter(resolveImage func(string) (string, bool)) *epubConverter {
	return &epubConverter{resolveImage: resolveImage}
}

// newEpubTextConverter 创建纯文本型转换器（用于评论与讨论：保留排版，剔除图片）。
func newEpubTextConverter() *epubConverter {
	return &epubConverter{stripImage: true}
}

// Body 把 HTML 片段转换为 XHTML 片段（不含 html/body 外壳）。
func (c *epubConverter) Body(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return ""
	}
	// html.Parse 会把片段补全成完整文档（html/head/body），
	// 只取 body 的子节点，否则会嵌出一层多余的 html/head。
	body := epubFindBody(doc)
	if body == nil {
		return ""
	}
	c.clean(body)
	var buf bytes.Buffer
	for n := body.FirstChild; n != nil; n = n.NextSibling {
		if err := html.Render(&buf, n); err != nil {
			return ""
		}
	}
	return buf.String()
}

// clean 递归清洗节点树。
func (c *epubConverter) clean(parent *html.Node) {
	for node := parent.FirstChild; node != nil; {
		next := node.NextSibling
		switch node.Type {
		case html.CommentNode:
			// 注释内容可能含 "--"，在 XML 中非法，直接删除最省事。
			parent.RemoveChild(node)
		case html.TextNode:
			node.Data = epubStripInvalidXMLChars(node.Data)
		case html.ElementNode:
			name := strings.ToLower(node.Data)
			switch {
			case epubDroppedElements[name]:
				parent.RemoveChild(node)
			case name == "img":
				c.handleImage(node)
				if node.Parent == nil {
					// 已被删除（stripImage 模式）
					break
				}
				c.clean(node)
			case epubMediaElements[name]:
				c.handleMedia(node, parent)
			case name == "a":
				// 缓存内容里的站内链接已失效，去掉链接但保留文字。
				c.clean(node)
				epubUnwrapNode(node)
			default:
				c.sanitizeAttrs(node)
				c.clean(node)
			}
		}
		node = next
	}
}

// sanitizeAttrs 剔除在 XHTML 中非法或有害的属性。
func (c *epubConverter) sanitizeAttrs(n *html.Node) {
	attrs := n.Attr[:0]
	for _, a := range n.Attr {
		// 带命名空间的属性（xlink:href 之类）需要额外声明命名空间，直接丢弃。
		if a.Namespace != "" {
			continue
		}
		if !epubXMLNameRe.MatchString(a.Key) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(a.Key), "on") {
			// 事件绑定属性
			continue
		}
		a.Val = epubStripInvalidXMLChars(a.Val)
		attrs = append(attrs, a)
	}
	n.Attr = attrs
}

// resolveImage0 安全调用图片解析回调（未配置回调时视为「不内嵌」）。
func (c *epubConverter) resolveImage0(src string) (string, bool) {
	if c.resolveImage == nil {
		return "", false
	}
	return c.resolveImage(src)
}

// handleImage 重写图片引用；未内嵌的图片保留原始外链，失败时不做处理。
func (c *epubConverter) handleImage(n *html.Node) {
	if c.stripImage {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
		return
	}
	src := epubAttr(n, "src")
	if src == "" {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
		return
	}
	if href, ok := c.resolveImage0(src); ok && href != "" {
		epubSetAttr(n, "src", href)
	}
	for key := range epubImageDroppedAttrs {
		epubDelAttr(n, key)
	}
	// 正文里图片常缺 alt，补一个避免部分阅读器排版异常
	if epubAttr(n, "alt") == "" {
		epubSetAttr(n, "alt", "图片")
	}
	// 上游图片 URL 自带 ?wh=WxH 尺寸提示，写进属性可避免阅读器排版错乱。
	if w, h := epubImageSize(src); w != "" {
		epubSetAttr(n, "width", w)
		epubSetAttr(n, "height", h)
	}
}

// handleMedia 把无法内嵌的音视频降级为静态封面 + 提示文案。
func (c *epubConverter) handleMedia(node, parent *html.Node) {
	if parent == nil {
		return
	}
	hint := c.mediaHint
	if hint == "" {
		hint = "（本文含音视频内容，请在网页端观看）"
	}
	replacements := make([]*html.Node, 0, 2)
	poster := epubAttr(node, "poster")
	if poster != "" && !c.stripImage {
		if p, ok := c.resolveImage0(poster); ok && p != "" {
			replacements = append(replacements, epubNewImage(p))
		}
	}
	replacements = append(replacements, epubNewTextElement("p", "media-hint", hint))
	epubReplaceNode(parent, node, replacements...)
}

// epubNewImage 构造一个自闭合的 img 节点。
func epubNewImage(src string) *html.Node {
	n := epubNewElement("img")
	n.Attr = []html.Attribute{
		{Key: "src", Val: src},
		{Key: "alt", Val: "视频封面"},
	}
	return n
}

// epubNewTextElement 构造 <name class="class">text</name>。
func epubNewTextElement(name, class, text string) *html.Node {
	n := epubNewElement(name)
	if class != "" {
		n.Attr = []html.Attribute{{Key: "class", Val: class}}
	}
	if text != "" {
		n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
	}
	return n
}

// epubNewElement 构造元素节点（同时填充 DataAtom，html.Render 依赖它输出标签名）。
func epubNewElement(name string) *html.Node {
	return &html.Node{
		Type:     html.ElementNode,
		Data:     name,
		DataAtom: atom.Lookup([]byte(name)),
	}
}

// epubReplaceNode 用 replacements 替换 ref 节点。
func epubReplaceNode(parent, ref *html.Node, replacements ...*html.Node) {
	for _, n := range replacements {
		parent.InsertBefore(n, ref)
	}
	parent.RemoveChild(ref)
}

// epubUnwrapNode 去掉节点本身、把子节点提升到父节点（用于剥掉 <a>）。
func epubUnwrapNode(n *html.Node) {
	parent := n.Parent
	if parent == nil {
		return
	}
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		n.RemoveChild(child)
		parent.InsertBefore(child, n)
		child = next
	}
	parent.RemoveChild(n)
}

// epubFindBody 在解析结果里找到 body 节点。
func epubFindBody(root *html.Node) *html.Node {
	if root.Type == html.ElementNode && root.Data == "body" {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := epubFindBody(child); found != nil {
			return found
		}
	}
	return nil
}

// epubAttr 读取属性值（大小写不敏感）。
func epubAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// epubSetAttr 设置属性值。
func epubSetAttr(n *html.Node, key, val string) {
	for i := range n.Attr {
		if strings.EqualFold(n.Attr[i].Key, key) {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

// epubDelAttr 删除属性。
func epubDelAttr(n *html.Node, keys ...string) {
	if len(keys) == 0 || len(n.Attr) == 0 {
		return
	}
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[strings.ToLower(k)] = true
	}
	attrs := n.Attr[:0]
	for _, a := range n.Attr {
		if drop[strings.ToLower(a.Key)] {
			continue
		}
		attrs = append(attrs, a)
	}
	n.Attr = attrs
}

// epubImageSize 从上游图片 URL 的 ?wh=WxH 参数里取出宽高。
func epubImageSize(src string) (string, string) {
	i := strings.Index(src, "wh=")
	if i < 0 {
		return "", ""
	}
	v := src[i+len("wh="):]
	if j := strings.IndexAny(v, "&#"); j >= 0 {
		v = v[:j]
	}
	lw, lh, ok := strings.Cut(v, "x")
	if !ok || !epubAllDigits(lw) || !epubAllDigits(lh) {
		return "", ""
	}
	return lw, lh
}

func epubAllDigits(s string) bool {
	if s == "" || len(s) > 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// epubValidXMLRune 判断字符是否属于 XML 1.0 允许的字符集。
func epubValidXMLRune(r rune) bool {
	switch {
	case r == 0x9 || r == 0xA || r == 0xD:
		return true
	case r >= 0x20 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	default:
		return false
	}
}

// epubStripInvalidXMLChars 剔除 XML 不允许的控制字符。
// 上游正文偶尔混入 \x00-\x08 这类字符，会让整篇 XHTML 校验失败。
func epubStripInvalidXMLChars(s string) string {
	if strings.IndexFunc(s, func(r rune) bool { return !epubValidXMLRune(r) }) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if epubValidXMLRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// epubIsValidXML 用 XML 解码器逐 token 校验整篇文档。
// 比反序列化到结构体更宽松，也更能反映阅读器的真实解析结果。
func epubIsValidXML(doc string) bool {
	dec := xml.NewDecoder(strings.NewReader(doc))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			return false
		}
	}
}

// epubPlainText 把 HTML 抽成纯文本，作为 XHTML 校验失败时的降级内容。
func epubPlainText(rawHTML string) string {
	if strings.TrimSpace(rawHTML) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			data := epubStripInvalidXMLChars(n.Data)
			if strings.TrimSpace(data) != "" {
				b.WriteString(data)
				b.WriteString(" ")
			}
			return
		case html.ElementNode:
			switch strings.ToLower(n.Data) {
			case "script", "style":
				return
			case "p", "div", "br", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "pre":
				b.WriteString(" ")
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return strings.Join(strings.Fields(b.String()), " ")
}
