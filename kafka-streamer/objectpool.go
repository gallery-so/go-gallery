package main

type ObjectPool[T any] struct {
	pool []*T
}

func NewObjectPool[T any](initialCapacity int) *ObjectPool[T] {
	return &ObjectPool[T]{pool: make([]*T, 0, initialCapacity)}
}

// Get an object from the pool
func (p *ObjectPool[T]) Get() *T {
	var obj *T
	if len(p.pool) > 0 {
		// Pop the last element
		obj = p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
	} else {
		// Return a new zero value of T if the pool is empty
		obj = new(T)
	}
	return obj
}

func (p *ObjectPool[T]) GetMany(count int) []*T {
	var objects []*T
	for i := 0; i < count; i++ {
		objects = append(objects, p.Get())
	}
	return objects
}

func (p *ObjectPool[T]) Put(obj *T) {
	p.pool = append(p.pool, obj)
}

func (p *ObjectPool[T]) PutMany(objects []*T) {
	p.pool = append(p.pool, objects...)
}
