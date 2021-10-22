package server

import (
	"sync"

	"golang.org/x/time/rate"
)

// ipRateLimiter .
type ipRateLimiter struct {
	ips map[string]*rate.Limiter
	mu  *sync.RWMutex
	r   rate.Limit
	b   int
}

// newIPRateLimiter .
func newIPRateLimiter(r rate.Limit, b int) *ipRateLimiter {
	i := &ipRateLimiter{
		ips: make(map[string]*rate.Limiter),
		mu:  &sync.RWMutex{},
		r:   r,
		b:   b,
	}

	return i
}

// addIP creates a new rate limiter and adds it to the ips map,
// using the IP address as the key
func (i *ipRateLimiter) addIP(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter := rate.NewLimiter(i.r, i.b)

	i.ips[ip] = limiter

	return limiter
}

// getLimiter returns the rate limiter for the provided IP address if it exists.
// Otherwise calls AddIP to add IP address to the map
func (i *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	limiter, exists := i.ips[ip]

	if !exists {
		i.mu.Unlock()
		return i.addIP(ip)
	}

	i.mu.Unlock()

	return limiter
}
