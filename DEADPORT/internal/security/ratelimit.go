package security

import (
	"sync"
	"time"
)

// Bucket is a classic token bucket. It is safe for concurrent use.
type Bucket struct {
	mu       sync.Mutex
	capacity float64
	tokens   float64
	rate     float64 // tokens per second
	last     time.Time
	now      func() time.Time
}

// NewBucket creates a bucket holding up to burst tokens, refilling at
// perSecond tokens per second. It starts full.
func NewBucket(burst int, perSecond float64) *Bucket {
	return &Bucket{
		capacity: float64(burst),
		tokens:   float64(burst),
		rate:     perSecond,
		now:      time.Now,
	}
}

// Allow consumes one token if available.
func (b *Bucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if !b.last.IsZero() {
		b.tokens += now.Sub(b.last).Seconds() * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

// KeyedLimiter keeps one bucket per key (for example per IP address).
// Idle buckets are garbage-collected so memory stays bounded.
type KeyedLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*keyedEntry
	burst     int
	perSecond float64
	maxKeys   int
	now       func() time.Time
}

type keyedEntry struct {
	b        *Bucket
	lastSeen time.Time
}

// NewKeyedLimiter creates a per-key limiter.
func NewKeyedLimiter(burst int, perSecond float64) *KeyedLimiter {
	return &KeyedLimiter{
		buckets:   make(map[string]*keyedEntry),
		burst:     burst,
		perSecond: perSecond,
		maxKeys:   10000,
		now:       time.Now,
	}
}

// Allow consumes a token for key.
func (k *KeyedLimiter) Allow(key string) bool {
	k.mu.Lock()
	now := k.now()
	e, ok := k.buckets[key]
	if !ok {
		if len(k.buckets) >= k.maxKeys {
			k.gcLocked(now)
		}
		e = &keyedEntry{b: NewBucket(k.burst, k.perSecond)}
		e.b.now = k.now
		k.buckets[key] = e
	}
	e.lastSeen = now
	k.mu.Unlock()
	return e.b.Allow()
}

// gcLocked drops buckets idle long enough to have refilled completely.
func (k *KeyedLimiter) gcLocked(now time.Time) {
	full := time.Duration(float64(k.burst)/k.perSecond*float64(time.Second)) + time.Minute
	for key, e := range k.buckets {
		if now.Sub(e.lastSeen) > full {
			delete(k.buckets, key)
		}
	}
	// Still too many? Drop everything; worst case a spammer gets a fresh bucket.
	if len(k.buckets) >= k.maxKeys {
		k.buckets = make(map[string]*keyedEntry)
	}
}

// Len returns the number of tracked keys (for tests/metrics).
func (k *KeyedLimiter) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.buckets)
}
