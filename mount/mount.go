package mount

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type File struct {
	Name        string
	Size        int64
	LastModify  time.Time
	IsDirectory bool // True for a directory, false for a file
}

var (
	ErrFileNotFound = os.ErrNotExist
	ErrFileFound    = os.ErrExist
	ErrNoPermission = os.ErrPermission
	ErrNotFolder    = errors.New("listing a non-folder")
	ErrOther        = errors.New("unknown error")
)

// Node is a node (folder) in the virtual filesystem.
// The file/dir parameters are relative paths, but with no "./"
// at the beginning, or other relative marks like "..".
type Node interface {
	// Name returns the name to be printed for a human reader to identify.
	Name() string
	List(folder string) ([]File, error) // List lists the files under the node.
	Stat(file string) (File, error)     // Stat stats a single file under the node.

	// ReadFile returns a io.Reader created by reading the file
	// under this directory, possibly transversing through other
	// sub-directories.
	//
	// If the returned Reader satifies io.Closer, the file is
	// closed after use.
	ReadFile(file string) (io.Reader, error)

	// WriteFile returns a io.Writer that stores the file under
	// this directory, rewriting existing one if present.
	//
	// If the returned Writer satifies io.Closer, the file is
	// closed after use.
	WriteFile(file string) (io.Writer, error)

	// AppendFile returns a io.Writer that stores the file under
	// this directory, appending existing one if present.
	//
	// If the returned Writer satifies io.Closer, the file is
	// closed after use.
	AppendFile(file string) (io.Writer, error)

	// DeleteFile deletes a file at the given location, but
	// does nothing if the file is a directory.
	DeleteFile(file string) error

	// MakeDirectory makes a new directory (or directories) under
	// this node.
	MakeDirectory(dir string) error

	// RemoveDirectory deletes a directory at the given location, but
	// does nothing if the file is not a directory.
	RemoveDirectory(dir string) error
}

type MountErrType int

const (
	MountErrorLeafHasChild MountErrType = iota
	MountErrorPathHasLeaf
	MountErrorLeafExists
	MountErrorRootIsNil
)

var MountErrMsg = [...]string{
	MountErrorLeafHasChild: "mount leaf not empty",
	MountErrorPathHasLeaf:  "mount path contains leaf",
	MountErrorLeafExists:   "mount leaf exists",
	MountErrorRootIsNil:    "mount called with (root *node) == nil",
}

// MountError indicates errors arising when mounting a node.
type MountError struct {
	MountPath, NodeName string
	Type                MountErrType
}

func (err *MountError) Error() string {
	return fmt.Sprintf("error mounting \"%s\" onto %s: %s", err.NodeName, err.MountPath, MountErrMsg[err.Type])
}

// unexported Node is the wrapper for a tree strcture with exported
// Node-s at the leaves
type node struct {
	// Lock this please when visiting children but unlock it before recursing
	sync.RWMutex

	completePath string // the complete path of the node, with / at the beginning and the end

	node   Node             // a node
	ch     map[string]*node // or children
	parent *node            // a parent, nil for root
}

// NodeTree is a root node for containing a mount tree hierachy.
// It should also satsify Node, but please don't mount it onto another tree.
// Weird things might happen.
type NodeTree node

func NewRootNode() *NodeTree {
	return &NodeTree{completePath: "/"}
}

func stripSlash(path string) string {
	if path == "/" {
		return ""
	}
	if path[0] == '/' {
		path = path[1:]
	}
	if path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	return path
}

func (root *NodeTree) walk(path string) *node {
	dirs := strings.Split(stripSlash(path), "/")
	fmt.Println("mount:", stripSlash(path), "dirs:", dirs, "len:", len(dirs))

	cur := (*node)(root)
	for _, str := range dirs {
		if len(str) == 0 {
			continue
		}
		fmt.Println("walking past", str)

		cur.RLock()

		if cur.ch == nil || cur.ch[str] == nil {
			cur.RUnlock()
			return nil
		}

		prev := cur
		cur = cur.ch[str]
		prev.RUnlock()
		cur.RLock()

		// has a node
		if cur.node != nil {
			cur.RUnlock()
			fmt.Printf("Returning %s (%v)", cur.completePath, cur)
			return cur
		}

		cur.RUnlock()
	}

	return cur
}

