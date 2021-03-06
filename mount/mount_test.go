package mount

import (
	"fmt"
	"io"
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
		t.Errorf("Filesystem mount Read failed: %s", err.Error())
	}

	files, err := root.List("/root")
	if err != nil {
		t.Errorf("Filesystem mount List failed: %s", err.Error())
	}
	fmt.Print(files)

	stat, err := root.Stat("/root/mount_test.go")
	if err != nil {
		t.Errorf("Filesystem mount Stat failed: %s", err.Error())
	}
	fmt.Print(stat)

	//io.Copy(os.Stdout, file)
	if cl, ok := file.(io.Closer); ok {
		cl.Close()
	}

}
