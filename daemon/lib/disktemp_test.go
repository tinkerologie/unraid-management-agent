package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDiskTempsFromFile(t *testing.T) {
	const sample = `["disk1"]
name="disk1"
device="sdb"
temp="38"
["disk2"]
device="sdc"
temp="*"
["cache"]
device="nvme0n1"
temp=""
`
	dir := t.TempDir()
	p := filepath.Join(dir, "disks.ini")
	if err := os.WriteFile(p, []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadDiskTempsFromFile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if d := got["disk1"]; d.Device != "sdb" || d.TempC != 38 || d.SpunDown {
		t.Errorf("disk1: got %+v, want device=sdb temp=38 spundown=false", d)
	}
	if d := got["disk2"]; !d.SpunDown || d.TempC != 0 {
		t.Errorf("disk2 (temp=*): got %+v, want spundown=true temp=0", d)
	}
	if d := got["cache"]; !d.SpunDown {
		t.Errorf("cache (empty temp): got %+v, want spundown=true", d)
	}
}

func TestReadDiskTempsMissingFile(t *testing.T) {
	got, err := ReadDiskTempsFromFile(filepath.Join(t.TempDir(), "nope.ini"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if got == nil {
		t.Fatal("expected non-nil (empty) map even on error")
	}
}
