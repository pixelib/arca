package p2

import "testing"

func TestParseArtifact(t *testing.T) {
	cases := []struct {
		name string
		path string
		want Artifact
		ok   bool
	}{
		{
			name: "bundle",
			path: "plugins/org.eclipse.emf.edit.ui_2.25.0.v20241015-0644.jar",
			want: Artifact{ClassifierBundle, "org.eclipse.emf.edit.ui", "2.25.0.v20241015-0644"},
			ok:   true,
		},
		{
			name: "bundle nested under a composite child",
			path: "releases/2024-12/202412041000/plugins/org.eclipse.jgit.pgm_7.1.0.202411261347-r.jar",
			want: Artifact{ClassifierBundle, "org.eclipse.jgit.pgm", "7.1.0.202411261347-r"},
			ok:   true,
		},
		{
			name: "source bundle keeps its suffix in the id",
			path: "plugins/org.eclipse.modisco.kdm.source.extension.source_1.5.5.v20240826-1153.jar",
			want: Artifact{ClassifierBundle, "org.eclipse.modisco.kdm.source.extension.source", "1.5.5.v20240826-1153"},
			ok:   true,
		},
		{
			name: "feature",
			path: "features/org.eclipse.viatra.addon.feature_2.9.1.202409200831.jar",
			want: Artifact{ClassifierFeature, "org.eclipse.viatra.addon.feature", "2.9.1.202409200831"},
			ok:   true,
		},
		{
			name: "binary has no extension",
			path: "binary/org.eclipse.platform_root_4.34.0.v20241128-0730",
			want: Artifact{ClassifierBinary, "org.eclipse.platform_root", "4.34.0.v20241128-0730"},
			ok:   true,
		},
		{name: "metadata is not an artifact", path: "releases/2024-12/artifacts.jar"},
		{name: "unknown directory", path: "downloads/something_1.0.0.jar"},
		{name: "bundle without a version separator", path: "plugins/org.eclipse.foo.jar"},
		{name: "bundle without the jar suffix", path: "plugins/org.eclipse.foo_1.0.0"},
		{name: "bare filename", path: "org.eclipse.foo_1.0.0.jar"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ParseArtifact(test.path)
			if ok != test.ok {
				t.Fatalf("ParseArtifact(%q) ok = %v, want %v", test.path, ok, test.ok)
			}
			if ok && got != test.want {
				t.Fatalf("ParseArtifact(%q) = %+v, want %+v", test.path, got, test.want)
			}
		})
	}
}

func TestMutable(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"releases/2024-12/p2.index", true},
		{"releases/2024-12/compositeContent.jar", true},
		{"releases/2024-12/compositeArtifacts.xml", true},
		{"releases/2024-12/202412041000/content.jar", true},
		{"releases/2024-12/202412041000/artifacts.xml.xz", true},
		{"releases/2024-12/202412041000/plugins/org.eclipse.jgit_7.1.0.jar", false},
		{"releases/2024-12/202412041000/features/org.eclipse.jgit.feature_7.1.0.jar", false},
		{"releases/2024-12/202412041000/binary/org.eclipse.platform_root_4.34.0", false},
	}

	for _, test := range cases {
		t.Run(test.path, func(t *testing.T) {
			if got := (Layout{}).Mutable(test.path); got != test.want {
				t.Fatalf("Mutable(%q) = %v, want %v", test.path, got, test.want)
			}
		})
	}
}

func TestArtifactDocumentClassification(t *testing.T) {
	cases := []struct {
		path       string
		index      bool
		artifacts  bool
		compressed bool
	}{
		{path: "a/p2.index", index: true},
		{path: "a/artifacts.jar", artifacts: true},
		{path: "a/artifacts.xml", artifacts: true},
		{path: "a/compositeArtifacts.xml", artifacts: true},
		{path: "a/compositeArtifacts.jar", artifacts: true},
		{path: "a/artifacts.xml.xz", compressed: true},
		{path: "a/content.jar"},
		{path: "a/content.xml.xz"},
		{path: "a/plugins/org.eclipse.jgit_7.1.0.jar"},
	}

	for _, test := range cases {
		t.Run(test.path, func(t *testing.T) {
			if got := IsIndex(test.path); got != test.index {
				t.Errorf("IsIndex(%q) = %v, want %v", test.path, got, test.index)
			}
			if got := IsArtifactDocument(test.path); got != test.artifacts {
				t.Errorf("IsArtifactDocument(%q) = %v, want %v", test.path, got, test.artifacts)
			}
			if got := IsCompressedArtifactDocument(test.path); got != test.compressed {
				t.Errorf("IsCompressedArtifactDocument(%q) = %v, want %v", test.path, got, test.compressed)
			}
		})
	}
}
