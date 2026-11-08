package server

import (
	"errors"
	"os"
	"path/filepath"
)

// The app's folder picker asks the server to browse the netdisk, and to add
// or remove the folders the library reads.

// folderPage is where the picker is on the netdisk, and what is there.
type folderPage struct {
	Root    string        `json:"root"`
	Path    string        `json:"path"`
	Parent  string        `json:"parent"`
	AtRoot  bool          `json:"atRoot"`
	Crumbs  []folderCrumb `json:"crumbs"`
	Dirs    []folderDir   `json:"dirs"`
	Folders []folderPick  `json:"folders"`
	Added   bool          `json:"added"`
	Problem string        `json:"problem,omitempty"`
}

type folderCrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type folderDir struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Added   bool   `json:"added"`
	Symlink bool   `json:"symlink"`
}

type folderPick struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Whole bool   `json:"whole"`
}

// browseFolders answers the picker, opening on path, or on where the netdisk
// opens if it's empty.
func (s *Server) browseFolders(path string) (*folderPage, error) {
	root := s.cfg.MusicDir
	if path == "" {
		path = s.startPath(root)
	}
	path = abs(path)
	p := &folderPage{
		Root:   root,
		Path:   path,
		Parent: filepath.Dir(path),
		AtRoot: path == root,
	}
	if !under(path, root) {
		path = root
		p.Path, p.Parent, p.AtRoot = root, filepath.Dir(root), true
		p.Problem = "That folder is not on the netdisk."
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		p.Problem = "The netdisk is not mounted, or the folder below is empty."
	}
	for _, cr := range crumbs(root, path) {
		p.Crumbs = append(p.Crumbs, folderCrumb{Name: cr.Name, Path: cr.Path})
	}
	p.Added = containsPath(s.local.Folders(), path)
	if p.Problem == "" {
		dirs, err := s.setupDirs(path)
		if err != nil {
			p.Problem = "This folder could not be read. Check that the app may read the netdisk."
		} else {
			p.Dirs = make([]folderDir, len(dirs))
			for i, d := range dirs {
				p.Dirs[i] = folderDir{Name: d.Name, Path: d.Path, Added: d.Added, Symlink: d.Symlink}
			}
		}
	}
	for _, f := range s.local.Folders() {
		p.Folders = append(p.Folders, folderPick{
			Name:  shortPath(f, root),
			Path:  f,
			Whole: filepath.Clean(f) == filepath.Clean(root),
		})
	}
	return p, nil
}

// addFolder lets the library read a folder of the netdisk.
func (s *Server) addFolder(path string) error {
	path = abs(path)
	if !under(path, s.cfg.MusicDir) {
		return errors.New("That folder is not on the netdisk.")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return errors.New("That is not a folder.")
	}
	s.local.AddFolders(path)
	return nil
}
