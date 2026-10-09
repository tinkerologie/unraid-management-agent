package lib

import (
	"strconv"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
)

// FTPPort is the TCP port the Unraid FTP server (vsftpd, started by inetd)
// listens on.
const FTPPort = 21

// tcpListenState is the socket state code for LISTEN in /proc/net/tcp{,6}.
const tcpListenState = "0A"

// InetdServiceEnabled reports whether inetd.conf content has an active
// (uncommented) entry for service. Unraid's Settings > FTP Server page
// (webGui/scripts/ftpusers) enables FTP by uncommenting the
// "ftp ... vsftpd" line and disables it by commenting the line out.
func InetdServiceEnabled(inetdConf []byte, service string) bool {
	for line := range strings.SplitSeq(string(inetdConf), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == service {
			return true
		}
	}
	return false
}

// TCPPortListening reports whether any of the given /proc/net/tcp or
// /proc/net/tcp6 tables has a socket in LISTEN state on port.
func TCPPortListening(port int, tables ...[]byte) bool {
	for _, table := range tables {
		for line := range strings.SplitSeq(string(table), "\n") {
			// "sl local_address rem_address st ...", e.g. "0: 00000000:0015 00000000:0000 0A ..."
			fields := strings.Fields(line)
			if len(fields) < 4 || fields[3] != tcpListenState {
				continue
			}
			_, hexPort, ok := strings.Cut(fields[1], ":")
			if !ok {
				continue
			}
			if p, err := strconv.ParseUint(hexPort, 16, 16); err == nil && int(p) == port {
				return true
			}
		}
	}
	return false
}

// FTPServerListening reports whether something accepts connections on the
// FTP port, which is how the webGUI's FTP Server page decides that the
// server is enabled (it runs lsof -i:21 and looks for LISTEN). vsftpd itself
// only runs while a session is open, so a process check cannot tell.
// readFile reads the socket tables; tables that cannot be read are skipped.
func FTPServerListening(readFile func(name string) ([]byte, error)) bool {
	var tables [][]byte
	for _, path := range []string{constants.ProcNetTCP, constants.ProcNetTCP6} {
		if data, err := readFile(path); err == nil {
			tables = append(tables, data)
		}
	}
	return TCPPortListening(FTPPort, tables...)
}
