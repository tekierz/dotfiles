package ui

import (
	"sync"
	"testing"
)

// TestConcurrentNewAppNoRace constructs many Apps concurrently. Each NewApp
// calls SetTheme, which mutates package-global theme/style state. Before the
// fix this races (write-write on the globals) and fails under `go test -race`.
// In production there is exactly one App on a single goroutine, so this only
// guards against the parallel-test construction storm reintroducing the race.
func TestConcurrentNewAppNoRace(t *testing.T) {
	const goroutines = 16

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = NewApp(true /* skipIntro */)
		}()
	}
	wg.Wait()
}

// TestConcurrentSetThemeNoRace exercises SetTheme directly from many
// goroutines, mirroring concurrent theme-picker live previews / construction.
func TestConcurrentSetThemeNoRace(t *testing.T) {
	themes := make([]string, 0, len(ThemePalettes))
	for name := range ThemePalettes {
		themes = append(themes, name)
	}
	if len(themes) == 0 {
		t.Fatal("no theme palettes registered")
	}

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				SetTheme(themes[(idx+j)%len(themes)])
			}
		}(i)
	}
	wg.Wait()
}
