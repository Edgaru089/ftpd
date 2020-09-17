package ftpd

import (
	"bufio"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"
)

// If state.pasvConn is nil, state.pasvListener must not be nil. This code panics otherwise.
func (s *Server) ensureOpenDataConn(sc *bufio.Scanner, state *ctrlState, writer io.WriteCloser) {
	if state.pasvConn == nil {
		writeFTPReplySingleline(writer, 150)

		// Listen for the Data Connection for some while
		state.pasvListener.SetDeadline(time.Now().Add(s.DataConnTimeout))
		l, err := state.pasvListener.Accept()
		if err != nil {
			//if op, ok := err.(*net.OpError); ok && op.Timeout()
			writeFTPReplySingleline(writer, 426)
			return
		}

		state.pasvConn = l.(*net.TCPConn)

		// Close and dispose the listener (???)
		state.pasvListener.Close()
		state.pasvListener = nil

	} else {
		writeFTPReplySingleline(writer, 125)
	}
	log.Print("openDataConn: connected: ", state.pasvConn.RemoteAddr())
}

// It closes from.
// If state.pasvConn is nil, state.pasvListener must not be nil. This code panics otherwise.
func (s *Server) writeToDataConn(from io.Reader, sc *bufio.Scanner, state *ctrlState, writer io.WriteCloser) {
	s.ensureOpenDataConn(sc, state, writer)

	// We now have a stable data connection, state.pasvConn, to write to.
	log.Print("writeDataConn: starting transfer: ", state.pasvConn.RemoteAddr())
	atomic.StoreInt32(&state.transferError, 0)
	atomic.StoreInt32(&state.inTransfer, 1)
	go func() {
		_, err := io.Copy(state.pasvConn, from)
		if (err == nil || err == io.EOF) && atomic.LoadInt32(&state.transferError) == 0 {
			// Completed without much error, send the okay message
			writeFTPReplySingleline(writer, 226)
		} else {
			log.Print("writeDataConn: error: ", err)
			writeFTPReplySingleline(writer, 426)
		}

		log.Print("writeDataConn: ending transfer: ", state.pasvConn.RemoteAddr())

		// There should be no race condition here
		state.pasvConn.Close()
		state.pasvConn = nil

		if closer, ok := from.(io.Closer); ok {
			closer.Close()
		}

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
	go func() {
		_, err := io.Copy(to, state.pasvConn)
		if (err == nil || err == io.EOF) && atomic.LoadInt32(&state.transferError) == 0 {
			// Completed without much error, send the okay message
			writeFTPReplySingleline(writer, 226)
		} else {
			log.Print("writeDataConn: error: ", err)
			writeFTPReplySingleline(writer, 426)
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
