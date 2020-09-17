package mount

import (
	"bufio"
	"bytes"
	"log"
	"os"
)

// NewNodeTreeFromFile creates a new node tree, with multiple system
// folder mounted, reading from a listing in the file filename.
//
// The file is composed of lines that are empty, begin with #,
// or are of the following format:
//
//    [VFS mount target path]:[System folder path]
//
// A TVFS path does not have colons so the first colon ends the target.
func NewNodeTreeFromFile(filename string) (t *NodeTree, err error) {
	var f *os.File
	f, err = os.Open(filename)
	if err != nil {
		return
	}

	t = NewNodeTree()

	lnum := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		lnum++

		if len(line) == 0 || line[0] == '#' {
			continue
		}

		id := bytes.IndexByte(line, ':')
		if id == -1 {
			log.Printf("mount.NewTreeFile: line %d fromat error (no seperator)", lnum)
			continue
		}

		ls := string(line)
		target := ls[:id]
		folder := ls[id+1:]
		log.Printf(`mount.NewTreeFile: line %d: target="%s", folder="%s"`, lnum, target, folder)

		err := t.Mount(target, &NodeSysFolder{Path: folder})
		if err != nil {
			log.Printf("mount.NewTreeFile: line %d: mount error: %s", lnum, err.Error())
		}
	}

	return
}
