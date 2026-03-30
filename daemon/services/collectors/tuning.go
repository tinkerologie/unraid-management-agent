package collectors

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// TuningCollector collects system tuning parameters (kernel settings, NIC offloads, ring buffers).
type TuningCollector struct {
	ctx *domain.Context
}

// NewTuningCollector creates a new tuning parameter collector.
func NewTuningCollector(ctx *domain.Context) *TuningCollector {
	return &TuningCollector{ctx: ctx}
}

// Start begins the tuning collector's periodic data collection.
func (c *TuningCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting tuning collector (interval: %v)", interval)

	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("Tuning collector", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("Tuning collector", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect gathers all tuning parameters and publishes to the event bus.
func (c *TuningCollector) Collect() {
	logger.Debug("Collecting tuning parameters...")

	info := &dto.TuningInfo{
		Timestamp: time.Now(),
	}

	// Turbo Boost / AMD Performance Boost
	turbo := lib.ReadTurboBoost()
	if turbo.Available {
		info.TurboBoost = &dto.TurboBoostInfo{
			Available: turbo.Available,
			Enabled:   turbo.Enabled,
			Vendor:    turbo.Vendor,
		}
	}

	// Disk cache (vm.dirty_*) parameters
	diskCache, err := lib.ReadDiskCacheSettings()
	if err == nil && diskCache != nil {
		info.DiskCache = &dto.DiskCacheInfo{
			DirtyBackgroundRatio: diskCache.DirtyBackgroundRatio,
			DirtyRatio:           diskCache.DirtyRatio,
			DirtyWritebackCenti:  diskCache.DirtyWritebackCenti,
			DirtyExpireCenti:     diskCache.DirtyExpireCenti,
		}
	}

	// Inotify limits
	inotify, err := lib.ReadInotifySettings()
	if err == nil && inotify != nil {
		info.Inotify = &dto.InotifyInfo{
			MaxUserWatches:   inotify.MaxUserWatches,
			MaxUserInstances: inotify.MaxUserInstances,
			MaxQueuedEvents:  inotify.MaxQueuedEvents,
		}
	}

	// NIC offloads and ring buffers for physical interfaces
	info.NICOffloads = make(map[string]*dto.NICOffloadInfo)
	info.NICRingBuffers = make(map[string]*dto.NICRingBufferInfo)

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			name := iface.Name
			if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") {
				continue
			}

			offloads, err := lib.ParseNICOffloads(name)
			if err == nil && offloads != nil {
				info.NICOffloads[name] = &dto.NICOffloadInfo{
					RxChecksumming:        offloads.RxChecksumming,
					TxChecksumming:        offloads.TxChecksumming,
					ScatterGather:         offloads.ScatterGather,
					TCPSegmentOffload:     offloads.TCPSegmentOffload,
					GenericSegmentOffload: offloads.GenericSegmentOffload,
					GenericReceiveOffload: offloads.GenericReceiveOffload,
					LargeReceiveOffload:   offloads.LargeReceiveOffload,
					RxVlanOffload:         offloads.RxVlanOffload,
					TxVlanOffload:         offloads.TxVlanOffload,
				}
			}

			ringBufs, err := lib.ParseNICRingBuffers(name)
			if err == nil && ringBufs != nil {
				info.NICRingBuffers[name] = &dto.NICRingBufferInfo{
					RxMax:     ringBufs.RxMax,
					RxCurrent: ringBufs.RxCurrent,
					TxMax:     ringBufs.TxMax,
					TxCurrent: ringBufs.TxCurrent,
				}
			}
		}
	}

	domain.Publish(c.ctx.Hub, constants.TopicTuningUpdate, info)
}
