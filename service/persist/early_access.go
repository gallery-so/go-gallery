package persist

import "context"

type EarlyAccessRepository interface {
	IsAllowedByAddresses(context.Context, []Address) (bool, error)
}
