// Package terminal implements DEADPORT's fake shell: an in-memory filesystem
// plus a small command interpreter. Nothing in this package touches the host
// filesystem or executes host processes - every "file" is a Go string and every
// "command" is a Go function.
package terminal

import (
	"errors"
	"path"
	"sort"
	"strings"
)

// Errors returned by filesystem lookups. Messages are user-facing.
var (
	ErrNotFound = errors.New("No such file or directory")
	ErrNotDir   = errors.New("Not a directory")
	ErrIsDir    = errors.New("Is a directory")
)

// Node is a file or directory in the simulated filesystem.
type Node struct {
	Name     string
	Dir      bool
	Content  string
	children map[string]*Node
}

// FS is an in-memory, read-only (to players) filesystem tree.
// An FS is not mutated after construction, so it may be shared.
type FS struct {
	root *Node
}

// NewFS returns an empty filesystem containing only "/".
func NewFS() *FS {
	return &FS{root: &Node{Name: "/", Dir: true, children: map[string]*Node{}}}
}

// Clean turns any player-supplied path into a canonical absolute path inside
// the fake root. ".." can never climb above "/", so path traversal is
// impossible by construction.
func Clean(cwd, p string) string {
	if cwd == "" {
		cwd = "/"
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = cwd + "/" + p
	}
	c := path.Clean("/" + p)
	if !strings.HasPrefix(c, "/") {
		return "/"
	}
	return c
}

func splitPath(abs string) []string {
	abs = strings.Trim(abs, "/")
	if abs == "" {
		return nil
	}
	return strings.Split(abs, "/")
}

// MkdirAll creates a directory and its parents.
func (fs *FS) MkdirAll(abs string) (*Node, error) {
	n := fs.root
	for _, part := range splitPath(Clean("/", abs)) {
		child, ok := n.children[part]
		if !ok {
			child = &Node{Name: part, Dir: true, children: map[string]*Node{}}
			n.children[part] = child
		} else if !child.Dir {
			return nil, ErrNotDir
		}
		n = child
	}
	return n, nil
}

// WriteFile creates (or replaces) a file, creating parent directories.
func (fs *FS) WriteFile(abs, content string) error {
	abs = Clean("/", abs)
	if abs == "/" {
		return ErrIsDir
	}
	parent, err := fs.MkdirAll(path.Dir(abs))
	if err != nil {
		return err
	}
	name := path.Base(abs)
	if existing, ok := parent.children[name]; ok && existing.Dir {
		return ErrIsDir
	}
	parent.children[name] = &Node{Name: name, Content: content}
	return nil
}

// Lookup returns the node at an absolute path.
func (fs *FS) Lookup(abs string) (*Node, error) {
	n := fs.root
	for _, part := range splitPath(Clean("/", abs)) {
		if !n.Dir {
			return nil, ErrNotDir
		}
		child, ok := n.children[part]
		if !ok {
			return nil, ErrNotFound
		}
		n = child
	}
	return n, nil
}

// ReadFile returns the content of a file.
func (fs *FS) ReadFile(abs string) (string, error) {
	n, err := fs.Lookup(abs)
	if err != nil {
		return "", err
	}
	if n.Dir {
		return "", ErrIsDir
	}
	return n.Content, nil
}

// Children returns a directory's entries sorted by name.
func (n *Node) Children() []*Node {
	out := make([]*Node, 0, len(n.children))
	for _, c := range n.children {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Walk visits every node below (and including) abs in sorted, depth-first
// order. fn receives the absolute path of each node.
func (fs *FS) Walk(abs string, fn func(p string, n *Node)) error {
	start, err := fs.Lookup(abs)
	if err != nil {
		return err
	}
	var walk func(p string, n *Node)
	walk = func(p string, n *Node) {
		fn(p, n)
		if !n.Dir {
			return
		}
		for _, c := range n.Children() {
			walk(path.Join(p, c.Name), c)
		}
	}
	walk(Clean("/", abs), start)
	return nil
}

// Exists reports whether a path exists.
func (fs *FS) Exists(abs string) bool {
	_, err := fs.Lookup(abs)
	return err == nil
}

// FileCount returns the number of regular files in the tree.
func (fs *FS) FileCount() int {
	count := 0
	_ = fs.Walk("/", func(_ string, n *Node) {
		if !n.Dir {
			count++
		}
	})
	return count
}
