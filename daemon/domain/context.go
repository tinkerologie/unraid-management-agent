package domain

import "github.com/cskr/pubsub"

// Intervals holds collection interval settings in seconds.
type Intervals struct {
	System       int
	Array        int
	Disk         int
	Docker       int
	VM           int
	UPS          int
	NUT          int
	GPU          int
	Shares       int
	Network      int
	Hardware     int
	ZFS          int
	Notification int
	Registration int
	Unassigned   int
}

// Context holds the application runtime context including the event hub and configuration.
type Context struct {
	Hub       *pubsub.PubSub
	Intervals Intervals
	Config
}
