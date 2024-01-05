package onchfs

import (
	"strings"
)

func IsOnchfsURL(u string) bool {
	return strings.HasPrefix(u, "onchfs://")
}

func BestGatewayNodeFrom(u string) string {
	panic("not implemented")
}
