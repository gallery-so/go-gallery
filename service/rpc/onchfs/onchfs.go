package onchfs

import (
	"fmt"
	"strings"
)

const FxhashGateway = "https://onchfs.fxhash2.xyz"

func IsOnchfsURL(u string) bool {
	return strings.HasPrefix(u, "onchfs://")
}

func BestGatewayNodeFrom(u string) string {
	if !IsOnchfsURL(u) {
		return u
	}
	return DefaultGatewayFrom(u)
}

func DefaultGatewayFrom(u string) string {
	return fmt.Sprintf("%s/%s", FxhashGateway, uriFrom(u))
}

func uriFrom(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimPrefix(u, "onchfs://")
	return u
}

func pathURL(host, uri string) string {
	return fmt.Sprintf("%s/%s", host, uri)
}
