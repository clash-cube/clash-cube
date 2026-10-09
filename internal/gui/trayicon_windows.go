package gui

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var darkTrayIcons = sync.OnceValue(func() map[string][]byte {
	out := map[string][]byte{}
	for _, icons := range windowsTrayIcons() {
		for _, data := range icons {
			source, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				continue
			}
			light := image.NewNRGBA(source.Bounds())
			for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
				for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
					_, _, _, a := source.At(x, y).RGBA()
					light.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: uint8(a >> 8)})
				}
			}
			var b bytes.Buffer
			if png.Encode(&b, light) == nil {
				out[string(data)] = b.Bytes()
			}
		}
	}
	return out
})

var windowsTrayIcons = sync.OnceValue(func() map[bool]map[string][]byte {
	icons := map[bool]map[string][]byte{}
	for on, data := range map[bool][]byte{true: trayIcon, false: trayIconOff} {
		icons[on] = map[string][]byte{}
		for _, letter := range []string{"", "R", "G", "D"} {
			icons[on][letter] = windowsTrayBadge(data, letter)
		}
	}
	return icons
})

func setWindowsTrayState(tray *application.SystemTray, on bool, letter string) {
	icon := windowsTrayIcons()[on][letter]
	if len(icon) == 0 {
		icon = windowsTrayIcons()[on][""]
	}
	setTrayIcon(tray, icon)
}

// Use the macOS badge geometry in the source icon's 22-unit coordinate space.
// The letter and surrounding gap are transparent, so both taskbar themes work.
func windowsTrayBadge(data []byte, letter string) []byte {
	glyphs := map[string][7]string{
		"R": {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
		"G": {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
		"D": {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	}
	glyph, ok := glyphs[letter]
	if !ok {
		return data
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	inside := func(x, y, left, top, size, radius float64) bool {
		dx := math.Max(math.Abs(x-left-size/2)-(size/2-radius), 0)
		dy := math.Max(math.Abs(y-top-size/2)-(size/2-radius), 0)
		return dx*dx+dy*dy <= radius*radius
	}
	for y := dst.Bounds().Min.Y; y < dst.Bounds().Max.Y; y++ {
		for x := dst.Bounds().Min.X; x < dst.Bounds().Max.X; x++ {
			px := (float64(x-dst.Bounds().Min.X) + .5) * 22 / float64(dst.Bounds().Dx())
			py := (float64(y-dst.Bounds().Min.Y) + .5) * 22 / float64(dst.Bounds().Dy())
			if !inside(px, py, 11.75, 11.5, 11, 3.25) {
				continue
			}
			c := color.NRGBA{}
			if inside(px, py, 13, 12.75, 8.5, 2.25) {
				c.A = 255
				gx, gy := int(math.Floor(px-14.75)), int(math.Floor(py-13.5))
				if gx >= 0 && gx < 5 && gy >= 0 && gy < 7 && glyph[gy][gx] == '1' {
					c.A = 0
				}
			}
			dst.SetNRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if png.Encode(&out, dst) != nil {
		return data
	}
	return out.Bytes()
}

// Tray updates run on the UI thread; traffic tooltip updates need no new icon.
var lastWindowsTrayIcon []byte

func setTrayIcon(tray *application.SystemTray, icon []byte) {
	if bytes.Equal(lastWindowsTrayIcon, icon) {
		return
	}
	lastWindowsTrayIcon = icon
	tray.SetIcon(icon)
	if dark := darkTrayIcons()[string(icon)]; len(dark) > 0 {
		tray.SetDarkModeIcon(dark)
	}
}
