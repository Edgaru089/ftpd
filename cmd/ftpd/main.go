package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/Edgaru089/ftpd"
	"github.com/Edgaru089/ftpd/mount"
)

func main() {

	var dir, ctrladdr, dataaddr string
	var port int
	flag.StringVar(&dir, "dir", ".", "root directory")
	flag.IntVar(&port, "port", 21, "Control FTP Port")
	flag.StringVar(&ctrladdr, "ctrl-addr", "0.0.0.0", "Control listen address")
	flag.StringVar(&dataaddr, "data-addr", "0.0.0.0", "Data listen address")
	flag.Parse()

	tree := mount.NewNodeTree()
	tree.Mount("dir", &mount.NodeSysFolder{Path: dir})
	tree.Mount("wd", &mount.NodeSysFolder{Path: "."})

	s := &ftpd.Server{
		Node:        tree,
		Port:        port,
		Address:     ctrladdr,
		DataAddress: dataaddr,
	}

	err := s.Start()
	if err != nil {
		log.Fatal("ftpd start error: ", err)
	}

	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt)
	<-ch

	s.Stop()

}
