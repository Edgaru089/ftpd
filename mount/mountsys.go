package mount

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

// NodeSysFolder is a virtual filesystem node mounted from
// a system filesystem folder.
type NodeSysFolder struct {
	// Path is the system filesystem path.
	Path string
	// NodeName is the name of the folder in the mount. It is not reflected
	// in the virtual filesystem.
	NodeName string
}

var _ Node = &NodeSysFolder{}

func (n *NodeSysFolder) Name() string { return "sysfolder:" + n.NodeName }

func (n *NodeSysFolder) List() (files []File, err error) {
	osfiles, err := ioutil.ReadDir(n.Path)
	if err != nil {
		return nil, err
	}

	files = make([]File, len(osfiles))
	for i, f := range osfiles {
		files[i] = File{
			Name:        f.Name(),
			Size:        f.Size(),
			LastModify:  f.ModTime(),
			IsDirectory: f.IsDir(),
		}
	}

	return
}

func (n *NodeSysFolder) ReadFile(file string) (io.Reader, error) {
	fmt.Println("ReadFile", filepath.Join(n.Path, file))
	return os.Open(filepath.Join(n.Path, file))
}

func (n *NodeSysFolder) WriteFile(file string) (io.Writer, error) {
	return os.Create(filepath.Join(n.Path, file))
}

func (n *NodeSysFolder) AppendFile(file string) (io.Writer, error) {
	return os.OpenFile(filepath.Join(n.Path, file), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0)
}

func (n *NodeSysFolder) DeleteFile(file string) error {
	return os.Remove(filepath.Join(n.Path, file))
}

func (n *NodeSysFolder) MakeDirectory(dir string) error {
	return os.MkdirAll(filepath.Join(n.Path, dir), 0)
}

func (n *NodeSysFolder) RemoveDirectory(dir string) error {
	return os.Remove(filepath.Join(n.Path))
}
