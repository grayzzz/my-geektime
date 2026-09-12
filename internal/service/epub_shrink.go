package service

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png" // 注册 PNG 解码器
)

// EPUB 图片瘦身。
//
// 背景：极客时间的正文图片是原始高清截图。实测「晓寒 · 企业级多智能体设计实战」
// （43 讲）内嵌图片合计 493MB，单图最大 4.1MB、175 张 ≥1MB，整本 epub 近 500MB，
// 多数阅读器和手机无法打开。极客 CDN 不支持服务端缩放
// （?x-oss-process=image/resize,w_800 / ?imageView2/1/w/800 / ?wh=1920x1080 实测体积均无变化），
// 只能本地解码后重新编码。
//
// 铁律：**只压缩内嵌进 epub 的那一份，绝不回写共享缓存**。
// 共享缓存 key 是 {cache_prefix}/md5(url)，与 /v2/file/proxy 指向同一份文件；
// 一旦把压缩结果写回去，网页端显示的原图也会变成缩略图，属于破坏既有功能。
// 所以本文件只处理内存字节，不接触 global.Storage —— 缓存写入仍由 epub_image.go
// 用原始字节完成。

const (
	// epubImageMaxWidth 内嵌图片的最大宽度（px）。
	// 正文图片实际显示宽度约 720 CSS px，高清屏按 2 倍算即 1440；
	// 原始截图宽约 1650px，取 1440 相当于"满血 2 倍图"，几乎不牺牲清晰度。
	epubImageMaxWidth = 1440
	// epubImageJPEGQuality JPEG 质量。截图以文字为主，质量过低会在文字边缘出现彩边，取 88。
	epubImageJPEGQuality = 88
	// epubImageShrinkMinBytes 低于该体积不做处理——小图重新编码可能反而变大，白耗 CPU。
	epubImageShrinkMinBytes = 150 << 10
)

// epubShrinkImage 把图片缩到适合电子书的尺寸并转成 JPEG。
//
// 返回 (nil, "") 表示"不处理，请原样内嵌"，出现于以下情况：
//   - 格式不适合：gif 要保留动画、svg 是矢量、webp/bmp 需要 golang.org/x/image 才能解码
//   - 体积本来就小，没有压缩价值
//   - 解码失败（损坏或非常规编码）
//   - 压完没有比原图更小（宁可保留原图，也不做负优化）
func epubShrinkImage(data []byte, mediaType string) ([]byte, string) {
	switch mediaType {
	case "image/jpeg", "image/png":
	default:
		return nil, ""
	}
	if len(data) < epubImageShrinkMinBytes {
		return nil, ""
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ""
	}
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, ""
	}
	// 等比缩放到 epubImageMaxWidth 以内；本来就不宽的不放大
	dstW, dstH := srcW, srcH
	if dstW > epubImageMaxWidth {
		dstH = dstH * epubImageMaxWidth / dstW
		dstW = epubImageMaxWidth
		if dstH < 1 {
			dstH = 1
		}
	}
	out := epubScaleOpaque(epubToPremultipliedRGBA(src), dstW, dstH)
	var buf bytes.Buffer
	// 目标就是比原图小，预分配原图大小略保守，直接用原图长度起步即可
	buf.Grow(len(data))
	if err = jpeg.Encode(&buf, out, &jpeg.Options{Quality: epubImageJPEGQuality}); err != nil {
		return nil, ""
	}
	if buf.Len() >= len(data) {
		return nil, ""
	}
	return buf.Bytes(), "image/jpeg"
}

// epubToPremultipliedRGBA 把任意来源图统一转成预乘 alpha 的 *image.RGBA。
//
// 必须统一：image.RGBA 的 Pix 是**预乘** alpha，而 *image.NRGBA 是**非预乘**，
// 直接按字节读后者会把半透明像素算错。draw.Draw 能正确处理各种来源
// （YCbCr 的快速转换、Paletted 的查表等），只在类型已是 *image.RGBA 时才复用。
func epubToPremultipliedRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	sb := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, sb.Dx(), sb.Dy()))
	draw.Draw(dst, dst.Bounds(), src, sb.Min, draw.Src)
	return dst
}

// epubScaleOpaque 把预乘 RGBA 缩放到 dstW×dstH，并合成到白底后返回不透明 RGBA。
//
// 下采样用面积平均（box filter）：对预乘值求平均在数学上正是带 alpha 的正确下采样，
// 且能避免最近邻在缩小时丢失细线、让文字发毛。
// 缩放后统一合成白底，是因为 JPEG 不支持 alpha；对本来就全不透明的图这步是恒等变换。
func epubScaleOpaque(src *image.RGBA, dstW, dstH int) *image.RGBA {
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	if dstW == srcW && dstH == srcH {
		for y := 0; y < srcH; y++ {
			so, do := y*src.Stride, y*dst.Stride
			for x := 0; x < srcW; x++ {
				epubFlattenPixel(dst.Pix[do:do+4], src.Pix[so:so+4])
				so, do = so+4, do+4
			}
		}
		return dst
	}

	xRatio := float64(srcW) / float64(dstW)
	yRatio := float64(srcH) / float64(dstH)
	for dy := 0; dy < dstH; dy++ {
		y0 := int(float64(dy) * yRatio)
		y1 := int(float64(dy+1)*yRatio + 0.5)
		if y1 <= y0 {
			y1 = y0 + 1
		}
		if y1 > srcH {
			y1 = srcH
		}
		do := dy * dst.Stride
		for dx := 0; dx < dstW; dx++ {
			x0 := int(float64(dx) * xRatio)
			x1 := int(float64(dx+1)*xRatio + 0.5)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if x1 > srcW {
				x1 = srcW
			}
			var sr, sg, sb2, sa, n uint64
			for y := y0; y < y1; y++ {
				so := y*src.Stride + x0*4
				for x := x0; x < x1; x++ {
					sr += uint64(src.Pix[so])
					sg += uint64(src.Pix[so+1])
					sb2 += uint64(src.Pix[so+2])
					sa += uint64(src.Pix[so+3])
					so += 4
				}
				n += uint64(x1 - x0)
			}
			if n == 0 {
				do += 4
				continue
			}
			var px [4]byte
			px[0], px[1], px[2], px[3] = byte(sr/n), byte(sg/n), byte(sb2/n), byte(sa/n)
			epubFlattenPixel(dst.Pix[do:do+4], px[:])
			do += 4
		}
	}
	return dst
}

// epubFlattenPixel 把预乘 alpha 像素合成到白底，写出不透明像素。
//
// 合成公式（0-255 定点）：C_out = C_pre + 255 - A。
// 预乘满足 C_pre ≤ A，因此结果不会溢出 uint8；A=255 时是恒等变换。
func epubFlattenPixel(dst, src []byte) {
	a := src[3]
	inv := 255 - a
	dst[0] = src[0] + inv
	dst[1] = src[1] + inv
	dst[2] = src[2] + inv
	dst[3] = 0xFF
}
