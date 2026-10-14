package services

import (
	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
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

// hardwareProbes lists one check probe per hardware class (#261).
func hardwareProbes(hp *collectors.HardwareProbe) []platform.Probe {
	probes := make([]platform.Probe, 0, len(collectors.HardwareClasses))
	for _, class := range collectors.HardwareClasses {
		probes = append(probes, platform.Probe{
			Name:  class.Capability,
			Kind:  platform.ProbeCheck,
			Check: hp.CheckFor(class.Capability),
		})
	}
	return probes
}

// capabilityProbes is the full probe list of the capability snapshot.
func capabilityProbes() []platform.Probe {
	return append(resilienceProbes(), hardwareProbes(collectors.NewHardwareProbe())...)
}
