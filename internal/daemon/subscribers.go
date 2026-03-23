package daemon

import (
	"net"
	"sync"
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
func (ss *subscriberSet) broadcast(evt Event) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	alive := ss.subs[:0]
	for _, conn := range ss.subs {
		if err := WriteMessage(conn, evt); err != nil {
			conn.Close()
			continue
		}
		alive = append(alive, conn)
	}
	ss.subs = alive
}

// Broadcast sends an event to all subscriber connections.
func (s *Server) Broadcast(evt Event) {
	s.subscribers.broadcast(evt)
}
