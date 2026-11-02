package app

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// palette holds the colors of the three columns, for light and dark.
type palette struct {
	dark     bool
	bg       ui.Color // the reader
	side     ui.Color // the sidebar
	list     ui.Color // the article list
	text     ui.Color
	muted    ui.Color
	faint    ui.Color
	hairline ui.Color
	hover    ui.Color
	press    ui.Color
	accent   ui.Color
	onAccent ui.Color
	shade    ui.Color
}

func paletteFor(t *ui.Theme, accent string) palette {
	accentC, ok := accentColor(accent, t.Dark)
	if !ok {
		accentC, _ = accentColor("red", t.Dark)
	}
	if t.Dark {
		return palette{
			dark:     true,
			bg:       ui.Hex("#131315"),
			side:     ui.Hex("#1a1a1d"),
			list:     ui.Hex("#17171a"),
			text:     ui.Hex("#f5f5f7"),
			muted:    ui.RGBA(235, 235, 245, 0.58),
			faint:    ui.RGBA(235, 235, 245, 0.30),
			hairline: ui.RGBA(255, 255, 255, 0.08),
			hover:    ui.RGBA(255, 255, 255, 0.06),
			press:    ui.RGBA(255, 255, 255, 0.10),
			accent:   accentC,
			onAccent: ui.Hex("#ffffff"),
			shade:    ui.RGBA(0, 0, 0, 0.5),
		}
	}
	return palette{
		bg:       ui.Hex("#ffffff"),
		side:     ui.Hex("#f5f5f7"),
		list:     ui.Hex("#fbfbfc"),
		text:     ui.Hex("#1c1c1e"),
		muted:    ui.RGBA(60, 60, 67, 0.62),
		faint:    ui.RGBA(60, 60, 67, 0.32),
		hairline: ui.RGBA(0, 0, 0, 0.08),
		hover:    ui.RGBA(0, 0, 0, 0.045),
		press:    ui.RGBA(0, 0, 0, 0.09),
		accent:   accentC,
		onAccent: ui.Hex("#ffffff"),
		shade:    ui.RGBA(0, 0, 0, 0.18),
	}
}

func strokeIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

func fillIcon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor">` + paths + `</svg>`))
}

