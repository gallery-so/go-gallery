package sort

type lt interface{ Less(j any) bool }
type Heap[T lt] []T

func (h Heap[T]) Len() int           { return len(h) }
func (h Heap[T]) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *Heap[T]) Push(s any)        { *h = append(*h, s.(T)) }
func (h Heap[T]) Less(i, j int) bool { return h[i].Less(h[j]) }

func (h *Heap[T]) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old = nil
	*h = old[:n-1]
	return item
}
