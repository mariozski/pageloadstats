package pageloadstats

import (
	"fmt"
	"sync"

	phantomjs "github.com/urturn/go-phantomjs"
)

type workersPool struct {
	mu      sync.Mutex
	used    []bool
	workers []*phantomjs.Phantom
	size    int
}

// Close method should be used at the end to get rid of
// resources that LoadTimer has used.
func (p *workersPool) Close() {
	for i := 0; i < (*p).size; i++ {
		if (*p).workers[i] == nil {
			continue
		}

		(*p).workers[i].Exit()
	}
}

func (p *workersPool) getPhantom() (*phantomjs.Phantom, error) {
	(*p).mu.Lock()
	defer (*p).mu.Unlock()

	for i := 0; i < (*p).size; i++ {
		if !(*p).used[i] {
			(*p).used[i] = true
			return (*p).workers[i], nil
		}
	}

	return nil, fmt.Errorf("Could not get phantom process")
}

func (p *workersPool) releasePhantom(phantom *phantomjs.Phantom) error {
	(*p).mu.Lock()
	defer (*p).mu.Unlock()

	for i := 0; i < (*p).size; i++ {
		if (*p).workers[i] == phantom {
			(*p).used[i] = false
			return nil
		}
	}

	return fmt.Errorf("Could not release phantom process")
}