// Mount mounts the node at the path in the VFS rooted at root.
func (root *NodeTree) Mount(path string, n Node) error {
	if root == nil {
		return &MountError{MountPath: path, NodeName: n.Name(), Type: MountErrorRootIsNil}
	}

	dirs := strings.Split(stripSlash(path), "/")
	fmt.Println("mount:", stripSlash(path), "dirs:", dirs, "len:", len(dirs))

	cur := (*node)(root)
	for _, str := range dirs {
		if len(str) == 0 {
			continue
		}
		fmt.Println("walking past", str)

		cur.Lock()
		// has a node
		if cur.node != nil {
			cur.Unlock()
			return &MountError{MountPath: path, NodeName: n.Name(), Type: MountErrorPathHasLeaf}
		}

		if cur.ch == nil {
			cur.ch = make(map[string]*node)
		}

		// add the child if not there
		if cur.ch[str] == nil {
			cur.ch[str] = &node{
				completePath: cur.completePath + str + "/",
				parent:       cur,
			}
		}

		prev := cur
		cur = cur.ch[str]
		prev.Unlock()
	}

	fmt.Println("walk done, cur.Path =", cur.completePath)

	switch {
	case cur.ch != nil:
		return &MountError{MountPath: path, NodeName: n.Name(), Type: MountErrorLeafHasChild}
	case cur.node != nil:
		return &MountError{MountPath: path, NodeName: n.Name(), Type: MountErrorLeafExists}
	}

	cur.node = n

	return nil
}

// NodeTree (should) satsify Node
var _ Node = &NodeTree{}

func (*NodeTree) Name() string {
	return "nodetree"
}

func (n *NodeTree) List(folder string) (files []File, err error) {
	if folder[0] != '/' {
		folder = "/" + folder
	}
	if folder[len(folder)-1] != '/' {
		folder = folder + "/"
	}

	node := n.walk(folder)
	node.RLock()
	// Oops! defer executes after return
	//defer node.RUnlock()

	if node.node != nil {
		node.RUnlock()
		return node.node.List(folder[len(node.completePath):])
	}

	// We can defer now!
	defer node.RUnlock()

	// we have only folders in a virtual filesystem...
	files = make([]File, len(node.ch))
	i := 0
	for name := range node.ch {
		files[i] = File{
			IsDirectory: true,
			Name:        name,
		}
		i++
	}
	return
}

func (n *NodeTree) Stat(file string) (File, error) {
	node := n.walk(file)
	if node == nil || file[:len(node.completePath)] != node.completePath {
		return File{}, ErrFileNotFound
	}
	if node.node == nil {
		id := strings.LastIndexByte(file, '/')
		if id == -1 {
			return File{
				Name:        file,
				IsDirectory: true,
			}, nil
		} else {
			return File{
				Name:        file[id:],
				IsDirectory: true,
			}, nil
		}
	}
	return node.node.Stat(file[len(node.completePath):])
}

func (n *NodeTree) ReadFile(file string) (io.Reader, error) {
	fmt.Println("node.ReadFile", file)
	node := n.walk(file)
	if node == nil || node.node == nil || file[:len(node.completePath)] != node.completePath {
		return nil, ErrFileNotFound
	}

	fmt.Println("path =", node.completePath, " subpath =", file[len(node.completePath):])
	return node.node.ReadFile(file[len(node.completePath):])
}

func (n *NodeTree) WriteFile(file string) (io.Writer, error) {
	node := n.walk(file)
	if node == nil || node.node == nil || file[:len(node.completePath)] != n.completePath {
		return nil, ErrFileNotFound
	}
	return node.node.WriteFile(file[len(node.completePath):])
}
func (n *NodeTree) AppendFile(file string) (io.Writer, error) {
	node := n.walk(file)
	if node == nil || node.node == nil || file[:len(node.completePath)] != node.completePath {
		return nil, ErrFileNotFound
	}
	return node.node.AppendFile(file[len(node.completePath):])
}
func (n *NodeTree) DeleteFile(file string) error {
	node := n.walk(file)
	if node == nil || node.node == nil || file[:len(node.completePath)] != node.completePath {
		return ErrFileNotFound
	}
	return node.node.DeleteFile(file[len(node.completePath):])
}
func (n *NodeTree) MakeDirectory(dir string) error {
	node := n.walk(dir)
	if node == nil || node.node == nil || dir[:len(node.completePath)] != node.completePath {
		return ErrFileNotFound
	}
	return node.node.MakeDirectory(dir[len(node.completePath):])
}
func (n *NodeTree) RemoveDirectory(dir string) error {
	node := n.walk(dir)
	if node == nil || node.node == nil || dir[:len(node.completePath)] != node.completePath {
		return ErrFileNotFound
	}
	return node.node.RemoveDirectory(dir[len(node.completePath):])
}
