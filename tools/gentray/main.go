// Command gentray draws the app's artwork: the 1024x1024 icon that `mygo
// build` packs for every platform, and the tray icons embedded by the app: a
// colored RSS mark for the notification areas of Windows and Linux, and a
// black one on transparent for the macOS menu bar to tint.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	xdraw "golang.org/x/image/draw"
)

const scale = 4

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	trayDir := filepath.Join(root, "internal", "app", "tray")
	if err := os.MkdirAll(trayDir, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := writePNG(filepath.Join(trayDir, "tray.png"), colorIcon(64)); err != nil {
		log.Fatal(err)
	}
	if err := writePNG(filepath.Join(trayDir, "tray-template.png"), templateIcon(44)); err != nil {
		log.Fatal(err)
	}
	resourcesDir := filepath.Join(root, "resources")
	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := writePNG(filepath.Join(resourcesDir, "icon.png"), colorIcon(1024)); err != nil {
		log.Fatal(err)
	}
}

// colorIcon is a rounded square in the app's red with a white RSS mark.
func colorIcon(size int) *image.RGBA {
	big := image.NewRGBA(image.Rect(0, 0, size*scale, size*scale))
	accent := color.NRGBA{R: 0xE0, G: 0x3E, B: 0x48, A: 0xFF}
	roundedSquare(big, accent, float64(size*scale)*0.22)

	white := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	drawRSS(big, white, float64(size*scale)*0.18)
	return downsample(big, size)
}

// templateIcon is the RSS mark alone, black on transparent, for macOS.
func templateIcon(size int) *image.RGBA {
	big := image.NewRGBA(image.Rect(0, 0, size*scale, size*scale))
	black := color.NRGBA{A: 0xFF}
	drawRSS(big, black, float64(size*scale)*0.12)
	return downsample(big, size)
}

func roundedSquare(dst *image.RGBA, c color.NRGBA, radius float64) {
	w := dst.Bounds().Dx()
	for y := 0; y < w; y++ {
		for x := 0; x < w; x++ {
			if insideRounded(float64(x)+0.5, float64(y)+0.5, float64(w), radius) {
				dst.Set(x, y, c)
			}
		}
	}
}

func insideRounded(x, y, size, radius float64) bool {
	if x < 0 || y < 0 || x > size || y > size {
		return false
	}
	if x >= radius && x <= size-radius || y >= radius && y <= size-radius {
		return true
	}
	cx := math.Min(math.Max(x, radius), size-radius)
	cy := math.Min(math.Max(y, radius), size-radius)
	dx, dy := x-cx, y-cy
	return dx*dx+dy*dy <= radius*radius
}

// drawRSS draws the dot and two arcs of the feed mark, centered on a point
// inset from the bottom-left corner.
func drawRSS(dst *image.RGBA, c color.NRGBA, margin float64) {
	w := float64(dst.Bounds().Dx())
	cx, cy := margin, w-margin
	for y := 0; y < int(w); y++ {
		for x := 0; x < int(w); x++ {
			dx := float64(x) + 0.5 - cx
			dy := cy - (float64(y) + 0.5)
			r := math.Hypot(dx, dy)
			angle := math.Atan2(dy, dx)
			dot := r <= w*0.085
			arc1 := r >= w*0.29 && r <= w*0.42 && angle >= 0 && angle <= math.Pi/2
			arc2 := r >= w*0.57 && r <= w*0.70 && angle >= 0 && angle <= math.Pi/2
			if (dot || arc1 || arc2) && dx >= -0.6 && dy >= -0.6 {
				dst.Set(x, y, c)
			}
		}
	}
}

func downsample(src *image.RGBA, size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		return err
	}
	return f.Close()
}
