// Package ulid generates the identifiers Protean uses everywhere: ULIDs, whose
// lexical order equals creation order. Nothing in the system sorts by
// timestamp; it sorts by ID.
package ulid

import (
	crand "crypto/rand"
	"sync"

	oklog "github.com/oklog/ulid/v2"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// Generator hands out ULIDs. It is safe for concurrent use, and it reads "now"
// from the injected clock so tests are deterministic.
//
// IDs are strictly increasing even within one clock millisecond: the entropy
// source is monotonic, so two IDs minted from a frozen clock still sort in the
// order they were minted.
type Generator struct {
	clock ports.Clock

	mu      sync.Mutex
	entropy *oklog.MonotonicEntropy
	lastMS  uint64
}

// NewGenerator returns a Generator seeded from the process CSPRNG.
func NewGenerator(clock ports.Clock) *Generator {
	return &Generator{
		clock:   clock,
		entropy: oklog.Monotonic(crand.Reader, 0),
	}
}

// New returns a fresh ULID in its canonical 26-character uppercase form.
func (g *Generator) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	ms := oklog.Timestamp(g.clock.Now())
	if ms < g.lastMS {
		ms = g.lastMS
	}
	for {
		id, err := oklog.New(ms, g.entropy)
		if err == nil {
			g.lastMS = ms
			return id.String()
		}
		// The only failure mode is monotonic overflow: too many IDs inside one
		// millisecond. Moving to the next millisecond reseeds the entropy and
		// still yields an ID that sorts after everything already handed out.
		ms++
	}
}

// Valid reports whether s is a well-formed canonical ULID.
func Valid(s string) bool {
	_, err := oklog.ParseStrict(s)
	return err == nil
}
