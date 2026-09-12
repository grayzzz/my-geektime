package service

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"testing"
)

// 颜色允许 ±1 的误差：面积平均与白底合成都会涉及取整。
func near(a, b uint8) bool {
	d := int(a) - int(b)
	return d >= -1 && d <= 1
}

func TestEpubFlattenPixel(t *testing.T) {
	// 预乘 alpha 下，50% 透明度的纯红表示为 R=128, A=128。
	// 合成到白底后应是 R=255, G=127, B=127（即 50% 红 + 50% 白）。
	dst := make([]byte, 4)
	epubFlattenPixel(dst, []byte{128, 0, 0, 128})
	if !near(dst[0], 255) || !near(dst[1], 127) || !near(dst[2], 127) {
		t.Fatalf("半透明红合成白底结果错误: got %v want ~[255 127 127]", dst[:3])
	}
	if dst[3] != 0xFF {
		t.Fatalf("合成后必须不透明: got A=%d", dst[3])
	}

	// 全不透明像素应保持不变（恒等变换）——JPEG 路径的主要场景
	epubFlattenPixel(dst, []byte{10, 20, 30, 255})
	if dst[0] != 10 || dst[1] != 20 || dst[2] != 30 || dst[3] != 0xFF {
		t.Fatalf("不透明像素被改动了: got %v", dst)
	}

	// 全透明像素应变成纯白
	epubFlattenPixel(dst, []byte{0, 0, 0, 0})
	if dst[0] != 255 || dst[1] != 255 || dst[2] != 255 {
		t.Fatalf("全透明应合成成白色: got %v", dst[:3])
	}
}

