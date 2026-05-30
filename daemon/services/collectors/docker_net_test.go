package collectors

import (
	"strings"
	"testing"
)

func TestParseProcNetDev(t *testing.T) {
	const sample = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:    1234      10    0    0    0     0          0         0    1234      10    0    0    0     0       0          0
  eth0: 1000000     500    0    0    0     0          0         0   250000     300    0    0    0     0       0          0
  eth1:    5000       5    0    0    0     0          0         0     2000       2    0    0    0     0       0          0`
	rx, tx := parseProcNetDev(strings.NewReader(sample))
	if rx != 1005000 || tx != 252000 {
		t.Errorf("rx=%d tx=%d, want 1005000/252000 (lo excluded, eth0+eth1 summed)", rx, tx)
	}
}

func TestParseProcNetDev_Empty(t *testing.T) {
	rx, tx := parseProcNetDev(strings.NewReader(""))
	if rx != 0 || tx != 0 {
		t.Errorf("empty: rx=%d tx=%d, want 0/0", rx, tx)
	}
}
