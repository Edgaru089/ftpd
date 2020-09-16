package mount

import (
	"fmt"
	"io"
	"os"
	"testing"
)

func TestMount(t *testing.T) {

	n := &NodeSysFolder{NodeName: "Test", Path: "./"}

	root := NewRootNode()

	root.Mount("/213/4325/gfd/", n)
	root.Mount("/213/4325/dfs/", n)
	root.Mount("/213/4325/dpa/", n)

	root.Mount("/root", n)

	((*node)(root)).dump(0)

	fmt.Println(root.Mount("/213/4325/", n))
	fmt.Println(root.Mount("/213/4325/dfs/12", n))
	fmt.Println(root.Mount("/213/4325/dfs", n))

	_, err := root.ReadFile("/213")
	fmt.Println(err)
	_, err = root.WriteFile("/213")
	fmt.Println(err)
	_, err = root.WriteFile("/213/4310")
	fmt.Println(err)
	_, err = root.WriteFile("/213/4325")
	fmt.Println(err)

	file, err := root.ReadFile("/213/4325/gfd/mount_test.go")
	if err != nil {
		t.Errorf("Filesystem mount Read failed: %s", err)
	}

	io.Copy(os.Stdout, file)
	if cl, ok := file.(io.Closer); ok {
		cl.Close()
	}

}
