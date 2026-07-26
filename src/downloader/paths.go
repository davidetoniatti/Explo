package downloader

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// pathClaims records the output paths a run has already taken, so two tracks that
// render the same path cannot write over each other.
//
// Nothing guarantees the paths are unique. A PATH_TEMPLATE may omit a per-track
// placeholder, and the default name is built from the title and artist alone, which one
// playlist can legitimately repeat. Downloads for a service run concurrently, so
// without this two goroutines open and write the same file and one truncates the other
// mid-write; slskd migrations run sequentially, where the second track silently
// replaces the first instead.
type pathClaims struct {
	mu      sync.Mutex
	claimed map[string]struct{}
}

func newPathClaims() *pathClaims {
	return &pathClaims{claimed: make(map[string]struct{})}
}

// claim reserves path and returns it, or, when it is already taken, the first free
// variant with a numeric suffix before the extension. Both tracks are then kept, which
// is preferable to dropping one or overwriting it. The check and the reservation are one
// locked step, since concurrent callers would otherwise both see the path as free.
func (p *pathClaims) claim(path string) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	candidate := path
	for n := 2; ; n++ {
		if _, taken := p.claimed[candidate]; !taken {
			p.claimed[candidate] = struct{}{}
			return candidate
		}
		candidate = suffixPath(path, n)
	}
}

// suffixPath inserts "-n" before the extension, turning "01 - Intro.flac" into
// "01 - Intro-2.flac".
func suffixPath(path string, n int) string {
	ext := filepath.Ext(path)
	return fmt.Sprintf("%s-%d%s", strings.TrimSuffix(path, ext), n, ext)
}
