package domain

import "github.com/cskr/pubsub"

// Context holds the application runtime context including the event hub and configuration.
type Context struct {
	Hub *pubsub.PubSub
	Config
}
