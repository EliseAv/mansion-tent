package share

type Set[V comparable] struct {
	data map[V]struct{}
}

func (s *Set[V]) m() map[V]struct{} {
	if s.data == nil {
		s.data = make(map[V]struct{})
	}
	return s.data
}

func (s *Set[V]) Add(v V) {
	s.m()[v] = struct{}{}
}

func (s *Set[V]) Remove(v V) {
	delete(s.m(), v)
}

func (s *Set[V]) Len() int {
	return len(s.data)
}