func TestEpubScaleOpaqueBoxFilter(t *testing.T) {
	// 4x4：左两列纯黑、右两列纯白，缩到 2x2 后应保持左右分明（验证不是最近邻丢信息）
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if x < 2 {
				src.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				src.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	out := epubScaleOpaque(src, 2, 2)
	if out.Bounds().Dx() != 2 || out.Bounds().Dy() != 2 {
		t.Fatalf("尺寸错误: %v", out.Bounds())
	}
	if !near(out.RGBAAt(0, 0).R, 0) || !near(out.RGBAAt(1, 0).R, 255) {
		t.Fatalf("黑白分界处下采样错误: left=%d right=%d want 0/255",
			out.RGBAAt(0, 0).R, out.RGBAAt(1, 0).R)
	}
	if out.RGBAAt(0, 0).A != 255 || out.RGBAAt(1, 0).A != 255 {
		t.Fatalf("输出必须不透明")
	}

	// 4x4 棋盘缩到 2x2：每个目标像素平均 2 黑 2 白 → 约 127
	cb := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if (x+y)%2 == 0 {
				cb.SetRGBA(x, y, color.RGBA{0, 0, 0, 255})
			} else {
				cb.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	cbOut := epubScaleOpaque(cb, 2, 2)
	if !near(cbOut.RGBAAt(0, 0).R, 127) {
		t.Fatalf("棋盘面积平均应为 ~127, got %d", cbOut.RGBAAt(0, 0).R)
	}
}

func TestEpubScaleOpaqueNoResizeStillFlattens(t *testing.T) {
	// 尺寸不变时也必须走白底合成，否则带 alpha 的 PNG 转 JPEG 会变黑底
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.SetRGBA(0, 0, color.RGBA{0, 0, 0, 0}) // 全透明
	out := epubScaleOpaque(src, 2, 2)
	c := out.RGBAAt(0, 0)
	if !near(c.R, 255) || !near(c.G, 255) || !near(c.B, 255) {
		t.Fatalf("不缩放时未做白底合成: got %v", c)
	}
}

// noisyPNG 生成随机噪点 PNG——纯色图会被 PNG 压得极小，无法触发体积阈值。
func noisyPNG(t *testing.T, w, h int, withAlpha bool) []byte {
	t.Helper()
	rnd := rand.New(rand.NewSource(42))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if withAlpha {
				a = uint8(rnd.Intn(256))
			}
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(rnd.Intn(256)), G: uint8(rnd.Intn(256)),
				B: uint8(rnd.Intn(256)), A: a,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成测试 PNG 失败: %v", err)
	}
	return buf.Bytes()
}

func TestEpubShrinkImage(t *testing.T) {
	t.Run("非位图格式一律放行", func(t *testing.T) {
		big := noisyPNG(t, 2000, 1400, false)
		for _, mt := range []string{"image/gif", "image/svg+xml", "image/webp", "image/bmp"} {
			if out, _ := epubShrinkImage(big, mt); out != nil {
				t.Fatalf("%s 不应被处理", mt)
			}
		}
	})

	t.Run("小图不处理", func(t *testing.T) {
		small := noisyPNG(t, 64, 64, false)
		if len(small) >= epubImageShrinkMinBytes {
			t.Skipf("测试用图未达到阈值(%d)，跳过", epubImageShrinkMinBytes)
		}
		if out, _ := epubShrinkImage(small, "image/png"); out != nil {
			t.Fatal("低于阈值的图不应被重新编码")
		}
	})

	t.Run("损坏数据不 panic 且放行", func(t *testing.T) {
		junk := bytes.Repeat([]byte{0xAB}, epubImageShrinkMinBytes+1024)
		if out, _ := epubShrinkImage(junk, "image/png"); out != nil {
			t.Fatal("无法解码时应返回 nil")
		}
	})

	t.Run("高清 PNG 被缩放并转 JPEG", func(t *testing.T) {
		src := noisyPNG(t, 2400, 1600, false)
		if len(src) < epubImageShrinkMinBytes {
			t.Fatalf("测试图太小(%d)，无法验证压缩", len(src))
		}
		out, mt := epubShrinkImage(src, "image/png")
		if out == nil {
			t.Fatal("应发生压缩，实际返回 nil")
		}
		if mt != "image/jpeg" {
			t.Fatalf("media type 应为 image/jpeg, got %s", mt)
		}
		if len(out) >= len(src) {
			t.Fatalf("压缩后反而变大: %d -> %d", len(src), len(out))
		}
		decoded, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("产出的 JPEG 无法解码: %v", err)
		}
		if got := decoded.Bounds().Dx(); got != epubImageMaxWidth {
			t.Fatalf("宽度应被限制为 %d, got %d", epubImageMaxWidth, got)
		}
		if got := decoded.Bounds().Dy(); got != 1600*epubImageMaxWidth/2400 {
			t.Fatalf("高度比例不对, got %d", got)
		}
		t.Logf("2400x1600 PNG %d 字节 -> %d 字节 JPEG (%.1f%%)",
			len(src), len(out), float64(len(out))*100/float64(len(src)))
	})

	t.Run("带透明通道的 PNG 合成白底后不出现黑底", func(t *testing.T) {
		src := noisyPNG(t, 1600, 1200, true)
		out, mt := epubShrinkImage(src, "image/png")
		if out == nil {
			t.Fatal("应发生压缩")
		}
		if mt != "image/jpeg" {
			t.Fatalf("media type 应为 image/jpeg, got %s", mt)
		}
		decoded, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("产出的 JPEG 无法解码: %v", err)
		}
		// JPEG 本身无 alpha；这里主要确认尺寸收敛且能解码，黑底问题由 flatten 单测覆盖
		if decoded.Bounds().Dx() > epubImageMaxWidth {
			t.Fatalf("宽度未被限制: %d", decoded.Bounds().Dx())
		}
	})

	t.Run("窄图不放大", func(t *testing.T) {
		src := noisyPNG(t, 800, 600, false)
		out, _ := epubShrinkImage(src, "image/png")
		if out == nil {
			return // 没压小也算合法结果
		}
		decoded, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("无法解码: %v", err)
		}
		if decoded.Bounds().Dx() != 800 {
			t.Fatalf("比上限窄的图不应被放大: got %d", decoded.Bounds().Dx())
		}
	})
}

func TestEpubShrinkImageGIFPassThrough(t *testing.T) {
	// gif 静态图也不该被转 JPEG——会丢掉动图能力，且不在支持列表内
	pal := color.Palette{color.Black, color.White}
	img := image.NewPaletted(image.Rect(0, 0, 4, 4), pal)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("生成 GIF 失败: %v", err)
	}
	if out, _ := epubShrinkImage(buf.Bytes(), "image/gif"); out != nil {
		t.Fatal("GIF 不应被转换")
	}
}
