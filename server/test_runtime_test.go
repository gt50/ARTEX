package server

import "context"

// Tests must join task loops before closing their database and temporary trees.
// A Background context alone leaves writers racing testing.TempDir cleanup.
func newTestServer(parent context.Context, m *Manager, skills, data, key string) (*Server, func()) {
	ctx, cancel := context.WithCancel(parent)
	s := New(ctx, m, skills, data, key)
	return s, func() {
		cancel()
		for _, task := range m.List() {
			s.engine.StopTask(task.ID)
		}
	}
}
