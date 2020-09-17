package auth

import (
	"bufio"
	"log"
	"os"
	"strings"
)

// File represents an authenticator from an text file.
//
// The file has lines that are either empty, begin with #,
// or with the following format:
//
//    [Username]:[Password]:["r" or "rw"]
//
// The first semicolon ends the username, and the last ends the password.
// A line ending in "r" represents a read-only account, while one ending
// in "rw" represents a read-write one.
//
// Usernames are unique and later ones overwrite existing ones.
type File struct {
	// string key is username
	m map[string]struct {
		pass string
		l    AccessType
	}
}

// Login implements Auth.Login.
func (a *File) Login(username, password string) AccessType {
	obj, ok := a.m[username]
	if !ok || obj.pass != password {
		return NoPermission
	}
	return obj.l
}

// NewFile creates a new file based authenticator.
func NewFile(filename string) (a *File, err error) {
	var f *os.File
	f, err = os.Open(filename)
	if err != nil {
		return
	}

	a = &File{m: make(map[string]struct {
		pass string
		l    AccessType
	})}

	lnum := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Bytes()
		lnum++

		if len(line) == 0 || line[0] == '#' {
			continue
		}

		ls := string(line)

		id1 := strings.IndexByte(ls, ':')
		id2 := strings.LastIndexByte(ls, ':')
		if id1 == -1 || id1 == id2 {
			log.Printf("auth.NewFile: line %d fromat error (no enough seperator)", lnum)
			continue
		}

		uname := ls[:id1]
		pass := ls[id1+1 : id2]
		mode := ls[id2+1:]
		log.Printf("auth.NewFile: line %d: user=%s, len(pass)=%d, mode=%s", lnum, uname, len(pass), mode)

		switch mode {
		case "rw":
			// Read-Write
			a.m[uname] = struct {
				pass string
				l    AccessType
			}{
				pass: pass,
				l:    ReadWrite,
			}
		case "r":
			// Read-Only
			a.m[uname] = struct {
				pass string
				l    AccessType
			}{
				pass: pass,
				l:    ReadOnly,
			}
		default:
			log.Printf(`auth.NewFile: line %d format error (unknown mode "%s")`, lnum, mode)
		}
	}

	return
}
