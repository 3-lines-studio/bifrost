package usecase

import "sync"

type singleflightGroup struct {
	mu sync.Mutex
	m  map[string]*singleflightCall
}

type singleflightCall struct {
	wg  sync.WaitGroup
	err error
}

func (g *singleflightGroup) Do(key string, fn func() error) error {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[string]*singleflightCall)
	}
	if c, ok := g.m[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.err
	}
	c := &singleflightCall{}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.m, key)
	g.mu.Unlock()

	return c.err
}
