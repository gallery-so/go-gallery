package throttle

import (
	"context"
	"time"

	"github.com/mikeydub/go-gallery/service/redis"
)

// ErrThrottleLocked is returned when the throttle is already locked for a given key. We do not block
// with a lock, but return this error instead.
type ErrThrottleLocked struct {
	Key string
}

// Locker is a sort of mutex that should be used to ensure a task is not being done twice at the same time
// across the application. It useses a memstore to store empty data with a given key.
// The key will also be stored with the given expiry to ensure no state is locked indefinitely (unless the expiry is set to allow that).
type Locker struct {
	memstore *redis.Cache
	expiry   time.Duration
}

// NewThrottleLocker creates a new throttle locker
func NewThrottleLocker(memstore *redis.Cache, expiry time.Duration) *Locker {
	return &Locker{
		memstore: memstore,
		expiry:   expiry,
	}
}

// Lock locks a key in the throttle locker and will return ErrThrottleLocked if the key is already locked
func (t *Locker) Lock(ctx context.Context, key string) error {

	if isLocked, err := t.IsLocked(ctx, key); err != nil {
		return err
	} else if isLocked {
		return ErrThrottleLocked{Key: key}
	}

	err := t.memstore.Set(ctx, key, []byte{}, t.expiry)
	if err != nil {
		return err
	}

	return nil
}

// Unlock unlocks a key in the throttle locker, despite it being locked
func (t *Locker) Unlock(ctx context.Context, key string) error {

	err := t.memstore.Delete(ctx, key)
	if err != nil {
		return err
	}

	return nil
}

// IsLocked checks if a key is locked
func (t *Locker) IsLocked(ctx context.Context, key string) (bool, error) {

	_, err := t.memstore.Get(ctx, key)
	if err != nil {
		if _, ok := err.(redis.ErrKeyNotFound); ok {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (e ErrThrottleLocked) Error() string {
	return "throttle locked: " + e.Key
}
