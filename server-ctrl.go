package ftpd

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Edgaru089/ftpd/auth"
	"github.com/Edgaru089/ftpd/mount"
)

// This file houses the protocol implementation for the Control connection.

// Data transfer types
const (
	DataASCII = iota
	DataImage
	DataEBCDIC // Not supported
)

const (
	DataStream     = iota
	DataBlock      // Not supported
	DataCompressed // Not supported
)

const (
	DataConnActive = iota // Not supported
	DataConnPassive
)

// FTP control connections are stateful!
type ctrlState struct {
	auth     auth.AccessType // current auth level (zero-value means no permission)
	username string          // only store the username, verified on (USER or) PASS command
	wd       string          // working directory

	datatype int // ASCII, Image or EBCDIC(not implemented)
	//datamode int // Stream, Block or Compress(not implemented)

	activePort, pasvPort int              // Active(client-side) and Passive(server-side) ports
	activeIP             net.IP           // Active mode target IP
	dataConnMode         int              // Data Connect mode (Active or Passive)
	pasvListener         *net.TCPListener // Passive mode TCP Listener, nil if none
	pasvConn             *net.TCPConn     // Passive mode Data Connection

	inTransfer    int32 // 0 or 1, Must be read/written by the atomic package!!!
	transferError int32 // 0(no error) or 1(error), Must be atomic!!!
}

var defaultCtrlState = ctrlState{
	wd: "/",
}

// The conn can be any ReadWriteCloser (can be the ones from net or crypto/tls).
func (s *Server) goCtrlConn(conn io.ReadWriteCloser) {
	defer func() {
		err := recover()
		if err != nil {
			stack := make([]byte, 1024)
			stack = stack[:runtime.Stack(stack, false)]
			log.Print("goCtrlConn: panic: ", err, ", stack trace:\n", string(stack))
		}
	}()

	// recover goes after conn.Close
	defer conn.Close()

	// Hello!
	writeFTPReplySingleline(conn, 220)

	// The FTP protocol is strictly Telnet(CRLF) based so
	// we could just use bufio.Scanner with CRLF ending

	sc := bufio.NewScanner(conn)
	sc.Split(ScanCRLF)

	// FTP controls are stateful!
	state := defaultCtrlState
	defer func() { // State cleanup
		if state.pasvListener != nil {
			state.pasvListener.Close()
		}
		if state.pasvConn != nil {
			state.pasvConn.Close()
		}
	}()

	for sc.Scan() {
		nline := sc.Bytes()
		s.doCtrlLine(nline, sc, &state, conn)
	}
}

