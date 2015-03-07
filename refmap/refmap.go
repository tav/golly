// Public Domain (-) 2011-2015 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package refmap provides a map-like object so integer refs can be used in
// place of long strings.
package refmap

type value struct {
	count int
	s     string
}

type Map struct {
	lastRef uint64
	refs    map[uint64]value
	strings map[string]uint64
}

func (m *Map) Create(s string) uint64 {
	ref, found := m.strings[s]
	if found {
		return ref
	}
	m.lastRef += 1
	ref = m.lastRef
	m.strings[s] = ref
	m.refs[ref] = value{s: s}
	return ref
}

func (m *Map) Decr(s string, by int) {
	ref, found := m.strings[s]
	if !found {
		return
	}
	value := m.refs[ref]
	value.count -= by
	if value.count <= 0 {
		delete(m.strings, value.s)
		delete(m.refs, ref)
	}
}

func (m *Map) DecrRef(ref uint64, by int) {
	value, found := m.refs[ref]
	if found {
		value.count -= by
		if value.count <= 0 {
			delete(m.strings, value.s)
			delete(m.refs, ref)
		}
	}
}

func (m *Map) Delete(s string) {
	ref, found := m.strings[s]
	if !found {
		return
	}
	delete(m.strings, s)
	delete(m.refs, ref)
}

func (m *Map) DeleteRef(ref uint64) {
	value, found := m.refs[ref]
	if !found {
		return
	}
	delete(m.strings, value.s)
	delete(m.refs, ref)
}

func (m *Map) Get(s string) uint64 {
	if ref, found := m.strings[s]; found {
		return ref
	}
	return 0
}

func (m *Map) Incr(s string, by int) uint64 {
	ref, found := m.strings[s]
	if found {
		value := m.refs[ref]
		value.count += by
		return ref
	}
	m.lastRef += 1
	ref = m.lastRef
	m.strings[s] = ref
	m.refs[ref] = value{count: by, s: s}
	return ref
}

func (m *Map) IncrRef(ref uint64, by int) {
	value, found := m.refs[ref]
	if found {
		value.count += by
	}
}

func (m *Map) Lookup(ref uint64) (string, bool) {
	if value, found := m.refs[ref]; found {
		return value.s, true
	}
	return "", false
}

func New() *Map {
	return &Map{
		lastRef: 0,
		refs:    map[uint64]value{},
		strings: map[string]uint64{},
	}
}

func StartingFrom(start uint64) *Map {
	return &Map{
		lastRef: start,
		refs:    map[uint64]value{},
		strings: map[string]uint64{},
	}
}
