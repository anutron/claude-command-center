package daemon

import (
	"net"
	"sync"
	"time"
)

// subscriberSet manages a thread-safe list of subscriber connections.
// Subscribers receive push events from the server via Broadcast.
type subscriberSet struct {
	mu   sync.Mutex
	subs []net.Conn
}

// add registers a connection as a subscriber.
func (ss *subscriberSet) add(conn net.Conn) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.subs = append(ss.subs, conn)
}

// remove unregisters a subscriber connection.
func (ss *subscriberSet) remove(conn net.Conn) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	for i, c := range ss.subs {
		if c == conn {
			ss.subs = append(ss.subs[:i], ss.subs[i+1:]...)
			return
		}
	}
}

// broadcast sends an event to all subscribers, removing any that fail.
// Copies the subscriber list under the lock, then writes outside the lock
// so a slow consumer cannot stall other broadcasts.
func (ss *subscriberSet) broadcast(evt Event) {
	ss.mu.Lock()
	snapshot := make([]net.Conn, len(ss.subs))
	copy(snapshot, ss.subs)
	ss.mu.Unlock()

	var failed []net.Conn
	for _, conn := range snapshot {
		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := WriteMessage(conn, evt); err != nil {
			conn.Close()
			failed = append(failed, conn)
		}
	}

	if len(failed) > 0 {
		ss.mu.Lock()
		alive := ss.subs[:0]
		failSet := make(map[net.Conn]struct{}, len(failed))
		for _, c := range failed {
			failSet[c] = struct{}{}
		}
		for _, c := range ss.subs {
			if _, bad := failSet[c]; !bad {
				alive = append(alive, c)
			}
		}
		ss.subs = alive
		ss.mu.Unlock()
	}
}

// Broadcast sends an event to all subscriber connections.
func (s *Server) Broadcast(evt Event) {
	s.subscribers.broadcast(evt)
}
