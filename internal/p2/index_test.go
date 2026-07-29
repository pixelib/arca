package p2

import "testing"

func TestRewriteIndex(t *testing.T) {
	cases := []struct {
		name     string
		document string
		want     string
	}{
		{
			name: "the eclipse leaf loses only the compressed candidate",
			document: "version=1\n" +
				"metadata.repository.factory.order=content.xml.xz,content.xml,!\n" +
				"artifact.repository.factory.order=artifacts.xml.xz,artifacts.xml,!\n",
			want: "version=1\n" +
				"metadata.repository.factory.order=content.xml.xz,content.xml,!\n" +
				"artifact.repository.factory.order=artifacts.xml,!\n",
		},
		{
			// The entries name factories, and a composite site names composite
			// factories. Adding anything here sends p2 after a factory that
			// does not exist.
			name:     "a composite order is left alone",
			document: "version=1\nmetadata.repository.factory.order=compositeContent.xml\nartifact.repository.factory.order=compositeArtifacts.xml\n",
			want:     "version=1\nmetadata.repository.factory.order=compositeContent.xml\nartifact.repository.factory.order=compositeArtifacts.xml\n",
		},
		{
			// p2.index is a Java properties file, so '!' is often escaped.
			name:     "an escaped terminator is preserved",
			document: "artifact.repository.factory.order=artifacts.xml,\\!\nversion=1\n",
			want:     "artifact.repository.factory.order=artifacts.xml,\\!\nversion=1\n",
		},
		{
			name:     "an escaped terminator after a compressed candidate",
			document: "artifact.repository.factory.order=artifacts.xml.xz,artifacts.xml,\\!\n",
			want:     "artifact.repository.factory.order=artifacts.xml,\\!\n",
		},
		{
			name:     "an order left with only a terminator is dropped entirely",
			document: "version=1\nartifact.repository.factory.order=artifacts.xml.xz,!\n",
			want:     "version=1\n",
		},
		{
			name:     "an order left with only an escaped terminator is dropped entirely",
			document: "version=1\nartifact.repository.factory.order=artifacts.xml.xz,\\!\n",
			want:     "version=1\n",
		},
		{
			name:     "an order without a terminator keeps its shape",
			document: "artifact.repository.factory.order=artifacts.xml.xz,artifacts.xml\n",
			want:     "artifact.repository.factory.order=artifacts.xml\n",
		},
		{
			name:     "carriage returns survive",
			document: "version=1\r\nartifact.repository.factory.order=artifacts.xml.xz,artifacts.xml,!\r\n",
			want:     "version=1\r\nartifact.repository.factory.order=artifacts.xml,!\r\n",
		},
		{
			name:     "comments and unrelated keys are untouched",
			document: "# p2.index file to speed things up.\n#Thu Nov 28 08:14:13 UTC 2024\nversion=1\n",
			want:     "# p2.index file to speed things up.\n#Thu Nov 28 08:14:13 UTC 2024\nversion=1\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := string(RewriteIndex([]byte(test.document))); got != test.want {
				t.Fatalf("RewriteIndex() =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}
