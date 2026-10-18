package domain

import (
	"sync"
	"testing"
)

func TestEventBusPublishCount(t *testing.T) {
	bus := NewEventBus(1)
	if got := bus.PublishCount("a"); got != 0 {
		t.Fatalf("unpublished topic count = %d", got)
	}
	ch := bus.Sub("a")
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() { bus.Pub(1, "a", "b") })
	}
	wg.Wait()
	// Drops for a slow subscriber still count as published.
	if a, b := bus.PublishCount("a"), bus.PublishCount("b"); a != 10 || b != 10 {
		t.Fatalf("counts = %d, %d; want 10, 10", a, b)
	}
	bus.Unsub(ch)
}

func TestContextDynamicCollection(t *testing.T) {
	for mode, want := range map[string]bool{
		CollectionModeDynamic: true,
		CollectionModeManual:  false,
		"":                    false,
	} {
		if got := (&Context{CollectionMode: mode}).DynamicCollection(); got != want {
			t.Errorf("DynamicCollection(%q) = %v, want %v", mode, got, want)
		}
	}
}