func (s *Server) doCtrlLine(line []byte, sc *bufio.Scanner, state *ctrlState, writer io.WriteCloser) {

	// Read the first word ended either by Space or CRLF
	var cmd []byte
	for i, c := range line {
		if c == ' ' {
			break
		}
		cmd = line[:i+1]
	}

	log.Printf("doLine: Line=\"%s\", Cmd=%s\n", line, cmd)

	switch string(bytes.ToUpper(cmd)) {

	// ----- ACCESS CONTROL COMMANDS ----- //

	case "USER":
		// param should begin after the command and a Space
		param := string(line[len(cmd)+1:])

		// Reset the auth level
		state.auth = s.Auth.Login(param, "")
		if state.auth != auth.NoPermission {
			// Success
			writeFTPReplySingleline(writer, 230)
		} else {
			state.username = param
			writeFTPReplySingleline(writer, 331)
		}
	case "PASS":
		if len(state.username) == 0 { // Invalid: PASS comes after USER
			writeFTPReplySingleline(writer, 503)
			break
		}

		var param string
		if len(cmd) == len(line) {
			param = "\uffefINVALID\uffef"
		} else {
			param = string(line[len(cmd)+1:])
		}

		state.auth = s.Auth.Login(state.username, param)
		if state.auth != auth.NoPermission {
			writeFTPReplySingleline(writer, 230)
		} else {
			state.username = param
			writeFTPReplySingleline(writer, 530)
		}
	case "CWD":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := string(line[len(cmd)+1:])
		var target string
		if param[0] == '/' {
			target = param
		} else {
			target = state.wd + "/" + param
		}
		stat, err := s.Node.Stat(target)
		if target == "/" || (err == nil && stat.IsDirectory) { // A folder
			state.wd = target
			if state.wd != "/" && state.wd[len(state.wd)-1] == '/' {
				state.wd = state.wd[:len(state.wd)-1]
			}
			writeFTPReplySingleline(writer, 200)
		} else {
			log.Print("doLine: warning: CWD target folder \"", target, "\" Stat failed")
			writeFTPReplySingleline(writer, 501)
		}
	case "PWD":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		writeFTPReplySingleline(writer, 257, state.wd)
	case "CDUP":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		// TODO CDUP called on root directory
		if state.wd == "/" {
			writeFTPReplySingleline(writer, 550)
			break
		}
		newpath := state.wd[:strings.LastIndexByte(state.wd, '/')]
		if len(newpath) == 0 {
			newpath = "/"
		}
		stat, err := s.Node.Stat(newpath)
		if err != nil || !stat.IsDirectory {
			log.Print("doLine: warning: CDUP folder \"", state.wd, "\" -> \"", newpath, "\" Stat failed")
			writeFTPReplySingleline(writer, 550)
		} else {
			//log.Print("doLine: CDUP folder \"", state.wd, "\" -> \"", newpath, "\"")
			state.wd = newpath
			writeFTPReplySingleline(writer, 200)
		}
	case "REIN":
		(*state) = defaultCtrlState
		writeFTPReplySingleline(writer, 200)
	case "QUIT":
		writeFTPReplySingleline(writer, 221)
		writer.Close()

	// ----- TRANSFER PARAMETER COMMANDS ----- //

	case "PORT":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		state.activeIP, state.activePort = parseHostPort(param)
		state.dataConnMode = DataConnActive
		writeFTPReplySingleline(writer, 200)
	case "PASV":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		// Close previous
		if state.pasvListener != nil {
			state.pasvListener.Close()
		}

		// target listen data address
		addr := s.DataAddress
		if len(addr) == 0 || addr == "0.0.0.0" {
			if conn, ok := writer.(*net.TCPConn); ok {
				// Ignoring error
				addr, _, _ = net.SplitHostPort(conn.LocalAddr().String())
			}
		}

		// TODO Listen in specified port range instead of random
		l, err := net.Listen("tcp4", net.JoinHostPort(addr, "0"))
		state.pasvListener = l.(*net.TCPListener)
		if err != nil {
			writeFTPReplySingleline(writer, 421)
			writer.Close()
			log.Print("doCtrlLine: listen error: ", err)
			break
		}

		_, pasvStr, err := net.SplitHostPort(state.pasvListener.Addr().String())
		if err != nil {
			writeFTPReplySingleline(writer, 421)
			writer.Close()
			break
		}
		pasvPort, err := strconv.Atoi(pasvStr)
		if err != nil {
			writeFTPReplySingleline(writer, 421)
			writer.Close()
			break
		}
		writeFTPReplySingleline(writer, 227, packHostPortSlice(net.ParseIP(s.DataAddress), pasvPort))
	case "TYPE":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		switch string(param) {
		case "A":
			state.datatype = DataASCII
			writeFTPReplySingleline(writer, 200)
		case "I":
			state.datatype = DataImage
			writeFTPReplySingleline(writer, 200)
		case "E":
			writeFTPReplySingleline(writer, 504)
		default:
			writeFTPReplySingleline(writer, 501)
		}
	case "STRU":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		switch string(param) {
		case "F":
			writeFTPReplySingleline(writer, 200)
		case "R", "P":
			writeFTPReplySingleline(writer, 504)
		default:
			writeFTPReplySingleline(writer, 501)
		}
	case "MODE":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		switch string(param) {
		case "S":
			//state.datamode = DataStream
			writeFTPReplySingleline(writer, 200)
		case "B", "C":
			writeFTPReplySingleline(writer, 504)
		default:
			writeFTPReplySingleline(writer, 501)
		}

	// ----- FTP SERVICE COMMANDS ----- //

	case "ABOR":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		if state.pasvConn == nil {
			writeFTPReplySingleline(writer, 226)
			break
		}
		if atomic.LoadInt32(&state.inTransfer) != 0 {
			atomic.StoreInt32(&state.transferError, 1)
			state.pasvConn.Close()
			// Actively wait for the data connection to finish.
			for atomic.LoadInt32(&state.inTransfer) != 0 {
				//runtime.Gosched()
				time.Sleep(time.Millisecond)
			}
		} else {
			state.pasvConn.Close()
			state.pasvConn = nil
		}
		writeFTPReplySingleline(writer, 226)

	// TODO Active Mode Data Connection
	case "RETR":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		f, err := s.Node.ReadFile(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, 550)
		} else {
			s.writeToDataConn(f, sc, state, writer)
		}
	case "STOR":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		f, err := s.Node.WriteFile(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, 550)
		} else {
			s.readFromDataConn(f, sc, state, writer)
		}
	case "APPE":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		f, err := s.Node.AppendFile(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, 550)
		} else {
			s.readFromDataConn(f, sc, state, writer)
		}
	case "DELE":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		err := s.Node.DeleteFile(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, 550)
		} else {
			writeFTPReplySingleline(writer, 200)
		}
	case "RMD":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		err := s.Node.RemoveDirectory(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, 550)
		} else {
			writeFTPReplySingleline(writer, 200)
		}
	case "MKD":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := line[len(cmd)+1:]
		err := s.Node.MakeDirectory(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, 550)
		} else {
			writeFTPReplySingleline(writer, 200)
		}

	// ----- RFC3659 EXTENSION COMMANDS ----- //

	case "SIZE":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := state.wd + "/" + string(line[len(cmd)+1:])
		stat, err := s.Node.Stat(param)
		if err != nil {
			writeFTPReplySingleline(writer, 550)
		} else {
			writeFTPReplySingleline(writer, 213, strconv.FormatInt(stat.Size, 10))
		}
	case "MDTM":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		param := state.wd + "/" + string(line[len(cmd)+1:])
		stat, err := s.Node.Stat(param)
		if err != nil {
			writeFTPReplySingleline(writer, 550)
		} else {
			writeFTPReplySingleline(writer, 213, ftpTime(stat.LastModify))
		}
	case "MLST":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		var param string
		if len(line) == len(cmd) {
			param = state.wd
		} else {
			param = state.wd + "/" + string(line[len(cmd)+1:])
		}
		stat, err := s.Node.Stat(param)
		if err != nil {
			writeFTPReplySingleline(writer, 550)
			break
		}
		var buf bytes.Buffer
		buf.WriteString("250- Listing starting\r\n ")
		formatMLSXString(&buf, &stat)
		buf.WriteString("\r\n250 End\r\n")
		buf.WriteTo(writer)
	case "MLSD":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, 530)
			break
		}
		var param string
		if len(line) == len(cmd) {
			param = state.wd
		} else {
			param = state.wd + "/" + string(line[len(cmd)+1:])
		}
		list, err := s.Node.List(param)
		if err != nil {
			if err == mount.ErrNotFolder {
				writeFTPReplySingleline(writer, 501)
			} else {
				writeFTPReplySingleline(writer, 550)
			}
			break
		}

		s.writeToDataConn(newMLSDWriter(list), sc, state, writer)

	// ----- OTHER EXTENSION COMMANDS ----- //

	case "FEAT":
		var buf bytes.Buffer
		buf.WriteString("211- Features supported\r\n")
		buf.Write(Features)
		buf.WriteString("211 End\r\n")
		_, err := buf.WriteTo(writer)
		if err != nil {
			writer.Close()
		}

	case "ALLO", "NOOP":
		writeFTPReplySingleline(writer, 200)
	case "ACCT", "STOU", "REST", "LIST", "NLST", "SITE", "SYST", "STAT":
		writeFTPReplySingleline(writer, 502) // Command not Implemented
	default:
		writeFTPReplySingleline(writer, 500)
	}
}

type mlsdWriter struct {
	files []mount.File
	bytes.Buffer
}

func newMLSDWriter(files []mount.File) io.Reader {
	o := &mlsdWriter{files: files}
	for _, f := range files {
		formatMLSXString(o, &f)
		o.WriteString("\r\n")
	}
	return o
}
