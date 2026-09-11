package main

// generate_icons.go — 零外部依赖的 Cyrene Gateway 品牌资产多尺寸栅格化生成器。
//
// 色板与几何与 webui/public/icon.svg 及 webui/src/components/ui/CyreneLogo.tsx 保持 100% 同步：
// 品牌渐变: #2dd4bf (45, 212, 191) -> #06b6d4 (6, 182, 212) -> #6366f1 (99, 102, 241) -> #8b5cf6 (139, 92, 246)
// 倒角反光: #ffffff (纯白微光带)
// 路由晶核: 60° 等距菱形 (中心白色 -> 天青微光)
//
// 运行方式:
//   go run webui/scripts/generate_icons.go

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

type Point struct {
	X, Y float64
}

func inPolygon(p Point, poly []Point) bool {
	inside := false
	n := len(poly)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		pi := poly[i]
		pj := poly[j]
		if ((pi.Y > p.Y) != (pj.Y > p.Y)) &&
			(p.X < (pj.X-pi.X)*(p.Y-pi.Y)/(pj.Y-pi.Y)+pi.X) {
			inside = !inside
		}
	}
	return inside
}

func distToSegment(p, a, b Point) float64 {
	dx := b.X - a.X
	dy := b.Y - a.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(p.X-a.X, p.Y-a.Y)
	}
	t := ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / l2
	t = math.Max(0, math.Min(1, t))
	proj := Point{X: a.X + t*dx, Y: a.Y + t*dy}
	return math.Hypot(p.X-proj.X, p.Y-proj.Y)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func clamp(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

func liquidColor(t float64) (r, g, b float64) {
	t = max(0, min(1, t))
	if t <= 0.35 {
		f := t / 0.35
		return 45*(1-f) + 6*f, 212*(1-f) + 182*f, 191*(1-f) + 212*f
	} else if t <= 0.70 {
		f := (t - 0.35) / 0.35
		return 6*(1-f) + 99*f, 182*(1-f) + 102*f, 212*(1-f) + 241*f
	} else {
		f := (t - 0.70) / 0.30
		return 99*(1-f) + 139*f, 102*(1-f) + 92*f, 241*(1-f) + 246*f
	}
}

// renderIcon 采用 4x4 (16倍) 超采样抗锯齿光栅化器
func renderIcon(dim int, includeRhombus bool) *image.RGBA {
	scale := float64(dim) / 32.0

	rawC := []Point{
		{27.69, 9.25}, {16, 2.5}, {4.31, 9.25}, {4.31, 22.75}, {16, 29.5},
		{27.69, 22.75}, {22.5, 19.75}, {16, 23.5}, {9.5, 19.75}, {9.5, 12.25},
		{16, 8.5}, {22.5, 12.25},
	}
	polyC := make([]Point, len(rawC))
	for i, pt := range rawC {
		polyC[i] = Point{X: pt.X * scale, Y: pt.Y * scale}
	}

	rawRhombus := []Point{
		{16, 13.5}, {18.17, 16.0}, {16, 18.5}, {13.83, 16.0},
	}
	polyRhombus := make([]Point, len(rawRhombus))
	for i, pt := range rawRhombus {
		polyRhombus[i] = Point{X: pt.X * scale, Y: pt.Y * scale}
	}

	chamferPtA := polyC[2]
	chamferPtB := polyC[1]
	chamferPtC := polyC[0]
	chamferWidth := 0.85 * scale

	img := image.NewRGBA(image.Rect(0, 0, dim, dim))

	ss := 4
	subStep := 1.0 / float64(ss)

	for py := 0; py < dim; py++ {
		for px := 0; px < dim; px++ {
			var accR, accG, accB, accA float64

			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					sample := Point{
						X: float64(px) + (float64(sx)+0.5)*subStep,
						Y: float64(py) + (float64(sy)+0.5)*subStep,
					}

					if includeRhombus && inPolygon(sample, polyRhombus) {
						ct := (sample.Y - polyRhombus[0].Y) / (polyRhombus[2].Y - polyRhombus[0].Y)
						ct = max(0, min(1, ct))
						cr := 255.0*(1-ct) + 56.0*ct
						cg := 255.0*(1-ct) + 189.0*ct
						cb := 255.0*(1-ct) + 248.0*ct
						accR += cr
						accG += cg
						accB += cb
						accA += 255.0
						continue
					}

					if inPolygon(sample, polyC) {
						gt := (sample.X/float64(dim))*0.5 + (sample.Y/float64(dim))*0.5
						pr, pg, pb := liquidColor(gt)

						d1 := distToSegment(sample, chamferPtA, chamferPtB)
						d2 := distToSegment(sample, chamferPtB, chamferPtC)
						minD := min(d1, d2)
						if minD < chamferWidth {
							f := 1.0 - (minD / chamferWidth)
							specAlpha := f * 0.90
							pr = pr*(1-specAlpha) + 255.0*specAlpha
							pg = pg*(1-specAlpha) + 255.0*specAlpha
							pb = pb*(1-specAlpha) + 255.0*specAlpha
						}

						accR += pr
						accG += pg
						accB += pb
						accA += 255.0
					}
				}
			}

			totalSamples := float64(ss * ss)
			alpha := accA / totalSamples
			if alpha > 0 {
				r := accR / totalSamples
				g := accG / totalSamples
				b := accB / totalSamples
				img.SetRGBA(px, py, color.RGBA{
					R: clamp(r * (alpha / 255.0)),
					G: clamp(g * (alpha / 255.0)),
					B: clamp(b * (alpha / 255.0)),
					A: clamp(alpha),
				})
			}
		}
	}

	return img
}

