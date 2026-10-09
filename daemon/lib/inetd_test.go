package lib

import (
	"errors"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
)

// inetdConfDisabled is the FTP part of /etc/inetd.conf on Unraid 7.4 with
// the FTP server disabled (the default).
const inetdConfDisabled = `# limetech - disable ftp - new security policy
#ftp     stream  tcp     nowait  root    /usr/sbin/tcpd  vsftpd
#
# ftp     stream  tcp     nowait  root    /usr/sbin/tcpd  proftpd
#
# limetech - disable telnet (see also /etc/securetty) - new security policy
#telnet	stream  tcp     nowait  root    /usr/sbin/tcpd	in.telnetd
`

// inetdConfEnabled is the same file after Settings > FTP Server > Enabled
// (webGui/scripts/ftpusers runs sed 's/^#\(ftp.*vsftpd\)$/\1/').
const inetdConfEnabled = `# limetech - disable ftp - new security policy
ftp     stream  tcp     nowait  root    /usr/sbin/tcpd  vsftpd
#
# ftp     stream  tcp     nowait  root    /usr/sbin/tcpd  proftpd
`

const tcpHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

// tcpListen21 has inetd listening on 0.0.0.0:21 next to other sockets.
const tcpListen21 = tcpHeader +
	"   0: 7E844F64:1484 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 387384 1 0000000078e8a331 100 0 0 10 0\n" +
	"   1: 00000000:0015 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 52536051 1 00000000f5c97f48 100 0 0 10 0\n"

// tcpEstablished21 has a connection whose local port is 21 but no listener.
const tcpEstablished21 = tcpHeader +
	"   0: 7E844F64:0015 0A02030A:C350 01 00000000:00000000 00:00000000 00000000     0        0 387384 1 0000000078e8a331 100 0 0 10 0\n"

const tcp6Listen21 = "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
	"   0: 00000000000000000000000000000000:0015 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 492036 1 000000009eb598a2 100 0 0 10 0\n"

// TestInetdServiceEnabled checks which inetd.conf lines count as an active entry.
func TestInetdServiceEnabled(t *testing.T) {
	tests := []struct {
		name    string
		conf    string
		service string
		want    bool
	}{
		{"ftp disabled (default)", inetdConfDisabled, "ftp", false},
		{"ftp enabled", inetdConfEnabled, "ftp", true},
		{"telnet still disabled", inetdConfEnabled, "telnet", false},
		{"tab separated", "ftp\tstream\ttcp\tnowait\troot\t/usr/sbin/tcpd\tvsftpd\n", "ftp", true},
		{"service name only as a prefix", "ftpd stream tcp nowait root /usr/sbin/tcpd x\n", "ftp", false},
		{"empty file", "", "ftp", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InetdServiceEnabled([]byte(tt.conf), tt.service); got != tt.want {
				t.Errorf("InetdServiceEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTCPPortListening checks LISTEN detection in /proc/net/tcp and tcp6 tables.
func TestTCPPortListening(t *testing.T) {
	tests := []struct {
		name   string
		tables []string
		port   int
		want   bool
	}{
		{"ipv4 listener", []string{tcpListen21}, 21, true},
		{"ipv6 listener", []string{tcpHeader, tcp6Listen21}, 21, true},
		{"other port", []string{tcpListen21}, 22, false},
		{"established, not listening", []string{tcpEstablished21}, 21, false},
		{"header only", []string{tcpHeader}, 21, false},
		{"no tables", nil, 21, false},
		{"malformed lines", []string{"garbage\n 0: nocolon 00000000:0000 0A\n 1: 00000000:zz 00000000:0000 0A\n"}, 21, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables := make([][]byte, 0, len(tt.tables))
			for _, table := range tt.tables {
				tables = append(tables, []byte(table))
			}
			if got := TCPPortListening(tt.port, tables...); got != tt.want {
				t.Errorf("TCPPortListening() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestFTPServerListening checks the port 21 check reads both socket tables and skips unreadable ones.
func TestFTPServerListening(t *testing.T) {
	errMissing := errors.New("no such file")
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"ipv4 listener", map[string]string{constants.ProcNetTCP: tcpListen21, constants.ProcNetTCP6: tcpHeader}, true},
		{"ipv6 only, tcp unreadable", map[string]string{constants.ProcNetTCP6: tcp6Listen21}, true},
		{"nothing listening", map[string]string{constants.ProcNetTCP: tcpEstablished21, constants.ProcNetTCP6: tcpHeader}, false},
		{"tables unreadable", map[string]string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readFile := func(name string) ([]byte, error) {
				if data, ok := tt.files[name]; ok {
					return []byte(data), nil
				}
				return nil, errMissing
			}
			if got := FTPServerListening(readFile); got != tt.want {
				t.Errorf("FTPServerListening() = %v, want %v", got, tt.want)
			}
		})
	}
}
