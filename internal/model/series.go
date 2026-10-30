package model

// SparkWindow is how far back the rolling buffers reach, in seconds.
const SparkWindow = 30 * 60

// Series keeps rolling per-item buffers, so sparklines need no history read
// after the first one. It belongs to the poller goroutine.
type Series struct {
	seeded map[string]bool
	data   map[string][]Point
}

// NewSeries returns an empty store.
func NewSeries() *Series {
	return &Series{seeded: map[string]bool{}, data: map[string][]Point{}}
}

// Seeded reports whether the item's history has been loaded.
func (s *Series) Seeded(itemID string) bool { return s.seeded[itemID] }

// MarkSeeded records a successful history load. A failed one is not marked,
// so it is retried (zabterm gave up on the item, D6).
func (s *Series) MarkSeeded(itemID string) { s.seeded[itemID] = true }

// Push appends a sample if it is newer than the last, then trims samples
// older than the window ending at now.
func (s *Series) Push(itemID string, clock int64, v float64, now int64) {
	buf := s.data[itemID]
	if n := len(buf); n == 0 || float64(clock) > buf[n-1].T {
		buf = append(buf, Point{float64(clock), v})
	}
	cut := 0
	for cut < len(buf) && buf[cut].T < float64(now-SparkWindow) {
		cut++
	}
	s.data[itemID] = buf[cut:]
}

// Get returns a copy, so a snapshot stays valid as the buffers move on.
func (s *Series) Get(itemID string) []Point {
	return append([]Point(nil), s.data[itemID]...)
}

// Retain forgets items not in keep (zabterm kept them forever, D19).
func (s *Series) Retain(keep map[string]bool) {
	for id := range s.data {
		if !keep[id] {
			delete(s.data, id)
		}
	}
	for id := range s.seeded {
		if !keep[id] {
			delete(s.seeded, id)
		}
	}
}
