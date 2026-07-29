package p2

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"equal", "1.2.3", "1.2.3", 0},
		{"major", "2.0.0", "1.9.9", 1},
		{"minor", "1.2.0", "1.10.0", -1},
		{"micro", "1.2.10", "1.2.9", 1},
		{"missing segments default to zero", "1", "1.0.0", 0},
		{"qualifier beats none", "1.0.0.v20241015", "1.0.0", 1},
		{"qualifiers order bytewise", "1.0.0.v20241015", "1.0.0.v20241016", -1},
		{"numeric segments beat qualifier", "1.0.1.a", "1.0.0.z", 1},
		{"maven suffix keeps its micro", "1.2.3-SNAPSHOT", "1.2.2", 1},
		{"maven suffix orders after release", "1.2.3-SNAPSHOT", "1.2.3", 1},
		{"jgit style release", "7.1.0.202411261347-r", "7.1.0.202411261346-r", 1},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := sign(CompareVersions(test.a, test.b)); got != test.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
			}
			if got := sign(CompareVersions(test.b, test.a)); got != -test.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", test.b, test.a, got, -test.want)
			}
		})
	}
}

func TestPrerelease(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"1.0.0", false},
		{"1.0.0.v20241015-0644", false},
		{"7.1.0.202411261347-r", false},
		{"1.0.0.qualifier", true},
		{"2.25.0-SNAPSHOT", true},
		{"1.0.0.SNAPSHOT", true},
	}

	for _, test := range cases {
		t.Run(test.version, func(t *testing.T) {
			if got := Prerelease(test.version); got != test.want {
				t.Fatalf("Prerelease(%q) = %v, want %v", test.version, got, test.want)
			}
		})
	}
}

func sign(value int) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	default:
		return 0
	}
}
