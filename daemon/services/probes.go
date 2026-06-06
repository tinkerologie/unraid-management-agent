package services

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

// resilienceProbes lists the OS capabilities the hot-path subsystems depend on.
// Kept here (not in platform) so platform stays Unraid-agnostic and cycle-free.
func resilienceProbes() []platform.Probe {
	return []platform.Probe{
		{Name: "var.ini", Target: constants.VarIni, Kind: platform.ProbePath},
		{Name: "disks.ini", Target: constants.DisksIni, Kind: platform.ProbePath},
		{Name: "shares.ini", Target: constants.SharesIni, Kind: platform.ProbePath},
		{Name: "mdcmd", Target: constants.MdcmdBin, Kind: platform.ProbeBinary},
		{Name: "smartctl", Target: constants.SmartctlBin, Kind: platform.ProbeBinary},
		{Name: "docker", Target: constants.DockerBin, Kind: platform.ProbeBinary},
		{Name: "virsh", Target: constants.VirshBin, Kind: platform.ProbeBinary},
	}
}