func createIco(pngs [][]byte) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(len(pngs)))

	offset := uint32(6 + 16*len(pngs))

	for _, p := range pngs {
		cfg, _, _ := image.DecodeConfig(bytes.NewReader(p))
		w := uint8(cfg.Width)
		if cfg.Width >= 256 {
			w = 0
		}
		h := uint8(cfg.Height)
		if cfg.Height >= 256 {
			h = 0
		}

		binary.Write(&buf, binary.LittleEndian, w)
		binary.Write(&buf, binary.LittleEndian, h)
		binary.Write(&buf, binary.LittleEndian, uint8(0))
		binary.Write(&buf, binary.LittleEndian, uint8(0))
		binary.Write(&buf, binary.LittleEndian, uint16(1))
		binary.Write(&buf, binary.LittleEndian, uint16(32))
		binary.Write(&buf, binary.LittleEndian, uint32(len(p)))
		binary.Write(&buf, binary.LittleEndian, offset)
		offset += uint32(len(p))
	}

	for _, p := range pngs {
		buf.Write(p)
	}

	return buf.Bytes()
}

func main() {
	outDir := "webui/public"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	fmt.Println("Generating Cyrene Gateway brand assets into:", outDir)

	// 1. icon.png (512x512) & icon-512.png
	img512 := renderIcon(512, true)
	var b512 bytes.Buffer
	png.Encode(&b512, img512)
	os.WriteFile(filepath.Join(outDir, "icon.png"), b512.Bytes(), 0644)
	os.WriteFile(filepath.Join(outDir, "icon-512.png"), b512.Bytes(), 0644)
	fmt.Println(" -> icon.png & icon-512.png [512x512 ok]")

	// 2. icon-192.png (192x192)
	img192 := renderIcon(192, true)
	var b192 bytes.Buffer
	png.Encode(&b192, img192)
	os.WriteFile(filepath.Join(outDir, "icon-192.png"), b192.Bytes(), 0644)
	fmt.Println(" -> icon-192.png [192x192 ok]")

	// 3. Multi-resolution favicon.ico (16, 32, 48)
	img48 := renderIcon(48, true)
	var b48 bytes.Buffer
	png.Encode(&b48, img48)

	img32 := renderIcon(32, false)
	var b32 bytes.Buffer
	png.Encode(&b32, img32)

	img16 := renderIcon(16, false)
	var b16 bytes.Buffer
	png.Encode(&b16, img16)

	icoData := createIco([][]byte{b16.Bytes(), b32.Bytes(), b48.Bytes()})
	os.WriteFile(filepath.Join(outDir, "favicon.ico"), icoData, 0644)
	fmt.Println(" -> favicon.ico [16, 32, 48 multi-res ok]")
}
