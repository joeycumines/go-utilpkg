package tournament

import "testing"

func TestTimerReferenceConsideredStrategyArchiveReconstructsExactAuthority(t *testing.T) {
	want := timerReferenceConsideredArchive()
	if want != (timerReferenceMaterializationArchive{
		PatchPath:         "revisions/candidates/0005-timer-reference-considered-strategies.patch",
		PatchSHA256:       "7c090dafd52ab00ab8dfa35dbda1a7111957791cba4e48ea7b039e10d6a913c4",
		PatchBytes:        23487,
		EmptyTree:         "4b825dc642cb6eb9a060e54bf8d69288fbee4904",
		ReconstructedTree: "a474db0f740db26f05e40f54dc09d405550c3e78",
	}) {
		t.Fatalf("considered-strategy archive = %+v", want)
	}
	for _, descriptor := range timerReferenceStrategyDescriptors() {
		if descriptor.MaterializationArchive != want {
			t.Errorf("descriptor %q archive = %+v", descriptor.ID, descriptor.MaterializationArchive)
		}
	}

	verifyMaterializationArchive(t, materializationArchiveSpec{
		files: []materializationArchiveFile{
			{"eventloop/internal/tournament/component/timerrefalwayssubmit/core.go", "100644", "0bbfd8572c2334f8f5cb7640ecc43194981574a4", "cff807587d2a7b7a647f667b30598529b1a6fe6e0a4f4b74696036e2a4bdef8e", 1828},
			{"eventloop/internal/tournament/component/timerrefalwayssubmit/core_test.go", "100644", "f6ab5d36cef05f238150f97f1f25eb9f000b4d2f", "09a540669d60c5e036d0142dd500cf28aa7f962feed06f5e10626311f7d003e9", 1519},
			{"eventloop/internal/tournament/component/timerrefalwayssubmit/layout64_test.go", "100644", "b14fe771a2f4c88e97ef3da02b8069b1aca68980", "41bb1e3fab572487ea3c9e0da3116874d81adda17ec9c7187f728e26310692b6", 316},
			{"eventloop/internal/tournament/component/timerrefownersubmit/core.go", "100644", "bad855f6c4281b1eecf993068dcf11301b89b984", "986a0aa238a5e562f1f0fbf107c64863ae47d5471e0cbc3b52b16c88a98c7c04", 2697},
			{"eventloop/internal/tournament/component/timerrefownersubmit/core_test.go", "100644", "554d3baca12952c9daf060786a24f38910c9936a", "8c8ca01722f2fb264702cbcff4972a577ec4c54e7334533f25d76db84f49036b", 2020},
			{"eventloop/internal/tournament/component/timerrefownersubmit/layout64_test.go", "100644", "b64595a00f06e2da3762252cf9fb78684e927065", "f6bf2a482c7313cf5811350a7b7649ea7daca5cc7fc23f81bfc2498f55e2667d", 315},
			{"eventloop/internal/tournament/component/timerrefrwmutex/core.go", "100644", "b035cf708c93e5440fbccbc30d4012528f249300", "194449ee0f317d0fbff6b97532b803cf7ce5a0daa520d171b62529358ccd62f6", 2162},
			{"eventloop/internal/tournament/component/timerrefrwmutex/core_test.go", "100644", "e63e3673ea5e16cbd755a0b8608f46252224a404", "41f7342d49aa9ccf2f7d0035ae9eed41b8205eaf3914c29f310424e1d5981747", 3156},
			{"eventloop/internal/tournament/component/timerrefrwmutex/layout64_test.go", "100644", "d21cfe804aa644b46cb29ddacca0ab610b919d53", "36d6186e27e7b6f50af40654e4849e5a6cb8cb559219facf1bb3483bec7d5c1e", 311},
			{"eventloop/internal/tournament/component/timerrefsyncmap/core.go", "100644", "55c6fe6cf871c458852afe0b715bf42ee3d80562", "352130445736d2356beadd3d044f72880b73333a0954ba64c75519d91ec6f874", 1491},
			{"eventloop/internal/tournament/component/timerrefsyncmap/core_test.go", "100644", "4bd5e97bc5f876be4ee2bba25c6d7abf04ec506d", "1d72d629daee9a403ef9a2626a3c80cf2ee4109bba2ee00ccbafe353a5b18e0f", 2817},
			{"eventloop/internal/tournament/component/timerrefsyncmap/layout64_test.go", "100644", "013f9aca05b2994b1f40929b7dd813c4137a74b9", "70176a3a129cfcde603cac4988799def3cc703195d900169eba375439e0b8304", 311},
		},
		roots: []string{
			"eventloop/internal/tournament/component/timerrefalwayssubmit",
			"eventloop/internal/tournament/component/timerrefownersubmit",
			"eventloop/internal/tournament/component/timerrefrwmutex",
			"eventloop/internal/tournament/component/timerrefsyncmap",
		},
		archive:      want,
		id:           "0005",
		objectFormat: "sha1",
		patchFormat:  materializationArchivePatchAbbrev7,
		pathCount:    12,
		payloadBytes: 18943,
	})
}
