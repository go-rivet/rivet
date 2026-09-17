package orderedmap

import "iter"

type Element[K comparable, V any] struct {
	Key   K
	Value V
}

type entry[K comparable, V any] struct {
	key       K
	value     V
	tombstone bool
}

type OrderedMap[K comparable, V any] struct {
	lookup     map[K]int
	entries    []entry[K, V]
	tombstones int
}

func NewOrderedMap[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		lookup:  make(map[K]int, 4),
		entries: make([]entry[K, V], 0, 4), // Baseline buffer prevents instant first-append growth
	}
}

func NewOrderedMapWithCapacity[K comparable, V any](capacity int) *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		lookup:  make(map[K]int, capacity),
		entries: make([]entry[K, V], 0, capacity),
	}
}

func (m *OrderedMap[K, V]) ensureLookup() {
	if len(m.entries) == 0 {
		if m.lookup == nil {
			m.lookup = make(map[K]int, 4)
		}
		return
	}
	if m.lookup == nil || (len(m.lookup) == 0 && len(m.entries) > 0) {
		if m.lookup == nil {
			m.lookup = make(map[K]int, len(m.entries))
		}
		for i, ent := range m.entries {
			if !ent.tombstone {
				m.lookup[ent.key] = i
			}
		}
	}
}

func (m *OrderedMap[K, V]) Set(key K, value V) bool {
	m.ensureLookup()

	if index, ok := m.lookup[key]; ok {
		m.entries[index].value = value
		return false
	}
	if m.tombstones > 0 && m.tombstones >= len(m.entries)-m.tombstones {
		m.compact()
	}

	currentLen := len(m.entries)
	m.lookup[key] = currentLen

	// Reverted to Go's native assembler-optimized append allocation mechanics
	m.entries = append(m.entries, entry[K, V]{key: key, value: value})
	return true
}

func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	if len(m.entries) == 0 {
		var zero V
		return zero, false
	}
	m.ensureLookup()
	if index, ok := m.lookup[key]; ok {
		return m.entries[index].value, true
	}
	var zero V
	return zero, false
}

func (m *OrderedMap[K, V]) Has(key K) bool {
	if len(m.entries) == 0 {
		return false
	}
	m.ensureLookup()
	_, ok := m.lookup[key]
	return ok
}

func (m *OrderedMap[K, V]) Delete(key K) bool {
	if len(m.entries) == 0 {
		return false
	}
	m.ensureLookup()

	index, ok := m.lookup[key]
	if !ok {
		return false
	}
	delete(m.lookup, key)
	m.entries[index] = entry[K, V]{tombstone: true}
	m.tombstones++

	for len(m.entries) > 0 && m.entries[len(m.entries)-1].tombstone {
		last := len(m.entries) - 1
		m.entries[last] = entry[K, V]{}
		m.entries = m.entries[:last]
		m.tombstones--
	}
	if m.tombstones > 0 && m.tombstones >= len(m.entries)-m.tombstones {
		m.compact()
	}
	return true
}

func (m *OrderedMap[K, V]) Len() int {
	if m == nil || len(m.entries) == 0 {
		return 0
	}
	return len(m.entries) - m.tombstones
}

func (m *OrderedMap[K, V]) ForEach(yield func(K, V) bool) {
	if m == nil {
		return
	}
	for index := range m.entries {
		item := &m.entries[index]
		if !item.tombstone && !yield(item.key, item.value) {
			return
		}
	}
}

func (m *OrderedMap[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		if m == nil {
			return
		}
		for index := range m.entries {
			item := &m.entries[index]
			if !item.tombstone && !yield(item.key, item.value) {
				return
			}
		}
	}
}

func (m *OrderedMap[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		if m == nil {
			return
		}
		for index := range m.entries {
			item := &m.entries[index]
			if !item.tombstone && !yield(item.key) {
				return
			}
		}
	}
}

func (m *OrderedMap[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		if m == nil {
			return
		}
		for index := range m.entries {
			item := &m.entries[index]
			if !item.tombstone && !yield(item.value) {
				return
			}
		}
	}
}

func (m *OrderedMap[K, V]) Copy() *OrderedMap[K, V] {
	if m == nil {
		return nil
	}

	// OPTIMIZATION: Match exact entry slices bounds, but don't overallocate capacity
	// slots on trailing unused fields during a copy sequence.
	copied := &OrderedMap[K, V]{
		entries:    make([]entry[K, V], len(m.entries)),
		tombstones: m.tombstones,
	}
	copy(copied.entries, m.entries)
	return copied
}

func (m *OrderedMap[K, V]) compact() {
	if m.lookup == nil {
		m.lookup = make(map[K]int)
	}
	clear(m.lookup)
	writeIndex := 0
	for readIndex := range m.entries {
		if m.entries[readIndex].tombstone {
			continue
		}
		if writeIndex != readIndex {
			m.entries[writeIndex] = m.entries[readIndex]
		}
		m.lookup[m.entries[writeIndex].key] = writeIndex
		writeIndex++
	}
	clear(m.entries[writeIndex:])
	m.entries = m.entries[:writeIndex]
	m.tombstones = 0
}
