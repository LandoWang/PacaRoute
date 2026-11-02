//go:build desktop && (darwin || windows)

package pacaroute

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/getlantern/systray"
	"path/filepath"
	"runtime"
	"time"
)

//go:embed trayassets/mark.png
var trayPNG []byte

//go:embed trayassets/mark.ico
var trayICO []byte

const desktopEnabled = true

func runDesktop(ctx context.Context, a arguments, s *ConfigStore, auth *Auth, port int, emit func(any), fail func(error) int) int {
	port = desktopPort(a, s.Root, port)
	appUnlock, e := fileLock(ctx, filepath.Join(s.Root, "app.lock"), 200*time.Millisecond)
	if e != nil {
		_ = openManagement(s.Root, port)
		return fail(errors.New("a desktop application already owns this data directory"))
	}
	defer appUnlock()
	serviceUnlock, e := fileLock(ctx, filepath.Join(s.Root, "pacaroute-service.lock"), 200*time.Millisecond)
	if e != nil {
		return fail(errors.New("a service already owns this data directory; use pacaroute ui"))
	}
	defer serviceUnlock()
	auth.systemKeys = runtime.GOOS == "darwin"
	control := &desktopControl{store: s, auth: auth, port: port}
	_ = control.start()
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
		toggle := systray.AddMenuItem("停止服务", "")
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
				toggle.SetTitle("停止服务")
			} else {
				toggle.SetTitle("启动服务")
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
				if e := control.start(); e == nil {
					_ = openManagement(s.Root, control.portValue())
				}
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
					if e := control.start(); e == nil {
						if e = openManagement(s.Root, control.portValue()); e != nil {
							control.mu.Lock()
							control.lastError = "无法打开浏览器"
							control.mu.Unlock()
						}
					}
					update()
				case <-toggle.ClickedCh:
					running, _, _ := control.state()
					if running {
						control.stop()
					} else {
						_ = control.start()
					}
					update()
				case <-startup.ClickedCh:
					if e := setStartup(!startupEnabled(), s.Root, control.portValue(), auth.vault.root); e != nil {
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
		emit(map[string]any{"event": "desktop-ready", "port": port})
	}, func() { control.stop() })
	return 0
}
