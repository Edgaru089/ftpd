package ftpd

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"
)

// If state.pasvConn is nil, state.pasvListener must not be nil. This code panics otherwise.
func (s *Server) ensureOpenDataConn(sc *bufio.Scanner, state *ctrlState, writer io.WriteCloser) {
	if state.pasvConn == nil {
		writeFTPReplySingleline(writer, &s.buffer, 150)

		// Listen for the Data Connection for some while
		state.pasvListener.SetDeadline(time.Now().Add(s.DataConnTimeout))
		l, err := state.pasvListener.Accept()
		if err != nil {
			//if op, ok := err.(*net.OpError); ok && op.Timeout()
			writeFTPReplySingleline(writer, &s.buffer, 426)
			return
		}

		state.pasvConn = l.(*net.TCPConn)

		// Close and dispose the listener (???)
		state.pasvListener.Close()
		state.pasvListener = nil
		s.freePort(state.pasvPort)

	} else {
		writeFTPReplySingleline(writer, &s.buffer, 125)
	}
	log.Print("openDataConn: connected: ", state.pasvConn.RemoteAddr())
}

// It closes from.
// If state.pasvConn is nil, state.pasvListener must not be nil. This code panics otherwise.
func (s *Server) writeToDataConn(from io.Reader, sc *bufio.Scanner, state *ctrlState, writer io.WriteCloser) {
	s.ensureOpenDataConn(sc, state, writer)
	if state.pasvConn == nil {
		writer.Close()
	}

	// We now have a stable data connection, state.pasvConn, to write to.
	log.Print("writeDataConn: starting transfer: ", state.pasvConn.RemoteAddr())
	atomic.StoreInt32(&state.transferError, 0)
	atomic.StoreInt32(&state.inTransfer, 1)
	asbuf := &bytes.Buffer{}
	go func() {
		_, err := io.Copy(state.pasvConn, from)
		if (err == nil || err == io.EOF) && atomic.LoadInt32(&state.transferError) == 0 {
			// Completed without much error, send the okay message
			writeFTPReplySingleline(writer, asbuf, 226)
		} else {
			log.Print("writeDataConn: error: ", err)
			writeFTPReplySingleline(writer, asbuf, 426)
		}

		raddr := state.pasvConn.RemoteAddr().String()

		// There should be no race condition here
		state.pasvConn.Close()
		state.pasvConn = nil

		if closer, ok := from.(io.Closer); ok {
			closer.Close()
		}

		log.Print("writeDataConn: ended transfer: ", raddr)

		atomic.StoreInt32(&state.inTransfer, 0)
	}()
}

// It closes to.
// If state.pasvConn is nil, state.pasvListener must not be nil. This code panics otherwise.
// This function is copied from above(writeToDataConn) so keep them in sync please.
func (s *Server) readFromDataConn(to io.Writer, sc *bufio.Scanner, state *ctrlState, writer io.WriteCloser) {
	s.ensureOpenDataConn(sc, state, writer)

	// We now have a stable data connection, state.pasvConn, to write to.
	log.Print("readDataConn: starting transfer: ", state.pasvConn.RemoteAddr())
	atomic.StoreInt32(&state.transferError, 0)
	atomic.StoreInt32(&state.inTransfer, 1)
	asbuf := &bytes.Buffer{}
	go func() {
		_, err := io.Copy(to, state.pasvConn)
		if (err == nil || err == io.EOF) && atomic.LoadInt32(&state.transferError) == 0 {
			// Completed without much error, send the okay message
			writeFTPReplySingleline(writer, asbuf, 226)
		} else {
			log.Print("readDataConn: error: ", err)
			writeFTPReplySingleline(writer, asbuf, 426)
		}

		log.Print("readDataConn: ending transfer: ", state.pasvConn.RemoteAddr())

		// There should be no race condition here
		state.pasvConn.Close()
		state.pasvConn = nil

		if closer, ok := to.(io.Closer); ok {
			closer.Close()
		}

		atomic.StoreInt32(&state.inTransfer, 0)
	}()

}
