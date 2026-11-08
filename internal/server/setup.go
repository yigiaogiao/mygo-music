package server

import (
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mygo-music/internal/i18n"
)

var setupTmpl = template.Must(template.New("setup").Funcs(template.FuncMap{
	"t":  i18n.T,
	"tf": i18n.Tf,
}).Parse(setupHTML))

type crumb struct {
	Name string
	Path string
	URL  string
}

type dirRow struct {
	Name    string
	Path    string
	URL     string
	Added   bool
	Symlink bool
}

type folderRow struct {
	Name  string
	Path  string
	Whole bool
}

type setupData struct {
	Root         string
	Path         string
	Parent       string
	ParentURL    string
	Crumbs       []crumb
	Dirs         []dirRow
	Folders      []folderRow
	Scanning     bool
	Tracks       int
	Message      string
	Problem      string
	Version      string
	OnlineLyrics bool
	AtRoot       bool
	Added        bool
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.setupPage(w, r)
	case http.MethodPost:
		s.setupAction(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) setupPage(w http.ResponseWriter, r *http.Request) {
	root := s.cfg.MusicDir
	path := r.URL.Query().Get("path")
	if path == "" {
		path = s.startPath(root)
	}
	path = abs(path)
	var problem string
	if !under(path, root) {
		path, problem = root, "That folder is not on the netdisk."
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		problem = "The netdisk is not mounted, or the folder below is empty."
	}

	d := setupData{
		Root:         root,
		Path:         path,
		Parent:       filepath.Dir(path),
		ParentURL:    browseURL(filepath.Dir(path)),
		Scanning:     s.local.Scanning(),
		Tracks:       s.local.TrackCount(),
		Version:      s.cfg.Version,
		OnlineLyrics: s.cfg.OnlineLyrics,
		Problem:      problem,
		AtRoot:       path == root,
		Message:      setupMessage(r),
	}
	d.Crumbs = crumbs(root, path)
	d.Added = containsPath(s.local.Folders(), path)
	if problem == "" {
		dirs, err := s.setupDirs(path)
		if err != nil {
			// The mount is there but its contents are not readable: usually
			// the package did not declare media.read.
			problem = "This folder could not be read. Check that the app may read the netdisk."
		} else {
			d.Dirs = dirs
		}
	}
	d.Problem = problem
	for _, f := range s.local.Folders() {
		d.Folders = append(d.Folders, folderRow{
			Name:  shortPath(f, root),
			Path:  f,
			Whole: filepath.Clean(f) == filepath.Clean(root),
		})
	}

	s.langMu.Lock()
	i18n.SetLanguage(language(r))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	err := setupTmpl.Execute(w, d)
	s.langMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// startPath is where the netdisk opens: its root, or else the home of the
// single user whose folder it holds, rather than a lone name.
func (s *Server) startPath(root string) string {
	if rows, err := s.setupDirs(root); err == nil && len(rows) == 1 && !rows[0].Symlink {
		return rows[0].Path
	}
	return root
}

func (s *Server) setupDirs(path string) ([]dirRow, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var rows []dirRow
	added := s.local.Folders()
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(path, name)
		info, err := os.Stat(full)
		if err != nil || !info.IsDir() {
			continue
		}
		rows = append(rows, dirRow{
			Name:    name,
			Path:    full,
			URL:     browseURL(full),
			Added:   containsPath(added, full),
			Symlink: e.Type()&os.ModeSymlink != 0,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	return rows, nil
}

func (s *Server) setupAction(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	action := r.FormValue("action")
	path := abs(r.FormValue("path"))
	back := path
	if back == "" || !under(back, s.cfg.MusicDir) {
		back = s.cfg.MusicDir
	}
	switch action {
	case "add":
		if !under(path, s.cfg.MusicDir) {
			http.Error(w, "not on the netdisk", http.StatusBadRequest)
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			http.Error(w, "not a folder", http.StatusBadRequest)
			return
		}
		s.local.AddFolders(path)
		http.Redirect(w, r, browseURL(back)+"&added=1", http.StatusSeeOther)
	case "remove":
		s.local.RemoveFolder(path)
		http.Redirect(w, r, browseURL(back)+"&removed=1", http.StatusSeeOther)
	case "rescan":
		s.local.Rescan()
		http.Redirect(w, r, browseURL(back)+"&rescanned=1", http.StatusSeeOther)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func setupMessage(r *http.Request) string {
	switch {
	case r.URL.Query().Get("added") == "1":
		return "Folder added. Looking for music…"
	case r.URL.Query().Get("removed") == "1":
		return "Folder removed."
	case r.URL.Query().Get("rescanned") == "1":
		return "Looking for music again…"
	}
	return ""
}

func browseURL(path string) string {
	return "/setup?path=" + url.QueryEscape(path)
}

func crumbs(root, path string) []crumb {
	cs := []crumb{{Name: filepath.Base(root), Path: root, URL: browseURL(root)}}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return cs
	}
	at := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		at = filepath.Join(at, part)
		cs = append(cs, crumb{Name: part, Path: at, URL: browseURL(at)})
	}
	return cs
}

func containsPath(paths []string, path string) bool {
	for _, p := range paths {
		if filepath.Clean(p) == filepath.Clean(path) {
			return true
		}
	}
	return false
}

// shortPath names a selected folder by where it is in the netdisk, if it is.
func shortPath(path, root string) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return rel
	}
	return path
}

const setupHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>{{t "Music folders"}} · MyGo Music</title>
<style>
:root{color-scheme:dark;--bg:#101014;--card:#191920;--hover:#22222b;--text:#f2f2f5;--muted:#9a9aa6;--line:#2b2b35;--accent:#8b7cff;--on:#15121f;--ok:#65d49a}
*{box-sizing:border-box}
html,body{margin:0;background:var(--bg);color:var(--text);font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
body{padding:calc(24px + env(safe-area-inset-top)) max(18px,env(safe-area-inset-right)) calc(40px + env(safe-area-inset-bottom)) max(18px,env(safe-area-inset-left))}
main{max-width:760px;margin:0 auto}
header{display:flex;align-items:baseline;gap:10px;flex-wrap:wrap;margin-bottom:8px}
h1{font-size:24px;margin:0;letter-spacing:-.4px}
.version{color:var(--muted);font-size:12px}
h2{font-size:13px;color:var(--muted);font-weight:600;letter-spacing:.4px;text-transform:uppercase;margin:26px 0 8px}
p{margin:0 0 12px;color:var(--muted)}
a{color:var(--accent);text-decoration:none}
.card{background:var(--card);border:1px solid var(--line);border-radius:14px;overflow:hidden}
.row{display:flex;align-items:center;gap:12px;padding:12px 16px;border-top:1px solid var(--line);min-height:52px}
.row:first-child{border-top:0}
.row .name{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.row .sub{color:var(--muted);font-size:12px}
.row:hover{background:var(--hover)}
button,.button{appearance:none;border:0;border-radius:9px;padding:9px 14px;background:var(--hover);color:var(--text);font:inherit;cursor:pointer;white-space:nowrap;min-height:38px}
button:hover,.button:hover{filter:brightness(1.15)}
button.primary{background:var(--accent);color:var(--on);font-weight:600}
button:disabled{opacity:.45;cursor:default}
form{display:inline;margin:0}
.bar{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin:14px 0}
.pill{display:inline-flex;align-items:center;gap:7px;border:1px solid var(--line);border-radius:999px;padding:6px 11px;color:var(--muted);font-size:12px}
.dot{width:7px;height:7px;border-radius:50%;background:var(--ok)}
.dot.scanning{background:var(--accent);animation:pulse 1.2s ease-in-out infinite}
@keyframes pulse{50%{opacity:.25}}
.crumbs{display:flex;gap:6px;flex-wrap:wrap;align-items:center;font-size:13px;color:var(--muted);margin:6px 0 14px}
.crumbs a{color:var(--text)}
.crumbs span{color:var(--muted)}
.notice{padding:12px 16px;border-radius:12px;background:#20251f;color:#bfe8cd;border:1px solid #2d4234;margin:16px 0}
.problem{padding:12px 16px;border-radius:12px;background:#2a1d22;color:#f2b8c6;border:1px solid #4a2b35;margin:16px 0}
.empty{color:var(--muted);padding:18px 16px;text-align:center}
code{background:var(--card);border:1px solid var(--line);border-radius:6px;padding:2px 6px;font-size:12px;word-break:break-all}
.footer{display:flex;gap:10px;flex-wrap:wrap;margin-top:28px}
.footer .button{background:var(--card);border:1px solid var(--line)}
@media (max-width:520px){
 body{font-size:14px}
 h1{font-size:21px}
 .row{padding:11px 13px}
 button,.button{padding:10px 13px}
}
</style>
</head>
<body>
<main>
<header><h1>MyGo Music</h1><span class="version">{{.Version}}</span></header>
<p>{{t "Pick the folders on your LazyCat netdisk, and their songs will all be here."}}</p>
<p>{{t "Netdisk folder"}}: <code>{{.Root}}</code></p>

<div class="bar">
  <span class="pill">{{if .Scanning}}<i class="dot scanning"></i>{{t "Looking for music…"}}{{else}}<i class="dot"></i>{{tf "%d songs found" .Tracks}}{{end}}</span>
  {{if .OnlineLyrics}}<span class="pill">{{t "Lyrics online"}} · LRCLib</span>{{end}}
  <span class="pill">{{t "Read only"}}</span>
  <form method="post" action="/setup"><input type="hidden" name="action" value="rescan"><input type="hidden" name="path" value="{{.Path}}"><button type="submit">{{t "Rescan"}}</button></form>
</div>

{{if .Message}}<div class="notice">{{t .Message}}</div>{{end}}
{{if .Problem}}<div class="problem">{{t .Problem}}</div>{{end}}

<h2>{{t "Selected Folders"}}</h2>
<div class="card">
{{range .Folders}}
  <div class="row">
    <span class="name" title="{{.Path}}">{{if .Whole}}{{t "The whole netdisk"}}{{else}}{{.Name}}{{end}}</span>
    <form method="post" action="/setup"><input type="hidden" name="action" value="remove"><input type="hidden" name="path" value="{{.Path}}"><button type="submit">{{t "Remove"}}</button></form>
  </div>
{{else}}
  <div class="empty">{{t "No folders chosen yet."}}</div>
{{end}}
</div>

<h2>{{t "Browse the netdisk"}}</h2>
<div class="crumbs">
{{range $i, $c := .Crumbs}}{{if $i}}<span>/</span>{{end}}<a href="{{$c.URL}}">{{$c.Name}}</a>{{end}}
</div>
<div class="bar">
  {{if not .AtRoot}}<a class="button" href="{{.ParentURL}}">{{t "Back"}}</a>{{end}}
  {{if .Added}}<button disabled>{{t "Added"}}</button>{{else}}<button class="primary" form="add-here" type="submit">{{t "Add This Folder"}}</button>{{end}}
</div>
<form id="add-here" method="post" action="/setup"><input type="hidden" name="action" value="add"><input type="hidden" name="path" value="{{.Path}}"></form>

<div class="card">
{{range .Dirs}}
  <div class="row">
    <a class="name" href="{{.URL}}">{{.Name}}{{if .Symlink}} <span class="sub">↗</span>{{end}}</a>
    {{if .Added}}<span class="sub">{{t "Added"}}</span>{{else}}<form method="post" action="/setup"><input type="hidden" name="action" value="add"><input type="hidden" name="path" value="{{.Path}}"><button type="submit">{{t "Add"}}</button></form>{{end}}
  </div>
{{else}}
  <div class="empty">{{if and .AtRoot (not .Problem)}}{{t "The netdisk looks empty. If it has folders, check that media access is on, then restart the app."}}{{else}}{{t "This folder has no subfolders."}}{{end}}</div>
{{end}}
</div>

<p style="margin-top:22px">{{t "Files are read from the netdisk as they play. Nothing is copied or changed."}}</p>
<p>{{t "Songs without timed lyrics get them from LRCLib when they play."}}</p>

<div class="footer">
  <a class="button" href="/">{{t "Open the Player"}}</a>
</div>
</main>
</body>
</html>`
