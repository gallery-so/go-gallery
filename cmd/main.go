package main

import (
	"fmt"
	"net/url"
	"strings"
)

func main() {
	// supposed to be https://remote-image.decentralized-content.com/image?url=https%3A%2F%2Fipfs.decentralized-content.com%2Fipfs%2Fbafybeibzyxpeoqidpjdwagztoa4lkrrcwalprwnuugghkwr7gf5qdxdnga&w=1080&q=75

	it := "ipfs://bafybeibzyxpeoqidpjdwagztoa4lkrrcwalprwnuugghkwr7gf5qdxdnga"
	afterIPFS := strings.TrimPrefix(it, "ipfs://")
	fallbackFormat, _ := url.Parse("https://remote-image.decentralized-content.com/image?w=1080&q=75")
	ipfsFallbackURLFormat := "https://ipfs.decentralized-content.com/ipfs/%s"
	u := fmt.Sprintf(ipfsFallbackURLFormat, afterIPFS)
	q := fallbackFormat.Query()
	q.Set("url", u)
	fallbackFormat.RawQuery = q.Encode()
	fmt.Println(fallbackFormat.String())
}
