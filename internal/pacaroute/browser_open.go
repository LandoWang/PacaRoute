package pacaroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func openURL(address string) error {
	u, e := url.Parse(address)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("invalid browser URL")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("/usr/bin/open", address)
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", address)
	default:
		command = exec.Command("xdg-open", address)
	}
	return command.Run()
}
func openManagement(root string, port int) error {
	key, e := os.ReadFile(filepath.Join(root, "admin.key"))
	if e != nil {
		return errors.New("start PacaRoute before opening management")
	}
	defer clear(key)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	address := fmt.Sprintf("http://127.0.0.1:%d", port)
	req, _ := http.NewRequestWithContext(ctx, "POST", address+"/api/local/browser-ticket", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+string(key))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := client.Do(req)
	if e != nil {
		return errors.New("PacaRoute is not listening on this port; start the service first")
	}
	defer response.Body.Close()
	if response.StatusCode == 403 {
		return openURL(address)
	}
	if response.StatusCode != 200 {
		return errors.New("management did not accept this data directory; check the selected port")
	}
	var v struct {
		Ticket string `json:"ticket"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&v) != nil || v.Ticket == "" {
		return errors.New("invalid browser ticket")
	}
	return openURL(address + "/#/connect?ticket=" + url.QueryEscape(v.Ticket))
}
