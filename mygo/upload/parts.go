package upload

import (
	"strings"
	"sync"
	"time"

	"github.com/3-lines-studio/bifrost/mygo/client"
)

const (
	// minPart is the smallest part S3 takes, and maxPart the largest.
	minPart = 8 << 20
	maxPart = 128 << 20
	// maxParts is how many parts S3 joins.
	maxParts = 10_000
	// signBatch is how many part URLs Run asks for at once.
	signBatch = 64
	// notifyEvery is how often progress goes to the app while parts go out:
	// a part of 128MiB would otherwise report thousands of times a second.
	notifyEvery = 50 * time.Millisecond
)

// PartSize returns how many bytes each part of a file of size bytes holds when
// neither the server nor the app chooses: at least 8MiB, in steps of 1MiB, so
// the parts stay under the number a store joins.
func PartSize(size int64) int64 {
	part := int64(minPart)
	if needed := (size + maxParts - 1) / maxParts; needed > part {
		part = (needed + (1 << 20) - 1) &^ ((1 << 20) - 1)
	}
	return min(part, maxPart)
}

// partCount is how many parts a file of size bytes takes.
func partCount(size, partSize int64) int {
	if size <= 0 || partSize <= 0 {
		return 0
	}
	return int((size + partSize - 1) / partSize)
}

// worthRetry reports whether an answer that is not a success is worth sending
// again: a busy server, a slow one, or one that failed.
func worthRetry(status int) bool {
	return status == 408 || status == 429 || (status >= 500 && status <= 599)
}

// trimETag takes the quotes off the ETag of an answer.
func trimETag(value string) string {
	return strings.TrimSpace(strings.Trim(value, `"`))
}

// header reads a header of an answer whichever way the platform spells its
// name.
func header(response client.Response, name string) string {
	for key, value := range response.Header {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

// progress counts the bytes the server received. Parts report as they go, and
// the mutex makes the sum of what they said.
type progress struct {
	mu       sync.Mutex
	sent     []int64
	sizes    []int64
	complete []bool
	total    int64
	parts    int
	finished int
	last     time.Time
	notify   func(Progress)
}

// newProgress returns the counter of an upload of total bytes in count parts of
// size bytes each.
func newProgress(total, size int64, count int) *progress {
	p := &progress{
		sent:     make([]int64, count+1),
		sizes:    make([]int64, count+1),
		complete: make([]bool, count+1),
		total:    total,
		parts:    count,
	}
	for number := 1; number <= count; number++ {
		start := int64(number-1) * size
		p.sizes[number] = min(size, total-start)
	}
	return p
}

// sending records the bytes of a part that went out.
func (p *progress) sending(number int, bytes int64) {
	p.mu.Lock()
	if p.open(number) {
		p.sent[number] = min(bytes, p.sizes[number])
	}
	p.report(false)
	p.mu.Unlock()
}

// retry reports that a part starts again, with none of it sent.
func (p *progress) retry(number int) {
	p.mu.Lock()
	if p.open(number) {
		p.sent[number] = 0
	}
	p.report(false)
	p.mu.Unlock()
}

// done records a part the server has.
func (p *progress) done(part Part) {
	p.mu.Lock()
	number := part.Number
	if p.open(number) {
		p.complete[number] = true
		p.sent[number] = p.sizes[number]
		p.finished++
	}
	p.report(true)
	p.mu.Unlock()
}

// stop reports the upload as it is, one last time.
func (p *progress) stop() {
	p.mu.Lock()
	p.report(true)
	p.mu.Unlock()
}

// changed reports the upload as it is now.
func (p *progress) changed() {
	p.mu.Lock()
	p.report(true)
	p.mu.Unlock()
}

// open reports whether number names a part the counter knows and the server
// does not have yet. The caller holds the lock.
func (p *progress) open(number int) bool {
	return number >= 1 && number <= p.parts && !p.complete[number]
}

// report calls the app with the bytes sent so far, at most once every
// notifyEvery unless final. The caller holds the lock.
func (p *progress) report(final bool) {
	if p.notify == nil {
		return
	}
	now := time.Now()
	if !final && now.Sub(p.last) < notifyEvery {
		return
	}
	p.last = now
	var sent int64
	for number := 1; number <= p.parts; number++ {
		sent += p.sent[number]
	}
	p.notify(Progress{Sent: sent, Total: p.total, Done: p.finished, Parts: p.parts})
}
