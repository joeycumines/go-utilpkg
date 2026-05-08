package tournament

import "testing"

func TestTimerReferenceMaterializationArchiveReconstructsExactAuthority(t *testing.T) {
	want := timerReferenceComponentArchive()
	if want != (timerReferenceMaterializationArchive{
		PatchPath:         "revisions/candidates/0004-timer-reference-materializations.patch",
		PatchSHA256:       "f11f2922f92bb7755e41f5b6c8cff5fd780c2726af08c8eef7ae5829508e904e",
		PatchBytes:        13791,
		EmptyTree:         "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		ReconstructedTree: "a1ebdf3e3cf109c1ce40870a8ce06b8c6cbe5de1",
	}) {
		t.Fatalf("materialization archive = %+v", want)
	}
	for _, descriptor := range timerReferenceDescriptors() {
		if descriptor.MaterializationArchive != want {
			t.Errorf("descriptor %q archive = %+v", descriptor.ID, descriptor.MaterializationArchive)
		}
	}

	verifyMaterializationArchive(t, timerReferenceComponentArchiveSpec())
}

func timerReferenceComponentArchiveSpec() materializationArchiveSpec {
	return materializationArchiveSpec{
		files: []materializationArchiveFile{
			{"eventloop/internal/tournament/component/timerrefint32/core.go", "100644", "ce9c1647e14504f00c04526d12cbdf7b056d2e15", "cfb6fca35731b9d304378959aa0133d222c46df31f794c30635bff9d45641554", 1902},
			{"eventloop/internal/tournament/component/timerrefint32/core_test.go", "100644", "eed83fbb2730a37028e974803bbafebc826d4374", "093be0a74296f7ecf9293c35ee82b5e71fe9a2f89c1230a78336fccec9699588", 3394},
			{"eventloop/internal/tournament/component/timerrefint32/layout64_test.go", "100644", "493531f8c200c36a18e90e0127d5d717ed0a9bd3", "e9ff7208dc3c04b2a675a50ffb899b7233098518226bccf3fce297096db9b07f", 654},
			{"eventloop/internal/tournament/component/timerrefint64/core.go", "100644", "94a0ee8300fb6d672eb23a5b4f0ddab4e9d312d0", "77de06b7ce9759c0642dd72cbd234fce4ca2e362b985360a9eb8f5a00eb20dad", 1826},
			{"eventloop/internal/tournament/component/timerrefint64/core_test.go", "100644", "a28ee19cd2213f3b83f91fdb0ff6ba48a033f560", "95c774062e26edc9bd35f8fd716f5f6e1b9cbd6c791ef50f85f6850a84e08673", 3112},
			{"eventloop/internal/tournament/component/timerrefint64/layout64_test.go", "100644", "e221c78c85278a353395fb4fb9575999c828a38f", "da96a121b4575a9d96ec2be5d472f51f850c0f63ddc7c86402d3f85ccb8ca6a8", 654},
		},
		roots: []string{
			"eventloop/internal/tournament/component/timerrefint32",
			"eventloop/internal/tournament/component/timerrefint64",
		},
		archive:      timerReferenceComponentArchive(),
		id:           "0004",
		objectFormat: "sha1",
		patchFormat:  materializationArchivePatchAbbrev7,
		pathCount:    6,
		payloadBytes: 11542,
	}
}
