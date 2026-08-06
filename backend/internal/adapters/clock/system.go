// Package clock holds adapters for the ports.Clock port.
package clock

import (
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

type System struct{}

var _ ports.Clock = System{}

func NewSystem() System { return System{} }

func (System) Now() time.Time { return time.Now() }
