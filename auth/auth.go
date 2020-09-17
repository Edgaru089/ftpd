// Package auth provides basic system-wide authentication for ftpd.
package auth

import "strings"

// AccessType is an access level.
type AccessType int

// Access levels
const (
	NoPermission AccessType = iota
	ReadOnly
	ReadWrite
)

// HasAccess decides whether receiver a has required access level.
func (a AccessType) HasAccess(required AccessType) bool {
	return a >= required // A little hack!
}

// Auth is for an authenticator to implement.
type Auth interface {
	// Login verifies the username/password pair, possibly does
	// some logging, and returns the AccessType the user has to the
	// virtual filesystem.
	//
	// A single USER command invokes this method with password empty.
	Login(username, password string) AccessType
}

// Anonymous is an authenticator that allows read-only login with the name "anonymous".
type Anonymous struct{}

// Login implements Auth.Login, and does nothing than verifying the name being "anonymous".
func (Anonymous) Login(username, password string) AccessType {
	if strings.ToLower(username) == "anonymous" && len(password) != 0 {
		return ReadOnly
	} else {
		return NoPermission
	}
}
