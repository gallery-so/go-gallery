package event

import (
	"github.com/mikeydub/go-gallery/service/persist"
)

type EventMessage struct {
	ID      persist.DBID
	EventID persist.EventID
}
