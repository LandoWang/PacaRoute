package pacaroute

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func (a *Auth) AzureAction(ctx context.Context, id, action, subscription, resource string) (map[string]any, error) {
	c := a.store.Snapshot()
	p, _ := c.Proxy(c.Proxies.AccountProxyID)
	env := a.azureEnv(id, p)
	args := []string{"account", "list", "-o", "json"}
	switch action {
	case "account", "account-status", "azure-subscriptions":
	case "azure-resources":
		if subscription == "" {
			return nil, errors.New("select an Azure subscription")
		}
		args = []string{"cognitiveservices", "account", "list", "--subscription", subscription, "-o", "json"}
	case "azure-deployments":
		parts := strings.Split(resource, "/")
		if len(parts) != 9 || parts[2] != subscription || !strings.EqualFold(parts[6], "Microsoft.CognitiveServices") {
			return nil, errors.New("select a resource in this subscription")
		}
		args = []string{"cognitiveservices", "account", "deployment", "list", "--subscription", subscription, "--resource-group", parts[4], "--name", parts[8], "-o", "json"}
	default:
		return nil, errors.New("unsupported Azure action")
	}
	b, e := commandJSON(ctx, env, args...)
	if e != nil {
		return nil, e
	}
	items := array(b)
	out := map[string]any{}
	switch action {
	case "account", "account-status", "azure-subscriptions":
		list := []any{}
		for _, v := range items {
			if rawString(v["state"]) != "Enabled" {
				continue
			}
			login := rawString(obj(v["user"])["name"])
			list = append(list, map[string]string{"id": rawString(v["id"]), "name": rawString(v["name"]), "login": login})
			if out["login"] == nil {
				out["login"] = login
			}
		}
		out["logged_in"] = len(list) > 0
		out["subscriptions"] = list
	case "azure-resources":
		list := []any{}
		for _, v := range items {
			kind := rawString(v["kind"])
			if kind != "OpenAI" && kind != "AIServices" {
				continue
			}
			list = append(list, map[string]string{"id": rawString(v["id"]), "name": rawString(v["name"]), "endpoint": rawString(obj(v["properties"])["endpoint"])})
		}
		out["resources"] = list
	case "azure-deployments":
		list := []Model{}
		for _, v := range items {
			if rawString(obj(obj(v["properties"])["model"])["format"]) == "OpenAI" {
				id := rawString(v["name"])
				list = append(list, Model{id, id, "Azure OpenAI", true, true})
			}
		}
		out["models"] = list
	}
	return out, nil
}
func (a *Auth) AzureLogin(ctx context.Context, id string, logout bool, emit func(any)) error {
	if id == "azure-local" {
		return errors.New("use az login in your terminal for the shared Azure CLI account, or add an isolated Azure account")
	}
	c := a.store.Snapshot()
	p, _ := c.Proxy(c.Proxies.AccountProxyID)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	args := []string{"login", "--use-device-code", "--allow-no-subscriptions", "-o", "none"}
	if logout {
		args = []string{"logout"}
	}
	cmd := exec.CommandContext(ctx, azureCLI(), args...)
	cmd.Env = append(a.azureEnv(id, p), "AZURE_CORE_LOGIN_EXPERIENCE_V2=off")
	stderr, e := cmd.StderrPipe()
	if e != nil {
		return e
	}
	if e = cmd.Start(); e != nil {
		return errors.New("install Azure CLI to sign in")
	}
	scan := bufio.NewScanner(stderr)
	re := regexp.MustCompile(`(?i)enter the code\s+([A-Z0-9]{6,20})`)
	for scan.Scan() {
		if m := re.FindStringSubmatch(scan.Text()); len(m) > 1 {
			emit(map[string]any{"ok": true, "event": "device_code", "user_code": m[1], "verification_uri": "https://microsoft.com/devicelogin", "expires_in": 900})
		}
	}
	if e = cmd.Wait(); e != nil {
		return errors.New("Azure device login failed or was cancelled")
	}
	return nil
}

var _ = json.Valid
