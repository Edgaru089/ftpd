package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/Edgaru089/ftpd"
	"github.com/Edgaru089/ftpd/auth"
	"github.com/Edgaru089/ftpd/mount"
)

func main() {

	var dir, ctrladdr, dataaddr, authfile, mountfile string
	var port int
	flag.StringVar(&dir, "dir", ".", "root directory")
	flag.IntVar(&port, "port", 21, "Control FTP Port")
	flag.StringVar(&ctrladdr, "ctrl-addr", "0.0.0.0", "Control listen address")
	flag.StringVar(&dataaddr, "data-addr", "0.0.0.0", "Data listen address")
	flag.StringVar(&authfile, "auth-file", "", "auth file path, Anonymous if not present")
	flag.StringVar(&mountfile, "mount-file", "", "mount file path, mounts working directory at root if not present")
	flag.Parse()

	s := &ftpd.Server{
		Port:        port,
		Address:     ctrladdr,
		DataAddress: dataaddr,
	}

	if len(authfile) != 0 {
		// Ignore error since s.Auth is nil this case, which defaults to anonymous
		s.Auth, _ = auth.NewFile(authfile)
	}

	if len(mountfile) == 0 {
		s.Node = &mount.NodeSysFolder{Path: "."}
	} else {
		var err error
		s.Node, err = mount.NewNodeTreeFromFile(mountfile)
		if err != nil {
			s.Node = &mount.NodeSysFolder{Path: "."}
		}
	}

	err := s.Start()
	if err != nil {
		log.Fatal("ftpd start error: ", err)
	}

	ch := make(chan os.Signal)
	signal.Notify(ch, os.Interrupt)
	<-ch

	s.Stop()

	log.Print("A graceful shutdown. Thank you.")

}
