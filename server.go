package ftpd

import (
	"errors"
	"log"
	"net"
	"strconv"
	"time"

	"github.com/Edgaru089/ftpd/auth"
	"github.com/Edgaru089/ftpd/mount"
)

// Server is a FTP Protocol server.
type Server struct {
	// Port is the (control) listening port, defaults to 21.
	Port int
	// Listen address for control connections, defaults to "0.0.0.0" which listens on
	// all IPv4/IPv6 addresses.
	Address string

	// Minimum and maximum data listening port for passive mode.
	// Defaults to 63700-63799.
	MinDataPort, MaxDataPort int
	// Listen address for data connections in passive mode, defaults to "0.0.0.0".
	DataAddress string

	// Root filesystem node. Should not be changed after server start.
	Node mount.Node

	// Simple authenticator. If nil, it defaults to auth.Anonymous.
	Auth auth.Auth

	// Timeout for a passive data connection to wait for. If nil, it defaults
	// to 3s.
	DataConnTimeout time.Duration

	listener *net.TCPListener // control listener

	// for closing the listener, atomic only!!
	close chan struct{}
}

// Start starts a FTP server.
func (s *Server) Start() error {

	if s.MinDataPort > s.MaxDataPort {
		return errors.New("Start: MinDataPort/MaxDataPort not a valid section")
	}

	if s.Port == 0 {
		s.Port = 21
	}
	if s.Address == "" {
		s.Address = "0.0.0.0"
	}
	if s.MinDataPort == 0 || s.MaxDataPort == 0 {
		s.MinDataPort = 63700
		s.MaxDataPort = 63799
	}
	if s.DataAddress == "" {
		s.DataAddress = "0.0.0.0"
	}
	if s.Auth == nil {
		s.Auth = auth.Anonymous{}
	}
	if s.DataConnTimeout == 0 {
		s.DataConnTimeout = time.Second * 3
	}

	laddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(s.Address, strconv.Itoa(s.Port)))
	if err != nil {
		return errors.New("ftpd.Server.Start: TCPAddr resolve error: " + err.Error())
	}
	s.listener, err = net.ListenTCP("tcp", laddr)
	if err != nil {
		return errors.New("ftpd.Server.Start: TCP listen error: " + err.Error())
	}

	log.Printf("ftpd: listening on ctrl %s, data [%s]:[%d-%d]", net.JoinHostPort(s.Address, strconv.Itoa(s.Port)), s.DataAddress, s.MinDataPort, s.MaxDataPort)

	s.close = make(chan struct{})
	go s.goListen()

	return nil
}

func (s *Server) goListen() {
infiloop:
	for {
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			select {
			case <-s.close: // Closed
				break infiloop
			default: // Some random error, log it
				log.Println("ftpd.Listener: listen error:", err)
			}
		}

		log.Print("ftpd.Listener: connected: ", conn.RemoteAddr().String())
		// try sending it!
		go s.goCtrlConn(conn)
	}
}

func (s *Server) Stop() {
	close(s.close)
	s.listener.Close()
}
