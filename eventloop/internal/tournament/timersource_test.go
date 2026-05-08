package tournament

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTimerComponentSourcesAuthenticate(t *testing.T) {
	want := map[string][]componentSourceIdentity{
		"timer.value-safe-task-v1": {
			{ProvenanceKind: "commit", Path: "internal/alternateone/loop.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "92f6a712ec79f613accc9a08bafe04f48839f4ee", SHA256: "556318602f3ccfa9e43dfe4fb83936202093e8b12e7fa330b770028fc5f0f92f"},
			{ProvenanceKind: "commit", Path: "internal/alternateone/chunk.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "c533e23f14c1eb024901f3ee70a94d338bd53681", SHA256: "1a4b368960d349a42e97c288dee9f9ac361f5aca67140f3ae98f5540f67e86a0"},
		},
		"timer.value-task-v1": {
			{ProvenanceKind: "commit", Path: "internal/alternatethree/loop.go", OriginCommit: "b77a13cf646877598039f2446673ad981486d58e", OriginBlob: "d5676de5ac43f65bfca44d386e0d4756c44b364d", SHA256: "35a30baac7985fb1756bbaa39f1efcc3daf71e347fb951329fd5be6f3755b1ec"},
			{ProvenanceKind: "commit", Path: "internal/alternatethree/ingress.go", OriginCommit: "b77a13cf646877598039f2446673ad981486d58e", OriginBlob: "516268d672af199e933b44ac794c15e6b518fdfd", SHA256: "f8e5e231a30c2fe9a1ead71eb932ab4a8a3278125f48becb74f283811fa78145"},
		},
		"timer.pointer-deadline-v1": {
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "506d6643cc1d45b1da156096870991ecb30b8847", OriginBlob: "f10d7c782a8785b4fcfdb61f75f3a6a21a378cc2", SHA256: "f213677a818476fadac5f44643f5c4fea6d5a550f75988d0b846ee27e1fa3d28"},
		},
		"timer.pointer-ref-v2": {
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "cc005d72b329fd91eee03aac62ba7188df7c91b9", OriginBlob: "b2eab189b1104aea1f58b15ace6497599cd31e08", SHA256: "364518a66b48b4180a23103eb9b8bcaf4433d5a919de8f6f89d76ef472349aa6"},
		},
		"timer.pointer-tick-stall-v3a": {
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "0def02e2ff987be01a38d237a5d84dae256a85ac", OriginBlob: "a9f343f0893478ea0591bc50c0fc159e13f12f4e", SHA256: "5913e0841e319c0b746adceecfb0fd8fc0f58a485b5c0be390ee28ec1e6cb1c4"},
		},
		"timer.pointer-tick-defer-v3b": {
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "0bc4ad0ae702ce2205615c31dcf37992d67ff9c8", OriginBlob: "30af1e53f31a01f035e68b0f58cf66f46d6de637", SHA256: "e0f4a10749f7da5cbcdb89aed6b3df7386b035d611898c73eb410272a6c7d636"},
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "802436f7fa69ff99842a58f5583d24b75c4b753e", OriginBlob: "ceef22c1e5b7af72f905a61e35a0d77062e0fa52", SHA256: "1791227841921341f49ce2f587bbb580f39f981c8dc6828f2718141f6a2ac9af"},
		},
		"timer.bucket-tick-v1": {
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "27b93ec32938ca838e1519bc8e17b6852d7df449", OriginBlob: "657b88c500022b09fef4acff47b7cbbbde0e0d71", SHA256: "534eabbcdd10a69d5ea303e389a04e332debabc83649f7da053b937fdf6b80e8"},
		},
		"timer.bucket-retire-v1-1": {
			{ProvenanceKind: "commit", Path: "loop.go", OriginCommit: "c8e744e4867c351d5b83e438fd2cb438c9b04898", OriginBlob: "51bd5698a6c7a05e1df6e73505a8463b2c497a9e", SHA256: "7f0ff7e2b5c0f2daf638116237f74101ed6b580db0256430128f0c208df697c0"},
		},
		"timer.bucket-phase-v2": {
			{ProvenanceKind: "archived-index-candidate", Path: "timer.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "919a22141f9511158c981abc519063e3c07e05bd", SHA256: "9a7e41d739c47259533738401ccd201320b37fea8cd9e66cad5cddb2bc57ef8f"},
			{ProvenanceKind: "archived-index-candidate", Path: "timercancel.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "065971859d43a3d775ad9a95b1cbc008cfd075b8", SHA256: "10abb483e6817c0fb48ff602e5213daaddd284d186d4486358399fc5ce6be955"},
			{ProvenanceKind: "archived-index-candidate", Path: "timerid.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "eb0e42f8bf220c7f62992500583c2bfa6f171a69", SHA256: "50d8df638566c95e7f255710957be536929af238aa5c5380b1f077149331a8d2"},
			{ProvenanceKind: "archived-index-candidate", Path: "loop.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "5c316e52dc7d0bf39c402b433e8c356cfe6ef58d", SHA256: "18cc725d1755b0d641be17344fe77a6198b5aa02bdf87b4855c6e3b8956751ba"},
			{ProvenanceKind: "archived-index-candidate", Path: "scheduler.go", BaseRevision: "469fd952ed251edc7ea1d2bb0faf4e04fc94dd88", OriginBlob: "4acd4c3910ea93b0abd5130b2d8dc72adfbb2eea", SHA256: "7272412ffc0421026037108c27df349bc803ff63b4ebbf9d9c834702cdff7391"},
		},
	}
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range timerComponentRegistry {
		expected, ok := want[descriptor.ID]
		if !ok {
			t.Errorf("unexpected timer descriptor %q", descriptor.ID)
			continue
		}
		if !reflect.DeepEqual(descriptor.Sources, expected) {
			t.Errorf("descriptor %q sources = %+v, want %+v", descriptor.ID, descriptor.Sources, expected)
			continue
		}
		for _, source := range descriptor.Sources {
			name := strings.Join([]string{descriptor.ID, source.ProvenanceKind, source.OriginBlob, strings.ReplaceAll(source.Path, "/", "_")}, "/")
			t.Run(name, func(t *testing.T) {
				var payload []byte
				var blob string
				switch source.ProvenanceKind {
				case "commit":
					if source.OriginCommit == "" || source.BaseRevision != "" {
						t.Fatalf("commit source has invalid authority fields: %+v", source)
					}
					commit := strings.TrimSpace(string(runComponentGit(t, repository, "rev-parse", source.OriginCommit+"^{commit}")))
					if commit != source.OriginCommit {
						t.Fatalf("origin commit = %s, want %s", commit, source.OriginCommit)
					}
					object := source.OriginCommit + ":eventloop/" + source.Path
					payload = runComponentGit(t, repository, "show", object)
					blob = strings.TrimSpace(string(runComponentGit(t, repository, "rev-parse", object)))
				case "archived-index-candidate":
					if source.OriginCommit != "" || source.BaseRevision == "" || descriptor.SourceArchive == (timerSourceArchive{}) {
						t.Fatalf("archived index source has invalid authority fields: %+v", source)
					}
					if source.BaseRevision != descriptor.SourceArchive.BaseRevision {
						t.Fatalf("source base revision = %s, want archive base %s", source.BaseRevision, descriptor.SourceArchive.BaseRevision)
					}
					return // Exact bytes are authenticated from the isolated reconstruction test.
				default:
					t.Fatalf("unsupported provenance kind %q", source.ProvenanceKind)
				}
				if blob != source.OriginBlob {
					t.Errorf("blob = %s, want %s", blob, source.OriginBlob)
				}
				if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != source.SHA256 {
					t.Errorf("SHA-256 = %s, want %s", got, source.SHA256)
				}
			})
		}
	}
}
