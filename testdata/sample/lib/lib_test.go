package lib

import (
	"testing"

	"example.com/sample/testkit"
)

func TestLegacy(t *testing.T) { Legacy(); t.Helper() }
func unusedTestHelper()       {}
func ExampleLegacy()          { Legacy() }

type Mock struct{}

func (Mock) Work()          {}
func TestMock(t *testing.T) { Dispatch(Mock{}); t.Helper() }
func BenchmarkLegacy(b *testing.B) {
	for b.Loop() {
		Legacy()
	}
}
func FuzzLegacy(f *testing.F) { f.Add("input"); f.Fuzz(func(t *testing.T, s string) { Legacy() }) }

func TestNested(t *testing.T) { t.Run("case", func(t *testing.T) { SubtestOnly() }) }

type fakeJob struct{}

func (fakeJob) finish(int)    {}
func (fakeJob) label() string { return "fake" }

func TestRunJob(t *testing.T) { RunJob(fakeJob{}); SupportOnly(); testkit.Helper(); t.Helper() }
