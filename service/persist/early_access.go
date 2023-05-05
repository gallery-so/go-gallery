package persist

import "context"

type EarlyAccessRepository interface {
	IsAllowedByAddresses(context.Context, []ChainAddress) (bool, error)
}
