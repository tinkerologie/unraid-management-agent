package controllers

import (
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
)

const (
	// statusWorkers bounds how many rc status scripts ListServiceStatuses runs
	// at once.
	statusWorkers = 4
	// statusTimeout bounds one rc status script, so a hung check (e.g.
	// "docker info" against an unresponsive daemon) cannot stall the list.
	statusTimeout = 15 * time.Second
)

// ServiceStatus is the running state of one service.
type ServiceStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
}

// runStatusScript runs an rc script's status action with statusTimeout.
func runStatusScript(command string, args ...string) (string, error) {
	return lib.ExecCommandOutputWithTimeout(statusTimeout, command, args...)
}

// ListServiceStatuses checks every service in ValidServiceNames and returns
// the results in that order. Each check runs the service's rc status script
// (about 0.1-1 s each, most sleep 0.1 s first), so up to statusWorkers checks
// run concurrently instead of one after another.
func (sc *ServiceController) ListServiceStatuses() []ServiceStatus {
	names := ValidServiceNames()
	statuses := make([]ServiceStatus, len(names))
	slots := make(chan struct{}, statusWorkers)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			// Only unknown names return an error; these all come from
			// ValidServiceNames.
			running, _ := sc.GetServiceStatus(name)
			statuses[i] = ServiceStatus{Name: name, Running: running}
		})
	}
	wg.Wait()
	return statuses
}
