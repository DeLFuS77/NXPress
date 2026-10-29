package main

import "sync"

// the computer is kept awake while files are at work, unless the work is paused
var awake struct {
	mutex   sync.Mutex
	holders int
	paused  bool
	release func()
}

// blockSleep keeps the computer from going to sleep until the function it gives is called.
var blockSleep = preventSleep

// keepAwake keeps the computer from going to sleep while files are at work, until the function
// it gives is called; while the work is paused, it may sleep (see pauseAwake).
func keepAwake() func() {
	awake.mutex.Lock()
	defer awake.mutex.Unlock()
	awake.holders++
	holdAwake()
	var once sync.Once
	return func() {
		once.Do(func() {
			awake.mutex.Lock()
			defer awake.mutex.Unlock()
			awake.holders--
			holdAwake()
		})
	}
}

// pauseAwake lets the computer sleep while the work is paused, and keeps it awake again once the
// work goes on.
func pauseAwake(paused bool) {
	awake.mutex.Lock()
	defer awake.mutex.Unlock()
	awake.paused = paused
	holdAwake()
}

// holdAwake keeps the computer awake while work wants it so, and lets it sleep once not; the
// mutex is held.
func holdAwake() {
	wanted := awake.holders > 0 && !awake.paused
	switch {
	case wanted && awake.release == nil:
		awake.release = blockSleep()
	case !wanted && awake.release != nil:
		awake.release()
		awake.release = nil
	}
}
