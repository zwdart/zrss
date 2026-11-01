package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// addDialog asks for the address of a subscription and, optionally, the
// folder it belongs in.
type addDialog struct {
	open     bool
	url      string
	folderID int64
	err      string
}

// promptDialog asks for one line of text, for a folder or feed name.
type promptDialog struct {
	open   bool
	title  string
	label  string
	value  string
	hint   string
	action func(string)
}

// confirmDialog asks before something is removed.
type confirmDialog struct {
	open    bool
	title   string
	message string
	action  string
	run     func()
}

func (a *app) addDialogView(c *ui.Context) {
	p := a.pal
	d := &a.add
	ui.DialogBase(c.Key("add-feed-dialog"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.34))
		panel.Width(430).Padding(24).Radius(18).Gap(12).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, "添加订阅").FontSize(17).FontWeight(700)
		ui.Text(c, "输入网站地址或 RSS / Atom 地址，zrss 会自己找到订阅源。").FontSize(12.5).TextColor(p.muted).LineHeight(1.45)
		in := ui.TextInput(c.Key("add-url"), &d.url).Placeholder("https://example.com").
			Label("订阅地址").FontSize(14).AutoFocus()
		if in.Submitted() {
			a.submitAdd()
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "文件夹").FontSize(12.5).TextColor(p.muted).Width(54)
			labels := make([]string, 0, len(a.folders))
			ids := make([]int64, 0, len(a.folders))
			chosen := ""
			for _, opt := range a.folderOptions() {
				labels = append(labels, opt.Path)
				ids = append(ids, opt.ID)
				if opt.ID == d.folderID {
					chosen = opt.Path
				}
			}
			if chosen == "" && len(labels) > 0 {
				chosen = labels[0]
				d.folderID = ids[0]
			}
			if len(labels) > 0 {
				if ui.Select(c.Key("add-folder"), &chosen, labels).Label("文件夹").Grow(1).Changed() {
					for i, label := range labels {
						if label == chosen {
							d.folderID = ids[i]
							break
						}
					}
				}
			}
		})
		if d.err != "" {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconAlert).Size(13, 13).TextColor(ui.Hex("#ef4444"))
				ui.Text(c, d.err).FontSize(12).TextColor(ui.Hex("#ef4444"))
			})
		}
		ui.Row(c).Gap(10).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			if pillButton(c.Key("add-cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			if pillButton(c.Key("add-ok"), nil, "添加", p.accent, p.onAccent).Clicked() {
				a.submitAdd()
			}
		})
	})
}

func (a *app) submitAdd() {
	if strings.TrimSpace(a.add.url) == "" {
		a.add.err = "请输入网站或订阅地址"
		return
	}
	a.subscribe(a.add.url)
}

func (a *app) promptDialogView(c *ui.Context) {
	p := a.pal
	d := &a.prompt
	ui.DialogBase(c.Key("prompt-dialog"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.34))
		panel.Width(376).Padding(24).Radius(18).Gap(12).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, d.title).FontSize(17).FontWeight(700)
		if d.hint != "" {
			ui.Text(c, d.hint).FontSize(12).TextColor(p.muted).LineHeight(1.4)
		}
		in := ui.TextInput(c.Key("prompt-value"), &d.value).Placeholder(d.label).
			Label(d.label).FontSize(14).AutoFocus()
		if in.Submitted() {
			a.submitPrompt()
		}
		ui.Row(c).Gap(10).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
			if pillButton(c.Key("prompt-cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			ok := pillButton(c.Key("prompt-ok"), nil, "保存", p.accent, p.onAccent)
			if strings.TrimSpace(d.value) == "" {
				ok.Disabled(true).Opacity(0.5)
			}
			if ok.Clicked() {
				a.submitPrompt()
			}
		})
	})
}

func (a *app) submitPrompt() {
	d := &a.prompt
	value := strings.TrimSpace(d.value)
	if value == "" {
		return
	}
	run := d.action
	d.open = false
	d.action = nil
	if run != nil {
		run(value)
	}
}

func (a *app) confirmDialogView(c *ui.Context) {
	p := a.pal
	d := &a.confirm
	ui.DialogBase(c.Key("confirm-dialog"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.34))
		panel.Width(388).Padding(24).Radius(18).Gap(10).
			Background(p.bg).Border(1, p.hairline).Shadow(0, 24, 60, -12, ui.RGBA(0, 0, 0, 0.4)).
			Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 0.94}, Exit: &ui.Motion{Scale: 0.96}})
		ui.Text(c, d.title).FontSize(17).FontWeight(700)
		ui.Text(c, d.message).FontSize(13).TextColor(p.muted).LineHeight(1.5)
		ui.Row(c).Gap(10).Justify(ui.End).Margin(10, 0, 0, 0).Children(func() {
			if pillButton(c.Key("confirm-cancel"), nil, "取消", p.hover.Over(p.bg), p.text).Clicked() {
				d.open = false
			}
			label := d.action
			if label == "" {
				label = "确定"
			}
			if pillButton(c.Key("confirm-ok"), nil, label, ui.Hex("#dc2626"), ui.Hex("#ffffff")).Clicked() {
				run := d.run
				d.open = false
				d.run = nil
				if run != nil {
					run()
				}
			}
		})
	})
}
