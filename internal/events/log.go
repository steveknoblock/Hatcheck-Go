// events/log.go
package events

import "sync"

type Log struct {
	mu      sync.RWMutex
	logPath string
	Log     []Entry
	indexes []Index
}