var (
	iconRSS         = strokeIcon(`<path d="M4 11a9 9 0 0 1 9 9"/><path d="M4 4a16 16 0 0 1 16 16"/><circle cx="5" cy="19" r="1.5" fill="currentColor" stroke="none"/>`)
	iconInbox       = strokeIcon(`<path d="M22 12h-6l-2 3h-4l-2-3H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>`)
	iconDot         = fillIcon(`<circle cx="12" cy="12" r="4.5"/>`)
	iconStar        = strokeIcon(`<path d="m12 2.8 2.9 5.9 6.5.9-4.7 4.6 1.1 6.5-5.8-3-5.8 3 1.1-6.5-4.7-4.6 6.5-.9z"/>`)
	iconStarFill    = fillIcon(`<path d="m12 2.8 2.9 5.9 6.5.9-4.7 4.6 1.1 6.5-5.8-3-5.8 3 1.1-6.5-4.7-4.6 6.5-.9z"/>`)
	iconClock       = strokeIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`)
	iconSearch      = strokeIcon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)
	iconPlus        = strokeIcon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	iconSettings    = strokeIcon(`<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>`)
	iconRefresh     = strokeIcon(`<path d="M20 7v5h-5"/><path d="M20 12a8 8 0 1 0-2.3 5.7"/>`)
	iconCheck       = strokeIcon(`<path d="m5 12 4 4L19 6"/>`)
	iconCheckAll    = strokeIcon(`<path d="m2 13 4 4L16 7"/><path d="m11 15 2 2L22 7"/>`)
	iconMail        = strokeIcon(`<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m3 7 9 6 9-6"/>`)
	iconMailOpen    = strokeIcon(`<path d="m3 9 9-6 9 6v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><path d="m3 9 9 6 9-6"/>`)
	iconExternal    = strokeIcon(`<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5"/>`)
	iconCopy        = strokeIcon(`<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
	iconTrash       = strokeIcon(`<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
	iconFolder      = strokeIcon(`<path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.5l-2-2H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2z"/>`)
	iconChevron     = strokeIcon(`<path d="m9 18 6-6-6-6"/>`)
	iconChevronLeft = strokeIcon(`<path d="m15 18-6-6 6-6"/>`)
	iconDown        = strokeIcon(`<path d="m6 9 6 6 6-6"/>`)
	iconX           = strokeIcon(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconMinimize    = strokeIcon(`<path d="M5 12h14"/>`)
	iconMaximize    = strokeIcon(`<rect x="5" y="5" width="14" height="14" rx="1.5"/>`)
	iconRestore     = strokeIcon(`<rect x="8" y="8" width="11" height="11" rx="1.5"/><path d="M5 16V7a2 2 0 0 1 2-2h9"/>`)
	iconMore        = fillIcon(`<circle cx="5" cy="12" r="1.8"/><circle cx="12" cy="12" r="1.8"/><circle cx="19" cy="12" r="1.8"/>`)
	iconRename      = strokeIcon(`<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>`)
	iconAlert       = strokeIcon(`<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4"/><path d="M12 17h.01"/>`)
	iconImport      = strokeIcon(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/>`)
	iconExport      = strokeIcon(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 8 5-5 5 5"/><path d="M12 3v12"/>`)
)

// iconButton is a borderless button showing an icon, with a soft circle under
// the pointer and a little give as it is pressed.
func iconButton(c *ui.Context, icon *ui.SVG, size float32, color ui.Color, label string) ui.Element {
	b := ui.ButtonBase(c).Size(size+16, size+16).Center().Radius(999).Label(label).TextColor(color)
	bg := ui.Transparent
	if b.Hovered() {
		bg = ui.RGBA(128, 128, 128, 0.14)
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.88
	}
	b.AnimatePose(pose, ui.Snappy)
	b.FocusRing(false)
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(size, size)
		}
	})
	return b
}

// pillButton is a rounded button with an icon and a label.
func pillButton(c *ui.Context, icon *ui.SVG, label string, bg, fg ui.Color) ui.Element {
	b := ui.ButtonBase(c).Height(34).PaddingX(16).Gap(7).Radius(999).AlignItems(ui.Center).TextColor(fg).Label(label)
	fill := bg
	if b.Hovered() {
		fill = bg.Mix(ui.RGB(255, 255, 255), 0.12)
	}
	b.Background(b.AnimateColor("bg", fill, ui.Animation{Duration: 120 * time.Millisecond}))
	pose := ui.Pose{}
	if b.Pressed() {
		pose.Scale = 0.96
	}
	b.AnimatePose(pose, ui.Snappy)
	b.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14)
		}
		ui.Text(c, label).FontSize(13).FontWeight(600).SingleLine()
	})
	return b
}

// countBadge is a small number, muted when the row is not chosen.
func countBadge(c *ui.Context, n int, color ui.Color) {
	if n <= 0 {
		return
	}
	text := fmt.Sprint(n)
	if n > 999 {
		text = "999+"
	}
	ui.Text(c, text).FontSize(11).FontWeight(600).FontFeatures("tnum").TextColor(color)
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 48*time.Hour:
		return "昨天"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	default:
		return t.Local().Format("2006-01-02")
	}
}

func fullDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.Local()
	if t.Year() == time.Now().Year() {
		return t.Format("1月2日 15:04")
	}
	return t.Format("2006年1月2日 15:04")
}

func readMinutes(plain string) int {
	n := utf8.RuneCountInString(plain)
	if n == 0 {
		return 0
	}
	m := (n + 349) / 350
	return max(1, m)
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Host, "www.")
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n]), " ") + "…"
}
