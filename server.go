package ftpd

import (
	"errors"
	"net"

	"github.com/Edgaru089/ftpd/mount"
)

// Server is a FTP Protocol server.
type Server struct {
	// Port is the (control) listening port, defaults to 21.
	Port int
	// Listen address for control connections, defaults to "::", which listens on
	// all IPv4/IPv6 addresses.
	Address string

	// Minimum and maximum data listening port for passive mode.
	// Defaults to 63700-63799.
	MinDataPort, MaxDataPort int
	// Listen address for data connections in passive mode, defaults to "::".
	DataAddress string

	// Root filesystem node. Should not be changed after server start.
	Node mount.NodeTree

	// for closing the listener
	close chan struct{}

	listener *net.TCPListener // control listener
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
		s.Address = "::"
	}
	if s.MinDataPort == 0 || s.MaxDataPort == 0 {
		s.MinDataPort = 63700
		s.MaxDataPort = 63799
	}
	if s.DataAddress == "" {
		s.DataAddress = "::"
	}

	s.listener = net.ListenTCP()

	s.close = make(chan struct{})
}

func (s *Server) goListen() {
infiloop:
	for {
		conn, err := s.listener.AcceptTCP()
		if err != nil {
			switch {
			case <-s.close: // closed
				break infiloop
			default:
			}
		}

		// try sending it!
		go s.goCtrlConn(conn)
	}
}

func (s *Server) Stop() {

}

func (s *Server) goCtrlConn(conn *net.TCPConn) {

}
