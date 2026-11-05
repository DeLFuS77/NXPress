package system

import (
	"os"
	"testing"
)

// The drive of the test folder answers, when Windows tells it.
func TestSeekPenalty(t *testing.T) {
	penalty, known := SeekPenalty(os.TempDir())
	t.Logf("%s: penalty %v, known %v", os.TempDir(), penalty, known)
	for _, path := range []string{`C:\`, `D:\`} {
		penalty, known := SeekPenalty(path)
		t.Logf("%s: penalty %v, known %v", path, penalty, known)
	}
}
