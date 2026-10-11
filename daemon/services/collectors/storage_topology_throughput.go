package collectors

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// diskstatsSectorBytes is the unit of the /proc/diskstats sector counters,
	// independent of the device's logical block size.
	diskstatsSectorBytes = 512
	// sasLaneBytesPerGbps is the payload rate of one SAS lane per Gbps of link
	// rate: SAS uses 8b/10b encoding, so 12 Gbps carries 1.2 GB/s.
	sasLaneBytesPerGbps = 1e8
	// minThroughputWindow is the shortest window a rate is computed over.
	minThroughputWindow = time.Second
)

// diskSample is one reading of /proc/diskstats.
type diskSample struct {
	at time.Time
	// sectors maps a block device name to its cumulative {read, written} sector counts.
	sectors map[string][2]uint64
}

// parseDiskstats parses /proc/diskstats: "major minor name reads merged
// sectors_read ms writes merged sectors_written ...". Malformed lines are skipped.
func parseDiskstats(data string) map[string][2]uint64 {
	result := map[string][2]uint64{}
	for line := range strings.SplitSeq(data, "\n") {
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		read, err1 := strconv.ParseUint(f[5], 10, 64)
		written, err2 := strconv.ParseUint(f[9], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		result[f[2]] = [2]uint64{read, written}
	}
	return result
}

// sampleDiskstats reads the block device I/O counters. Reading /proc/diskstats
// only returns kernel counters; nothing is sent to the drives.
func (c *StorageTopologyCollector) sampleDiskstats() *diskSample {
	b, err := os.ReadFile(c.DiskstatsPath)
	if err != nil {
		logger.Debug("StorageTopology: cannot read %s: %v", c.DiskstatsPath, err)
		return nil
	}
	return &diskSample{at: c.Now(), sectors: parseDiskstats(string(b))}
}

// sectorDelta returns cur-prev, or 0 when the counter went backwards (reset).
func sectorDelta(prev, cur uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

// throughputSum accumulates the sector deltas of a group of drives.
type throughputSum struct {
	read, written uint64
	drives        int
}

// add counts one drive's deltas, allocating the sum when s is nil.
func (s *throughputSum) add(read, written uint64) *throughputSum {
	if s == nil {
		s = &throughputSum{}
	}
	s.read += read
	s.written += written
	s.drives++
	return s
}

// result converts the accumulated deltas into rates over elapsed seconds.
func (s *throughputSum) result(capacity, elapsed float64) *dto.StorageThroughput {
	read := float64(s.read) * diskstatsSectorBytes / elapsed
	write := float64(s.written) * diskstatsSectorBytes / elapsed
	t := &dto.StorageThroughput{
		ReadBytesPerSec:     math.Round(read),
		WriteBytesPerSec:    math.Round(write),
		TotalBytesPerSec:    math.Round(read + write),
		CapacityBytesPerSec: capacity,
		Drives:              s.drives,
		IntervalSeconds:     math.Round(elapsed*1000) / 1000,
	}
	if capacity > 0 {
		u := math.Round((read+write)/capacity*1000) / 10
		t.UtilizationPercent = &u
	}
	return t
}

// controllerCapacity is the payload capacity of a controller's connected phys,
// falling back to its ports (width x rate) when no phy reports a link rate.
func controllerCapacity(ctrl dto.StorageController) float64 {
	var capacity float64
	for _, phy := range ctrl.Phys {
		if phy.Connected && phy.LinkRateGbps != nil {
			capacity += *phy.LinkRateGbps * sasLaneBytesPerGbps
		}
	}
	if capacity == 0 {
		for _, p := range ctrl.Ports {
			if p.LinkRateGbps != nil {
				capacity += float64(p.Width) * *p.LinkRateGbps * sasLaneBytesPerGbps
			}
		}
	}
	return capacity
}

// enclosureCapacity is the payload capacity of the controller ports cabled
// directly to the enclosure. An enclosure reached only through another
// (daisy-chained) enclosure has no direct link and gets 0.
func enclosureCapacity(enclosureID string, controllers []dto.StorageController) float64 {
	var capacity float64
	for _, ctrl := range controllers {
		for _, p := range ctrl.Ports {
			if p.AttachedEnclosureID == enclosureID && p.LinkRateGbps != nil {
				capacity += float64(p.Width) * *p.LinkRateGbps * sasLaneBytesPerGbps
			}
		}
	}
	return capacity
}

// applyThroughput sets controller and enclosure throughput from two diskstats
// samples. Nothing is set without a previous sample or when the samples are
// less than minThroughputWindow apart. Only drives whose block device appears
// in both samples are counted; a group without such drives gets no throughput.
func applyThroughput(topo *dto.StorageTopology, prev, cur *diskSample) {
	if prev == nil || cur == nil {
		return
	}
	window := cur.at.Sub(prev.at)
	if window < minThroughputWindow {
		return
	}
	elapsed := window.Seconds()

	byController := map[int]*throughputSum{}
	byEnclosure := map[string]*throughputSum{}
	for _, d := range topo.Drives {
		if d.Device == "" {
			continue
		}
		before, ok1 := prev.sectors[d.Device]
		after, ok2 := cur.sectors[d.Device]
		if !ok1 || !ok2 {
			continue
		}
		read, written := sectorDelta(before[0], after[0]), sectorDelta(before[1], after[1])
		byController[d.ControllerIndex] = byController[d.ControllerIndex].add(read, written)
		if d.EnclosureID != "" {
			byEnclosure[d.EnclosureID] = byEnclosure[d.EnclosureID].add(read, written)
		}
	}

	for i := range topo.Controllers {
		ctrl := &topo.Controllers[i]
		if s := byController[ctrl.Index]; s != nil {
			ctrl.Throughput = s.result(controllerCapacity(*ctrl), elapsed)
		}
	}
	for i := range topo.Enclosures {
		encl := &topo.Enclosures[i]
		if s := byEnclosure[encl.ID]; s != nil {
			encl.Throughput = s.result(enclosureCapacity(encl.ID, topo.Controllers), elapsed)
		}
	}
}
