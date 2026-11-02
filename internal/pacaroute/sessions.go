package pacaroute

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	_ "modernc.org/sqlite"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type localSession struct {
	ID       string `json:"id"`
	Agent    string `json:"agent"`
	Title    string `json:"title"`
	CWD      string `json:"cwd"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Updated  int64  `json:"updated"`
	Archived bool   `json:"archived"`
	File     string `json:"-"`
}
type sessionItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}
type cachedSession struct {
	Path     string       `json:"path"`
	Size     int64        `json:"size"`
	Modified int64        `json:"modified"`
	Session  localSession `json:"session"`
}
type sessionIndex struct {
	mu       sync.Mutex
	root     string
	files    map[string]cachedSession
	selected map[string]localSession
}

func (s *Server) sessionReader() *sessionIndex {
	s.sessionsOnce.Do(func() {
		s.sessions = &sessionIndex{root: s.Store.Root, files: map[string]cachedSession{}, selected: map[string]localSession{}}
		b, e := os.ReadFile(filepath.Join(s.Store.Root, "session-index.json"))
		if e == nil && len(b) <= 16<<20 {
			_ = json.Unmarshal(b, &s.sessions.files)
		}
	})
	return s.sessions
}
func sessionRoot(agent string) (string, error) {
	p, e := clientPath(agent)
	if e != nil {
		return "", e
	}
	return filepath.Dir(p), nil
}
func sessionDatabase(root, prefix string) string {
	files, _ := os.ReadDir(root)
	best := -1
	var path string
	for _, f := range files {
		if f.Type()&os.ModeSymlink != 0 || f.IsDir() {
			continue
		}
		name := f.Name()
		if !strings.HasPrefix(name, prefix+"_") || !strings.HasSuffix(name, ".sqlite") {
			continue
		}
		n, e := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, prefix+"_"), ".sqlite"))
		if e == nil && n > best {
			best = n
			path = filepath.Join(root, name)
		}
	}
	return path
}
func openSessionDB(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "trusted_schema(0)", "busy_timeout(1500)"}}
	u.RawQuery = q.Encode()
	db, e := sql.Open("sqlite", u.String())
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	return db, nil
}
func sessionColumns(ctx context.Context, db *sql.DB, table string) map[string]bool {
	out := map[string]bool{}
	rows, e := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if e != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var defaultValue any
		if rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk) == nil {
			out[name] = true
		}
	}
	return out
}
func column(columns map[string]bool, name, fallback string) string {
	if columns[name] {
		return name
	}
	return fallback
}
func (i *sessionIndex) catalog(ctx context.Context, agent, search string, offset int) (map[string]any, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	root, e := sessionRoot(agent)
	if e != nil {
		return nil, e
	}
	if offset < 0 || offset > 100000 {
		return nil, errors.New("invalid session offset")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	items := []localSession{}
	more := false
	notice := ""
	if path := sessionDatabase(root, "state"); agent == "codex" && path != "" {
		db, e := openSessionDB(path)
		if e != nil {
			return nil, e
		}
		defer db.Close()
		cols := sessionColumns(ctx, db, "threads")
		if !cols["id"] || !cols["cwd"] || !cols["updated_at"] {
			return nil, errors.New("unsupported Codex session index schema")
		}
		title := column(cols, "title", "''")
		if cols["name"] {
			title = "COALESCE(NULLIF(name,'')," + title + ")"
		}
		query := "SELECT id," + title + ",cwd," + column(cols, "model_provider", "''") + "," + column(cols, "model", "''") + ",updated_at," + column(cols, "archived", "0") + "," + column(cols, "rollout_path", "''") + " FROM threads WHERE (?='' OR instr(lower(" + title + "),lower(?))>0 OR instr(lower(cwd),lower(?))>0) ORDER BY updated_at DESC,id LIMIT 81 OFFSET ?"
		rows, e := db.QueryContext(ctx, query, search, search, search, offset)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		for rows.Next() {
			var v localSession
			var title, cwd, provider, model, path sql.NullString
			var archived int
			if e = rows.Scan(&v.ID, &title, &cwd, &provider, &model, &v.Updated, &archived, &path); e != nil {
				return nil, e
			}
			v.Agent = agent
			v.Title = title.String
			v.CWD = cwd.String
			v.Provider = provider.String
			v.Model = model.String
			v.File = path.String
			v.Archived = archived != 0
			items = append(items, v)
		}
		if e = rows.Err(); e != nil {
			return nil, e
		}
		more = len(items) > 80
		if more {
			items = items[:80]
		}
	} else {
		dirs := []string{"projects"}
		if agent == "codex" {
			dirs = []string{"sessions", "archived_sessions"}
		}
		type entry struct {
			path string
			info fs.FileInfo
		}
		files := []entry{}
		visited := 0
		capped := false
		for _, dir := range dirs {
			_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err != nil {
					return nil
				}
				visited++
				if visited > 20000 {
					capped = true
					return fs.SkipAll
				}
				if d.Type()&os.ModeSymlink != 0 {
					return nil
				}
				if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
					info, e := d.Info()
					if e == nil && info.Mode().IsRegular() {
						files = append(files, entry{path, info})
					}
				}
				return nil
			})
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sort.Slice(files, func(a, b int) bool { return files[a].info.ModTime().After(files[b].info.ModTime()) })
		matches := 0
		for _, f := range files {
			key := agent + ":" + keyHash(f.path)
			cached, ok := i.files[key]
			if !ok || cached.Size != f.info.Size() || cached.Modified != f.info.ModTime().UnixNano() {
				v := localSession{ID: key, Agent: agent, Title: "未命名会话", Updated: f.info.ModTime().Unix(), File: f.path, Archived: strings.Contains(f.path, "archived_sessions")}
				file, e := os.Open(f.path)
				if e != nil {
					continue
				}
				scan := bufio.NewScanner(io.LimitReader(file, 128<<10))
				scan.Buffer(make([]byte, 8192), 128<<10)
				for scan.Scan() {
					var record map[string]any
					if json.Unmarshal(scan.Bytes(), &record) != nil {
						continue
					}
					payload := anyObject(record["payload"])
					if sessionString(record["cwd"]) != "" {
						v.CWD = sessionString(record["cwd"])
					}
					if sessionString(record["type"]) == "session_meta" {
						v.CWD = sessionString(payload["cwd"])
						v.Provider = sessionString(payload["model_provider"])
					}
					if item := fileSessionItem(record, agent, 0); item != nil && item.Kind == "user" && v.Title == "未命名会话" {
						v.Title = trimSessionText(strings.ReplaceAll(item.Text, "\n", " "), 180)
					}
				}
				file.Close()
				cached = cachedSession{f.path, f.info.Size(), f.info.ModTime().UnixNano(), v}
				i.files[key] = cached
			}
			v := cached.Session
			v.File = f.path
			if search != "" && !strings.Contains(strings.ToLower(v.Title+" "+v.CWD), strings.ToLower(search)) {
				continue
			}
			matches++
			if matches > offset {
				items = append(items, v)
			}
			if len(items) > 80 {
				more = true
				items = items[:80]
				break
			}
		}
		if capped {
			notice = "目录较大，本次最多检查 20,000 个目录项。可使用搜索缩小范围。"
		}
		if len(i.files) > 10000 {
			i.files = map[string]cachedSession{}
		}
		b, _ := json.Marshal(i.files)
		_ = atomicWrite(filepath.Join(i.root, "session-index.json"), b, 0600)
	}
	if len(i.selected) > 10000 {
		i.selected = map[string]localSession{}
	}
	for _, s := range items {
		i.selected[agent+":"+s.ID] = s
	}
	return map[string]any{"sessions": items, "has_more": more, "notice": notice, "root": root}, nil
}
func (i *sessionIndex) page(ctx context.Context, agent, id, before string) (map[string]any, error) {
	i.mu.Lock()
	session, ok := i.selected[agent+":"+id]
	i.mu.Unlock()
	if !ok {
		return nil, errors.New("refresh the session list before opening this session")
	}
	root, e := sessionRoot(agent)
	if e != nil {
		return nil, e
	}
	if path := sessionDatabase(root, "thread_history"); agent == "codex" && path != "" && !strings.HasPrefix(before, "file:") {
		db, e := openSessionDB(path)
		if e != nil {
			return nil, e
		}
		defer db.Close()
		cols := sessionColumns(ctx, db, "thread_items")
		if !cols["thread_id"] || !cols["rollout_ordinal"] || !cols["item_json"] {
			return nil, errors.New("unsupported Codex history schema")
		}
		ordinal := int64(9223372036854775807)
		if before != "" {
			ordinal, e = strconv.ParseInt(strings.TrimPrefix(before, "db:"), 10, 64)
			if e != nil {
				return nil, errors.New("invalid history cursor")
			}
		}
		rows, e := db.QueryContext(ctx, "SELECT rollout_ordinal,CASE WHEN length(item_json)<=2000000 THEN item_json ELSE NULL END FROM thread_items WHERE thread_id=? AND rollout_ordinal<? ORDER BY rollout_ordinal DESC LIMIT 101", id, ordinal)
		if e != nil {
			return nil, e
		}
		items := []sessionItem{}
		var position int64
		count := 0
		older := ""
		for rows.Next() {
			var data sql.NullString
			if e = rows.Scan(&position, &data); e != nil {
				rows.Close()
				return nil, e
			}
			count++
			if count > 100 {
				older = "db:" + strconv.FormatInt(ordinal, 10)
				break
			}
			ordinal = position
			var record map[string]any
			if !data.Valid || json.Unmarshal([]byte(data.String), &record) != nil {
				items = append(items, sessionItem{ID: fmt.Sprint(position), Kind: "context", Title: "记录未展开", Text: "该记录过大或 JSON 不完整。"})
				continue
			}
			if item := codexSessionItem(record, fmt.Sprint(position)); item != nil {
				items = append(items, *item)
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if count > 0 || before != "" {
			for l, r := 0, len(items)-1; l < r; l, r = l+1, r-1 {
				items[l], items[r] = items[r], items[l]
			}
			return map[string]any{"items": items, "older": older}, nil
		}
	}
	return fileSessionPage(root, session, agent, before)
}
func fileSessionPage(root string, session localSession, agent, before string) (map[string]any, error) {
	base, e := filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	path, e := filepath.EvalSymlinks(session.File)
	if e != nil {
		return nil, errors.New("session file was moved or removed")
	}
	rel, e := filepath.Rel(base, path)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, errors.New("session file is outside the Agent directory")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil || !stat.Mode().IsRegular() {
		return nil, errors.New("not a session file")
	}
	end := stat.Size()
	if before != "" {
		end, e = strconv.ParseInt(strings.TrimPrefix(before, "file:"), 10, 64)
		if e != nil || end < 0 || end > stat.Size() {
			return nil, errors.New("invalid history cursor")
		}
	}
	start := max(int64(0), end-(4<<20))
	data := make([]byte, end-start)
	_, e = f.ReadAt(data, start)
	if e != nil && e != io.EOF {
		return nil, e
	}
	type positioned struct {
		offset int64
		item   sessionItem
	}
	items := []positioned{}
	position := 0
	boundary := start
	skipped := 0
	for position < len(data) {
		n := bytes.IndexByte(data[position:], '\n')
		if n < 0 {
			break
		}
		lineStart := position
		position += n + 1
		if lineStart == 0 && start > 0 {
			boundary = start + int64(position)
			continue
		}
		var record map[string]any
		if json.Unmarshal(data[lineStart:position-1], &record) != nil {
			skipped++
			continue
		}
		offset := start + int64(lineStart)
		if item := fileSessionItem(record, agent, offset); item != nil {
			items = append(items, positioned{offset, *item})
		}
	}
	seen := map[string]bool{}
	selected := []positioned{}
	for j := len(items) - 1; j >= 0; j-- {
		if !seen[items[j].item.ID] {
			seen[items[j].item.ID] = true
			selected = append(selected, items[j])
		}
	}
	older := ""
	if len(selected) > 100 {
		selected = selected[:100]
		older = "file:" + fmt.Sprint(selected[len(selected)-1].offset)
	} else if start > 0 {
		older = "file:" + fmt.Sprint(min(boundary, end-1))
	}
	out := []sessionItem{}
	for j := len(selected) - 1; j >= 0; j-- {
		out = append(out, selected[j].item)
	}
	notice := ""
	if start > 0 || skipped > 0 {
		notice = "按片段读取历史；超过 4 MB 或不完整的记录可能被省略。"
	}
	return map[string]any{"items": out, "older": older, "notice": notice}, nil
}
func anyObject(v any) map[string]any { m, _ := v.(map[string]any); return m }
func trimSessionText(v string, n int) string {
	r := []rune(v)
	if len(r) > n {
		return string(r[:n]) + "\n[内容截断]"
	}
	return v
}
func sessionText(v any) string {
	switch x := v.(type) {
	case string:
		return trimSessionText(x, 24000)
	case []any:
		parts := []string{}
		for _, b := range x {
			parts = append(parts, sessionText(b))
		}
		return trimSessionText(strings.Join(parts, "\n"), 24000)
	case map[string]any:
		switch sessionString(x["type"]) {
		case "text", "input_text", "output_text":
			return sessionText(x["text"])
		case "image", "image_url", "input_image":
			return "[图片附件]"
		case "tool_use":
			return sessionString(x["name"]) + "\n" + sessionJSON(x["input"])
		case "tool_result":
			return sessionText(x["content"])
		}
		return sessionJSON(x)
	default:
		return sessionJSON(x)
	}
}
func sessionJSON(v any) string {
	if v == nil {
		return ""
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return trimSessionText(string(b), 24000)
}
func codexSessionItem(v map[string]any, fallback string) *sessionItem {
	item := sessionItem{ID: sessionString(v["id"]), Kind: "context", Title: "记录"}
	if item.ID == "" {
		item.ID = fallback
	}
	switch sessionString(v["type"]) {
	case "userMessage":
		item.Kind = "user"
		item.Title = "你"
		item.Text = sessionText(v["content"])
	case "agentMessage":
		item.Kind = "assistant"
		item.Title = "Assistant"
		item.Text = sessionString(v["text"])
	case "commandExecution":
		item.Kind = "tool"
		item.Title = "命令"
		item.Text = sessionString(v["command"]) + "\n\n" + sessionString(v["aggregatedOutput"])
		if sessionString(v["status"]) == "failed" {
			item.Kind = "error"
		}
	case "mcpToolCall":
		item.Kind = "tool"
		item.Title = sessionString(v["tool"])
		item.Text = sessionJSON(v["arguments"]) + "\n" + sessionText(anyObject(v["result"])["content"]) + "\n" + sessionJSON(v["error"])
	case "fileChange":
		item.Kind = "tool"
		item.Title = "文件修改"
		item.Text = sessionJSON(v["changes"])
	case "functionCallOutput":
		item.Kind = "result"
		item.Title = "工具结果"
		item.Text = sessionText(v["output"])
	case "contextCompaction":
		item.Title = "上下文压缩"
		item.Text = "客户端在这里压缩了上下文。"
	case "webSearch":
		item.Kind = "tool"
		item.Title = "搜索"
		item.Text = sessionString(v["query"])
	case "imageView":
		item.Title = "图片"
		item.Text = "[图片附件]"
	default:
		return nil
	}
	item.Text = trimSessionText(item.Text, 24000)
	return &item
}
func fileSessionItem(v map[string]any, agent string, offset int64) *sessionItem {
	id := fmt.Sprintf("line-%d", offset)
	kind := sessionString(v["type"])
	if agent == "codex" {
		p := anyObject(v["payload"])
		if kind == "event_msg" {
			switch sessionString(p["type"]) {
			case "item_completed":
				return codexSessionItem(anyObject(p["item"]), id)
			case "user_message":
				return &sessionItem{ID: id, Kind: "user", Title: "你", Text: sessionText(p["message"])}
			case "agent_message":
				return &sessionItem{ID: id, Kind: "assistant", Title: "Assistant", Text: sessionText(p["message"])}
			case "error":
				return &sessionItem{ID: id, Kind: "error", Title: "错误", Text: sessionText(p["message"])}
			case "exec_command_end":
				return &sessionItem{ID: id, Kind: "tool", Title: "命令", Text: sessionText(p["command"]) + "\n" + sessionText(p["aggregated_output"])}
			}
		}
		if kind == "compacted" {
			return &sessionItem{ID: id, Kind: "context", Title: "上下文压缩", Text: "客户端在这里压缩了上下文。"}
		}
		return nil
	}
	if kind != "user" && kind != "assistant" {
		return nil
	}
	message := anyObject(v["message"])
	if uuid := sessionString(v["uuid"]); uuid != "" {
		id = uuid
	}
	title := "Assistant"
	if kind == "user" {
		title = "你"
	}
	blocks, _ := message["content"].([]any)
	for _, block := range blocks {
		b := anyObject(block)
		if sessionString(b["type"]) == "tool_use" {
			kind = "tool"
			title = "工具调用"
		}
		if sessionString(b["type"]) == "tool_result" {
			kind = "result"
			title = "工具结果"
		}
		if b["is_error"] == true {
			kind = "error"
			title = "错误"
		}
	}
	text := sessionText(message["content"])
	if text == "" {
		return nil
	}
	return &sessionItem{ID: id, Kind: kind, Title: title, Text: text, Timestamp: sessionString(v["timestamp"])}
}

func sessionString(v any) string { s, _ := v.(string); return s }
