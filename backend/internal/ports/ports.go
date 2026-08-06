// Package ports declares the interfaces that business logic depends on.
// Infrastructure lives behind these; adapters are injected at boot.
package ports

import "time"

// Clock is the demonstration port: it makes "now" injectable and testable.
type Clock interface {
	Now() time.Time
}
