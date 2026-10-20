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

func obsoleteSetup() { _ = setupState{calls: 1} }

func TestObsolete(t *testing.T) { obsoleteSetup(); Obsolete(); t.Helper() }

// TestMixed tests live code but also uses Obsolete, so it survives and needs an edit.
func TestMixed(t *testing.T) { Live(); Obsolete(); t.Helper() }

type unusedFixture struct{}

type setupState struct{ calls int }

func TestTestOnlyKind(t *testing.T) { _ = testOnlyKind{}; Live(); t.Helper() }

func TestPreset(t *testing.T) {
	if PresetMorning == PresetNoon {
		t.Fatal("presets collide")
	}
}
