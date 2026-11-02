package app

import (
	"context"
	"runtime"

	"github.com/egoist/mygo"

	"xin.dart.zrss/internal/storage"
)

// setMenu gives the app its menus: subscriptions in File, the views in View,
// and everything else in the roles the platforms expect. The same menus show
// in the title bar, so nothing hides with the system menu bar.
func (a *app) setMenu() {
	run := func(fn func()) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) {
			fn()
			a.redraw()
		}
	}
	name := mygo.App.Name()
	appItems := []*mygo.MenuItem{
		{Role: mygo.RoleAbout, Label: "关于" + name},
		{Label: "检查更新…", Click: run(a.checkUpdates)},
		mygo.Separator(),
		{Label: "设置…", Accelerator: "CmdOrCtrl+,", Click: run(a.openSettings)},
		mygo.Separator(),
	}
	if runtime.GOOS == "darwin" {
		appItems = append(appItems,
			&mygo.MenuItem{Role: mygo.RoleServices, Label: "服务"},
			mygo.Separator(),
			&mygo.MenuItem{Role: mygo.RoleHide, Label: "隐藏" + name},
			&mygo.MenuItem{Role: mygo.RoleHideOthers, Label: "隐藏其他"},
			&mygo.MenuItem{Role: mygo.RoleUnhide, Label: "全部显示"},
			mygo.Separator(),
		)
	}
	appItems = append(appItems, &mygo.MenuItem{Role: mygo.RoleQuit, Label: "退出" + name})

	fileItems := []*mygo.MenuItem{
		{Label: "添加订阅…", Accelerator: "CmdOrCtrl+N", Click: run(func() {
			a.showMain()
			a.newSubscription()
		})},
		mygo.Separator(),
		{Label: "刷新全部订阅", Accelerator: "CmdOrCtrl+R", Click: run(func() { a.refreshAll(true) })},
		{Label: "全部标为已读", Click: run(func() { a.markEverythingRead(context.Background()) })},
		mygo.Separator(),
		{Label: "导入 OPML…", Accelerator: "CmdOrCtrl+Shift+I", Click: run(a.importOPML)},
		{Label: "导出 OPML…", Accelerator: "CmdOrCtrl+Shift+E", Click: run(a.exportOPML)},
	}
	editItems := []*mygo.MenuItem{
		{Role: mygo.RoleUndo, Label: "撤销"},
		{Role: mygo.RoleRedo, Label: "重做"},
		mygo.Separator(),
		{Role: mygo.RoleCut, Label: "剪切"},
		{Role: mygo.RoleCopy, Label: "拷贝"},
		{Role: mygo.RolePaste, Label: "粘贴"},
		{Role: mygo.RoleSelectAll, Label: "全选"},
		mygo.Separator(),
		{Label: "复制文章链接", Click: run(a.copyCurrentLink)},
		{Label: "打开原网页", Accelerator: "CmdOrCtrl+O", Click: run(a.openOriginal)},
	}
	viewItems := []*mygo.MenuItem{
		{Label: "全部文章", Accelerator: "CmdOrCtrl+1", Click: run(func() {
			a.setScope(storage.Scope{Kind: storage.ScopeAll})
		})},
		{Label: "未读", Accelerator: "CmdOrCtrl+2", Click: run(func() {
			a.setScope(storage.Scope{Kind: storage.ScopeUnread})
		})},
		{Label: "星标", Accelerator: "CmdOrCtrl+3", Click: run(func() {
			a.setScope(storage.Scope{Kind: storage.ScopeStarred})
		})},
		{Label: "今天", Accelerator: "CmdOrCtrl+4", Click: run(func() {
			a.setScope(storage.Scope{Kind: storage.ScopeToday})
		})},
		mygo.Separator(),
		{Label: "搜索", Accelerator: "CmdOrCtrl+F", Click: run(func() {
			a.showMain()
			if a.win != nil {
				a.win.Update(func() { a.searchBox.Focus() })
			}
		})},
		mygo.Separator(),
		{Label: "增大字号", Accelerator: "CmdOrCtrl+=", Click: run(func() { a.adjustReaderSize(1) })},
		{Label: "减小字号", Accelerator: "CmdOrCtrl+-", Click: run(func() { a.adjustReaderSize(-1) })},
	}
	windowItems := []*mygo.MenuItem{
		{Role: mygo.RoleMinimize, Label: "最小化"},
		{Role: mygo.RoleZoom, Label: "缩放"},
		{Role: mygo.RoleClose, Label: "关闭"},
		mygo.Separator(),
		{Label: name, Accelerator: "CmdOrCtrl+0", Click: run(a.showMain)},
		{Role: mygo.RoleFront, Label: "前置全部窗口"},
	}

	// The menus the title bar opens, and the app menu that holds them.
	a.menuApp = mygo.NewMenu(appItems)
	a.menuFile = mygo.NewMenu(fileItems)
	a.menuEdit = mygo.NewMenu(editItems)
	a.menuView = mygo.NewMenu(viewItems)
	a.menuWindow = mygo.NewMenu(windowItems)

	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: name, Submenu: appItems},
		{Label: "文件", Submenu: fileItems},
		{Label: "编辑", Submenu: editItems},
		{Label: "显示", Submenu: viewItems},
		{Label: "窗口", Submenu: windowItems},
	}))
}
