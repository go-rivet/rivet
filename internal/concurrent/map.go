package concurrent

import "sync"

// MapOf is a type-safe generic concurrent map wrapper around sync.Map.
type MapOf[K comparable, V any] struct {
	mu sync.Map
}

func NewMapOf[K comparable, V any]() *MapOf[K, V] {
	return &MapOf[K, V]{}
}

func (m *MapOf[K, V]) Store(key K, value V) {
	m.mu.Store(key, value)
}

func (m *MapOf[K, V]) Load(key K) (value V, ok bool) {
	val, ok := m.mu.Load(key)
	if !ok {
		return value, false // Returns zero value
	}
	return val.(V), true // Type assertion is 100% safe due to generics
}

func (m *MapOf[K, V]) Delete(key K) {
	m.mu.Delete(key)
}

func (m *MapOf[K, V]) Range(f func(key K, value V) bool) {
	m.mu.Range(func(k, v any) bool {
		return f(k.(K), v.(V))
	})
}
