package ftpd

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/Edgaru089/ftpd/mount"
)

// ScanCRLF is a bufio.ScanFunc that stops at any CRLF line feed ("\r\n").
func ScanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	id := bytes.Index(data, []byte{'\r', '\n'})

	if id == -1 { // no newline
		if atEOF {
			return 0, nil, io.EOF // EOF
		} else {
			return 0, nil, nil // no newline and not EOF - request more data
		}
	}

	return id + 2, data[:id], nil
}

// Writes a single FTP reply line.
// the params are fed to fmt.Sprintf if provided.
// Panics if ReplyCodes does not have code in it.
// Closes the writer if error returned.
func writeFTPReplySingleline(writer io.WriteCloser, code int, params ...interface{}) {
	reply, ok := ReplyCodes[code]
	if !ok {
		panic(fmt.Errorf("writeFTPReply: %d not a valid reply code", code))
	}

	// We (should) avoid small writes and form a buffer.
	var buf bytes.Buffer
	buf.Write(strconv.AppendInt(nil, int64(code), 10))
	buf.WriteByte(' ')

	if len(params) == 0 {
		buf.Write(reply) // This should be a much faster path.
	} else {
		fmt.Fprintf(&buf, string(reply), params...)
	}

	buf.WriteString("\r\n")

	_, err := buf.WriteTo(writer)
	if err != nil {
		writer.Close()
	}
}

// Parses FTP Host-Port representation (h1,h2,h3,h4,p1,p2)
func parseHostPort(param []byte) (ip net.IP, port int) {
	var h1, h2, h3, h4, p1, p2 int
	fmt.Fscanf(bytes.NewReader(param), "%d,%d,%d,%d,%d,%d", &h1, &h2, &h3, &h4, &p1, &p2)
	port = p1<<8 + p2
	ip = net.IP{byte(h1), byte(h2), byte(h3), byte(h4)}
	return
}

// Packs FTP Host-Port representation. Panics if ip is not a IPv4 address.
func packHostPort(writer io.Writer, ip net.IP, port int) {
	ip4 := ip.To4()
	if ip4 == nil {
		panic("packHostPort: IP not IPv4")
	}

	fmt.Fprintf(writer, "%d,%d,%d,%d,%d,%d", ip4[0], ip4[1], ip4[2], ip4[3], (port&0xff00)>>8, port&0xff)
}

// Calls packHostPort with a Buffer.
func packHostPortSlice(ip net.IP, port int) []byte {
	var buf bytes.Buffer
	packHostPort(&buf, ip, port)
	return buf.Bytes()
}

func wrapSlash(src string) string {
	if len(src) == 0 {
		return "/"
	}
	if src[0] != '/' {
		src = "/" + src
	}
	if src[len(src)-1] != '/' {
		src = src + "/"
	}
	return src
}

func ftpTime(t time.Time) string {
	utc := t.UTC()
	y, m, d := utc.Date()
	h, min, s := utc.Clock()
	return fmt.Sprintf("%04d%02d%02d%02d%02d%02d", y, m, d, h, min, s)
	// TODO Sub-second time
}

func fileTypeString(isDir bool) string {
	if isDir {
		return "dir"
	} else {
		return "file"
	}
}

func formatMLSXString(writer io.Writer, file *mount.File) (n int, err error) {
	return fmt.Fprintf(writer, "type=%s;size=%d;modify=%s; %s", fileTypeString(file.IsDirectory), file.Size, ftpTime(file.LastModify), file.Name)
}
