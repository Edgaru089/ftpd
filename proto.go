package ftpd

var Features = []byte(" UTF8\r\n MDTM\r\n SIZE\r\n TVFS\r\n MLST type;size;modify;\r\n")

var ReplyCodes = map[int][]byte{
	200: []byte("Command okay."),
	500: []byte("Syntax error, command unrecognized."),
	501: []byte("Syntax error in parameters or arguments."),
	202: []byte("Command not implemented, superfluous at this site."),
	502: []byte("Command not implemented."),
	503: []byte("Bad sequence of commands."),
	504: []byte("Command not implemented for that parameter."),

	110: []byte("%s = %s"),                    // Restart Marker Reply
	211: []byte("%s"),                         // System status, or system help reply.
	212: []byte("%s"),                         // Directory status.
	213: []byte("%s"),                         // File status (file size / modify time)
	214: []byte("(Sorry, no help available)"), // This reply is useful only to the human user.
	215: []byte("%s"),                         // System type, an official system name from the list in the Assigned Numbers document.

	120: []byte("Service ready in %d minutes."),
	220: []byte("Service ready."),
	221: []byte("Service closing control connection."),                // Logged out if appropriate.
	421: []byte("Service not available, closing control connection."), // This may be a reply to any command if the service knows it must shut down.

	125: []byte("Data connection already open; transfer starting."),
	225: []byte("Data connection open; no transfer in progress."),
	425: []byte("Can't open data connection."),
	226: []byte("Closing data connection."), // Requested file action successful (for example, file transfer or file abort).
	426: []byte("Connection closed; transfer aborted."),
	//227: []byte("Entering Passive Mode (%d,%d,%d,%d,%d,%d)."),
	227: []byte("Entering Passive Mode (%s)."),

	230: []byte("User logged in, proceed."),
	530: []byte("Not logged in."),
	331: []byte("User name okay, need password."),
	332: []byte("Need account for login."),
	532: []byte("Need account for storing files."),

	150: []byte("File status okay; about to open data connection."),
	250: []byte("Requested file action okay, completed."),
	257: []byte("\"%s\" created."), // Pathname created.
	350: []byte("Requested file action pending further information."),
	450: []byte("Requested file action not taken."), // File unavailable (e.g., file busy).
	550: []byte("Requested action not taken."),      // File unavailable (e.g., file not found, no access).
	451: []byte("Requested action aborted. Local error in processing."),
	551: []byte("Requested action aborted. Page type unknown."),
	452: []byte("Requested action not taken."),    // Insufficient storage space in system.
	552: []byte("Requested file action aborted."), // Exceeded storage allocation (for current directory or dataset).
	553: []byte("Requested action not taken."),    // File name not allowed.

}

func init() {

}
