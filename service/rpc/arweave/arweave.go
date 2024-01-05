package arweave

import (
	"fmt"
	"strings"

	"github.com/everFinance/goar"
)

const ArweaveHost = "https://arweave.net"

func NewClient() *goar.Client { return goar.NewClient(ArweaveHost) }

func IsArweaveURL(u string) bool {
	return strings.HasPrefix(u, "ar://") || strings.HasPrefix(u, "arweave://")
}

func BestGatewayNodeFrom(u string) string {
	if !IsArweaveURL(u) {
		return u
	}
	return DefaultGatewayFrom(u)
}

func DefaultGatewayFrom(u string) string {
	return pathURL(ArweaveHost, uriFrom(u))
}

func uriFrom(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimPrefix(u, "arweave://")
	u = strings.TrimPrefix(u, "ar://")
	return u
}

func pathURL(host, uri string) string {
	return fmt.Sprintf("%s/%s", host, uri)
}
