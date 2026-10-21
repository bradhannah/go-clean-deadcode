// Package panel has struct fields only reflection could use.
package panel

import "example.com/reflection/bind"

// Panel is registered with bind: nothing in source uses signal or writes ready,
// but bind can see and set both.
type Panel struct {
	signal int
	ready  bool
}

func (p *Panel) isReady() bool { return p.ready }

// plain is never given to reflection, so its unused field is a finding.
type plain struct{ used, spare int }

// Open registers a panel and reports whether it is ready.
func Open() bool {
	_ = bind.Register[Panel]()
	return (&Panel{}).isReady() && plain{used: 1}.used == 1
}
