//go:build desktop && (darwin || windows)

package pacaroute

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/LandoWang/pacaroute/internal/pacaroute/atomicfile"
	"github.com/getlantern/systray"
)

//go:embed trayassets/mark.png
var trayPNG []byte

//go:embed trayassets/mark.ico
var trayICO []byte

const desktopEnabled = true

func runDesktop(ctx context.Context, a arguments, s *ConfigStore, auth *Auth, port int, emit func(any), fail func(error) int) int {
	port = desktopPort(a, s.Root, port)
	appUnlock, e := atomicfile.Lock(ctx, filepath.Join(s.Root, "app.lock"), 200*time.Millisecond)
	if e != nil {
		_ = openManagement(s.Root, readDesktopSettings(s.Root).AdminPort)
		return fail(errors.New("a desktop application already owns this data directory"))
	}
	defer appUnlock()
	serviceUnlock, e := atomicfile.Lock(ctx, filepath.Join(s.Root, "pacaroute-service.lock"), 200*time.Millisecond)
	if e != nil {
		return fail(errors.New("a service already owns this data directory; use pacaroute ui"))
	}
	defer serviceUnlock()
	auth.systemKeys = runtime.GOOS == "darwin"
	settings := readDesktopSettings(s.Root)
	control := &desktopControl{store: s, auth: auth, port: port, adminPort: settings.AdminPort, nativeTray: true}
	if e := control.startManagement(); e != nil {
		return fail(e)
	}
	if settings.Serving == nil || *settings.Serving {
		_ = control.start()
	} else {
		_ = control.stop()
	}
	systray.Run(func() {
		if runtime.GOOS == "windows" {
			systray.SetIcon(trayICO)
		} else {
			systray.SetIcon(trayPNG)
		}
		systray.SetTooltip("PacaRoute")
		status := systray.AddMenuItem("PacaRoute", "")
		status.Disable()
		open := systray.AddMenuItem("打开管理界面…", "在浏览器中管理来源与路由")
		toggle := systray.AddMenuItem("关闭模型服务端口", "管理界面保持运行")
		startup := systray.AddMenuItemCheckbox("登录时启动", "", startupEnabled())
		systray.AddSeparator()
		quit := systray.AddMenuItem("退出 PacaRoute", "")
		update := func() {
			running, p, err := control.state()
			if err != "" {
				status.SetTitle("⚠ " + err)
			} else if running {
				status.SetTitle(fmt.Sprintf("● 运行中 · %d", p))
			} else {
				status.SetTitle(fmt.Sprintf("○ 已停止 · %d", p))
			}
			if running {
				toggle.SetTitle("关闭模型服务端口")
			} else {
				toggle.SetTitle("打开模型服务端口")
			}
			if startupEnabled() {
				startup.Check()
			} else {
				startup.Uncheck()
			}
		}
		update()
		go func() {
			if a.get("no-open", "") != "true" {
				_ = openManagement(s.Root, control.managementPort())
			}
			tick := time.NewTicker(2 * time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					systray.Quit()
					return
				case <-quit.ClickedCh:
					systray.Quit()
					return
				case <-open.ClickedCh:
					if e = openManagement(s.Root, control.managementPort()); e != nil {
						control.mu.Lock()
						control.lastError = "无法打开浏览器"
						control.mu.Unlock()
					}
					update()
				case <-toggle.ClickedCh:
					running, _, _ := control.state()
					if running {
						_ = control.stop()
					} else {
						_ = control.start()
					}
					update()
				case <-startup.ClickedCh:
					if e := control.startup(!startupEnabled()); e != nil {
						control.mu.Lock()
						control.lastError = "开机启动设置失败"
						control.mu.Unlock()
					}
					update()
				case <-tick.C:
					update()
				}
			}
		}()
		emit(map[string]any{"event": "desktop-ready", "port": port, "admin_port": control.managementPort()})
	}, func() { control.close() })
	return 0
}
