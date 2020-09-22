package ftpd

import (
	"bufio"
	"bytes"
	"fmt"
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
			stack := make([]byte, 8192)
			stack = stack[:runtime.Stack(stack, false)]
			log.Print("goCtrlConn: panic: ", err, ", stack trace:\n", string(stack))
		}
	}()

	// recover goes after conn.Close
	defer conn.Close()

	// Hello!
	writeFTPReplySingleline(conn, &s.buffer, 220)

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
	// Reuse the server buffer
	buf := &s.buffer
	buf.Reset()

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
			writeFTPReplySingleline(writer, buf, 230)
		} else {
			state.username = param
			writeFTPReplySingleline(writer, buf, 331)
		}
	case "PASS":
		if len(state.username) == 0 { // Invalid: PASS comes after USER
			writeFTPReplySingleline(writer, buf, 503)
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
			writeFTPReplySingleline(writer, buf, 230)
		} else {
			state.username = param
			writeFTPReplySingleline(writer, buf, 530)
		}
	case "CWD":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := string(line[len(cmd)+1:])
		var target string
		if param[0] == '/' {
			target = param
		} else {
			if state.wd == "/" {
				target = "/" + param
			} else {
				target = state.wd + "/" + param
			}
		}
		stat, err := s.Node.Stat(target)
		if target == "/" || (err == nil && stat.IsDirectory) { // A folder
			state.wd = target
			if state.wd != "/" && state.wd[len(state.wd)-1] == '/' {
				state.wd = state.wd[:len(state.wd)-1]
			}
			writeFTPReplySingleline(writer, buf, 200)
		} else {
			log.Print("doLine: warning: CWD target folder \"", target, "\" Stat failed")
			writeFTPReplySingleline(writer, buf, 501)
		}
	case "PWD":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		writeFTPReplySingleline(writer, buf, 257, state.wd)
	case "CDUP":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		// TODO CDUP called on root directory
		if state.wd == "/" {
			writeFTPReplySingleline(writer, buf, 550)
			break
		}
		newpath := state.wd[:strings.LastIndexByte(state.wd, '/')]
		if len(newpath) == 0 {
			newpath = "/"
		}
		stat, err := s.Node.Stat(newpath)
		if err != nil || !stat.IsDirectory {
			log.Print("doLine: warning: CDUP folder \"", state.wd, "\" -> \"", newpath, "\" Stat failed")
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			//log.Print("doLine: CDUP folder \"", state.wd, "\" -> \"", newpath, "\"")
			state.wd = newpath
			writeFTPReplySingleline(writer, buf, 200)
		}
	case "REIN":
		(*state) = defaultCtrlState
		writeFTPReplySingleline(writer, buf, 200)
	case "QUIT":
		writeFTPReplySingleline(writer, buf, 221)
		writer.Close()

	// ----- TRANSFER PARAMETER COMMANDS ----- //

	case "PORT":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		state.activeIP, state.activePort = parseHostPort(param)
		state.dataConnMode = DataConnActive
		writeFTPReplySingleline(writer, buf, 200)
	case "PASV":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
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

		dport := s.alloPort()
		if dport == 0 {
			writeFTPReplySingleline(writer, buf, 421)
			break
		}
		state.pasvPort = dport

		l, err := net.Listen("tcp4", net.JoinHostPort(addr, strconv.Itoa(dport)))
		state.pasvListener = l.(*net.TCPListener)
		if err != nil {
			writeFTPReplySingleline(writer, buf, 421)
			log.Print("doCtrlLine: listen error: ", err)
			break
		}

		_, pasvStr, err := net.SplitHostPort(state.pasvListener.Addr().String())
		if err != nil {
			writeFTPReplySingleline(writer, buf, 421)
			break
		}
		pasvPort, err := strconv.Atoi(pasvStr)
		if err != nil {
			writeFTPReplySingleline(writer, buf, 421)
			break
		}
		writeFTPReplySingleline(writer, buf, 227, packHostPortSlice(net.ParseIP(s.DataAddress), pasvPort))
	case "TYPE":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		switch string(param) {
		case "A":
			state.datatype = DataASCII
			writeFTPReplySingleline(writer, buf, 200)
		case "I":
			state.datatype = DataImage
			writeFTPReplySingleline(writer, buf, 200)
		case "E":
			writeFTPReplySingleline(writer, buf, 504)
		default:
			writeFTPReplySingleline(writer, buf, 501)
		}
	case "STRU":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		switch string(param) {
		case "F":
			writeFTPReplySingleline(writer, buf, 200)
		case "R", "P":
			writeFTPReplySingleline(writer, buf, 504)
		default:
			writeFTPReplySingleline(writer, buf, 501)
		}
	case "MODE":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		switch string(param) {
		case "S":
			//state.datamode = DataStream
			writeFTPReplySingleline(writer, buf, 200)
		case "B", "C":
			writeFTPReplySingleline(writer, buf, 504)
		default:
			writeFTPReplySingleline(writer, buf, 501)
		}

	// ----- FTP SERVICE COMMANDS ----- //

	case "ABOR":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		if state.pasvConn == nil {
			writeFTPReplySingleline(writer, buf, 226)
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
		writeFTPReplySingleline(writer, buf, 226)

	// TODO Active Mode Data Connection
	case "RETR":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		f, err := s.Node.ReadFile(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			s.writeToDataConn(f, sc, state, writer)
		}
	case "STOR":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		f, err := s.Node.WriteFile(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			s.readFromDataConn(f, sc, state, writer)
		}
	case "APPE":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		f, err := s.Node.AppendFile(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			s.readFromDataConn(f, sc, state, writer)
		}
	case "DELE":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		err := s.Node.DeleteFile(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			writeFTPReplySingleline(writer, buf, 200)
		}
	case "RMD":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		err := s.Node.RemoveDirectory(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			writeFTPReplySingleline(writer, buf, 200)
		}
	case "MKD":
		if !state.auth.HasAccess(auth.ReadWrite) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := line[len(cmd)+1:]
		err := s.Node.MakeDirectory(state.wd + "/" + string(param))
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			writeFTPReplySingleline(writer, buf, 200)
		}

	// ----- RFC3659 EXTENSION COMMANDS ----- //

	case "SIZE":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := state.wd + "/" + string(line[len(cmd)+1:])
		if state.wd == "/" { // param="//dir"
			param = param[1:]
		}
		stat, err := s.Node.Stat(param)
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			writeFTPReplySingleline(writer, buf, 213, strconv.FormatInt(stat.Size, 10))
		}
	case "MDTM":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		param := state.wd + "/" + string(line[len(cmd)+1:])
		if state.wd == "/" { // param="//dir"
			param = param[1:]
		}
		stat, err := s.Node.Stat(param)
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
		} else {
			writeFTPReplySingleline(writer, buf, 213, ftpTime(stat.LastModify))
		}
	case "MLST":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		var param string
		if len(line) == len(cmd) {
			param = state.wd
		} else {
			param = state.wd + "/" + string(line[len(cmd)+1:])
			if state.wd == "/" { // param="//dir"
				param = param[1:]
			}
		}
		stat, err := s.Node.Stat(param)
		if err != nil {
			writeFTPReplySingleline(writer, buf, 550)
			break
		}
		buf.WriteString("250- Listing starting\r\n ")
		formatMLSXString(buf, &stat)
		buf.WriteString("\r\n250 End\r\n")
		buf.WriteTo(writer)
	case "MLSD":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		var param string
		if len(line) == len(cmd) {
			param = state.wd
		} else {
			param = state.wd + "/" + string(line[len(cmd)+1:])
			if state.wd == "/" { // param="//dir"
				param = param[1:]
			}
		}
		list, err := s.Node.List(param)
		if err != nil {
			if err == mount.ErrNotFolder {
				writeFTPReplySingleline(writer, buf, 501)
			} else {
				writeFTPReplySingleline(writer, buf, 550)
			}
			break
		}

		s.writeToDataConn(newMLSDWriter(list), sc, state, writer)

	case "LIST":
		if !state.auth.HasAccess(auth.ReadOnly) {
			writeFTPReplySingleline(writer, buf, 530)
			break
		}
		list, err := s.Node.List(state.wd)
		if err != nil {
			if err == mount.ErrNotFolder {
				writeFTPReplySingleline(writer, buf, 501)
			} else {
				writeFTPReplySingleline(writer, buf, 550)
			}
			break
		}

		var permstr string
		switch state.auth {
		case auth.ReadOnly:
			permstr = "r--r--r--"
		case auth.ReadWrite:
			permstr = "rw-rw-rw-"
		}

		// We need a new buffer for async send
		buf := &bytes.Buffer{}
		for _, f := range list {
			if f.IsDirectory {
				buf.WriteByte('d')
			} else {
				buf.WriteByte('-')
			}
			buf.WriteString(permstr)
			buf.WriteString(" 1 user group ")
			fmt.Fprintf(buf, "%12d %s %s\r\n",
				f.Size,
				f.LastModify.Format("Jan _2 2006"),
				f.Name,
			)
		}

		s.writeToDataConn(buf, sc, state, writer)

	// ----- OTHER EXTENSION COMMANDS ----- //

	case "FEAT":
		buf.WriteString("211- Features supported\r\n")
		buf.Write(Features)
		buf.WriteString("211 End\r\n")
		_, err := buf.WriteTo(writer)
		if err != nil {
			writer.Close()
		}

	case "SYST":
		// Write "UNIX Type: L8" as described in https://cr.yp.to/ftp/syst.html
		buf.WriteString("215 UNIX Type: L8\r\n")
		_, err := buf.WriteTo(writer)
		if err != nil {
			writer.Close()
		}

	case "ALLO", "NOOP":
		writeFTPReplySingleline(writer, buf, 200)
	case "ACCT", "STOU", "REST", "NLST", "SITE", "STAT":
		writeFTPReplySingleline(writer, buf, 502) // Command not Implemented
	default:
		writeFTPReplySingleline(writer, buf, 500)
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
