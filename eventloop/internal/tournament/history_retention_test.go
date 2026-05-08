package tournament

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type retainedRootBenchmarkFile struct {
	sha256     string
	benchmarks int
}

func TestImmutableBaseRootBenchmarksRemainExact(t *testing.T) {
	want := map[string]retainedRootBenchmarkFile{
		"alive_ref_bench_test.go":         {sha256: "74a70bc06378514f6e009ba83ddf9b456a98610d4467c184112a88036a7f95b6", benchmarks: 35},
		"alive_ref_test.go":               {sha256: "89b9c041910c96eda4760f9b8a87abdd3d3a321579e00f4638b4e3497e458e74", benchmarks: 0},
		"benchmark_comprehensive_test.go": {sha256: "314a697868271a22beb64180f9452c91573735fb87c3fbd7bf3c0124157c4cdf", benchmarks: 32},
		"check_then_sleep_test.go":        {sha256: "4f80e8c84c0be856bc928040958d9d7d90f51d06f2f7713941ed3bfd9b6c0c32", benchmarks: 1},
		"drain_bench_test.go":             {sha256: "2c7ca1f8f730f47d08df8d2cdf8ff65c902a3698eadfb464a0418eeb2d94171c", benchmarks: 4},
		"ingress_bench_test.go":           {sha256: "744652f3f12afb5ee79f92f6f1b9bcf759571d3562dfc5dd461a842de28cfe6c", benchmarks: 4},
		"js_bench_test.go":                {sha256: "8fa389060de133194a622e3319ef44bbb0f7b3523cf1838894e86ec3aea36cf7", benchmarks: 6},
		"latency_analysis_test.go":        {sha256: "5c656dd454e136d36a8d75585746492f39fff560e81dacdfceca1aa60518c628", benchmarks: 3},
		"latency_profile_test.go":         {sha256: "4fcf5914aa84676c766ee6f7fb4670eee14faa0f8b70ad00d2328c0eb40d6620", benchmarks: 19},
		"metrics_psquare_bench_test.go":   {sha256: "c9de9bfc6013eda6a77ff6c44172d611389a5c10fa0c83dca257df29a16c22f6", benchmarks: 8},
		"micro_pingpong_test.go":          {sha256: "ac2d14a2dcfe20ad3ee038bcc93288d7037fcf9d186d19ef376d7613d150e9e8", benchmarks: 8},
		"promise_memory_bench_test.go":    {sha256: "33f79ed092b98fd6694cfdf5e322d31e25f6d1614466fbefb322356f91b17a1d", benchmarks: 12},
		"promise_reaction_bench_test.go":  {sha256: "d23c32222d497e969f25d303e375295fd2af85ef5ad94befd2d28091a7d8f473", benchmarks: 11},
		"timer_pool_test.go":              {sha256: "ecd252ce8163667f49a370b2d20edbf42babaf1c16f05ad3c002198b61d3fe1f", benchmarks: 4},
	}
	directory := filepath.Join("testdata", "history", "986e237", "root-benchmarks")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	seen := make(map[string]struct{}, len(want))
	total := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		expected, ok := want[entry.Name()]
		if !ok {
			t.Errorf("unexpected immutable-base Go source %q", entry.Name())
			continue
		}
		seen[entry.Name()] = struct{}{}
		path := filepath.Join(directory, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("ReadFile(%q): %v", path, err)
			continue
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != expected.sha256 {
			t.Errorf("%s SHA-256 = %s, want %s", entry.Name(), got, expected.sha256)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, data, 0)
		if err != nil {
			t.Errorf("ParseFile(%q): %v", path, err)
			continue
		}
		count := 0
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && strings.HasPrefix(function.Name.Name, "Benchmark") {
				count++
			}
		}
		if count != expected.benchmarks {
			t.Errorf("%s benchmark roots = %d, want %d", entry.Name(), count, expected.benchmarks)
		}
		total += count
	}
	if len(seen) != len(want) {
		t.Errorf("immutable-base files seen = %d, want %d", len(seen), len(want))
	}
	if total != 147 {
		t.Errorf("immutable-base benchmark roots = %d, want 147", total)
	}
}

func TestHistoricalTournamentSourcesRemainExact(t *testing.T) {
	want := map[string]string{
		"testdata/history/802436f7/correctness/ingress_torture_test.go":             "ff848563564010578a0770c83eee9c73ea60203203ebd7fae7c7e0d874c7eaee",
		"testdata/history/802436f7/correctness/microtask_test.go":                   "3de7bf870d446da070e26945636e4c2a15b7934f9ffc7c39ccda2f58990968f5",
		"testdata/history/802436f7/correctness/microtaskring_test.go":               "2d128508f99150b911a8592cf4aa1c6920cc1b42a126ee89dd5d7f7e035af27c",
		"testdata/history/802436f7/correctness/ring_seq_zero_test.go":               "89707789dc7d3d95b6c15822bb23da018fd514ee27026374e38d79a65f08784a",
		"testdata/history/802436f7/correctness/timerheap_edge_case_test.go":         "e341b6c9b24ecb3ef7b5d622b01e2c6b99c2b74e5748c626a61025fe37c404c7",
		"testdata/history/802436f7/root-benchmarks/alive_ref_bench_test.go":         "1d94806888c2c7293b6fffbc324b8257993109c5b28c5e4ae2d7cd6e81e6fd19",
		"testdata/history/802436f7/root-benchmarks/benchmark_comprehensive_test.go": "e6affaefbe9fe5f43af48a5db7e76bcff190b5a835fdad8f4eaf6a01b0e29770",
		"testdata/history/802436f7/root-benchmarks/latency_profile_test.go":         "8cf0adca52cf5f389733e0bd7e5dec3f08e2d3fae5e6e921bcf21d910a704eb2",
		"testdata/history/986e237/tournament-tests/concurrent_stop_test.go":         "56dba77a7de5f0517e0ef0ee0f2d0c6f7e70c96d0538a51dc88264421a08c40f",
		"testdata/history/986e237/tournament-tests/goja_immediate_burst_test.go":    "7dc0aca13abe40b0df9bac4a462bdd3bb5e2e2f45f02f0e8db94badf73f01049",
		"testdata/history/986e237/tournament-tests/goja_mixed_workload_test.go":     "410631d7b44a6db2d0978fd689352c575f0b211af7c24157f620a29fc0d83b69",
		"testdata/history/986e237/tournament-tests/goja_nested_timeouts_test.go":    "12987968159f02c93f5e441f5f277b9abf2bd66404a794ca3dfd4ff5e367128e",
		"testdata/history/986e237/tournament-tests/goja_promise_chain_test.go":      "4a357b42eabccd98aa310e1438e523f631916b9d7244e2bd8e430248cceb8c61",
		"testdata/history/986e237/tournament-tests/goja_timer_stress_test.go":       "76326f68aa14a2ca1687e7d5818aec357629e527319950a5ecbfbfc247c8f519",
		"testdata/history/986e237/tournament-tests/race_wakeup_test.go":             "043c4d0c8982335304e2df63d473608c32fe213222f752d2e71473c71aa92e2f",
		"testdata/history/cd6a5332/root-benchmarks/wakeup_dedup_test.go":            "1964a114c4bcc0c202bf88ba034dfb239369b6228e2a400dfaaaa9b3f03d0328",
	}
	for path, expected := range want {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Errorf("ReadFile(%q): %v", path, err)
			continue
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != expected {
			t.Errorf("%s SHA-256 = %s, want %s", path, got, expected)
		}
	}
}

func TestUnreachableCommitPayloadsRemainExact(t *testing.T) {
	want := map[string]struct {
		bytes  int
		sha256 string
	}{
		"3249ec949f50e3743b34cdaf831728ec86e8796b": {bytes: 283, sha256: "43f0ba8550c35e6b1a47200e66c3247622bf0d24a5f06067877362eddd826dc4"},
		"537692fbb199d9e7e13255260ae878faff94fc02": {bytes: 320, sha256: "1cd4d35604cae3f01df021aa7136d7b0a7a7080e4ca2e39045c55e658fd8cd70"},
		"5f691abe5d4557c13eb7c16d42f60f203df24336": {bytes: 261, sha256: "ab1c1b4beaf797cbcb78e134a30a408b48b3a903cce3528d8221b504830df9f7"},
		"60f89427c8b36ecb2b1c495309ac91187c26fd06": {bytes: 261, sha256: "597ff77baa81e2109bf1475b06b4f75a4950551fc322438400462b038eedbfcd"},
		"786998e8eb654d670dcf2174c1e362c784a08d3e": {bytes: 403, sha256: "2b485f621d7ab4392b7edf904184e1f0a98d6e1e73d5469a6206bd5e70e9bcfb"},
		"819ee003aa8ff73d51e5a43dd35169db784f3111": {bytes: 261, sha256: "e4bb6d29fac32e6f5c573b928b37c4bf909a61f01a307eb16ed92b96613eb25f"},
		"81caf61f96167e7b7f5ecf497af9110890ff6e03": {bytes: 261, sha256: "b1c9083de73e77ff2f0c2825ec7753a77fa856dcc417fd5a06a82097ac91f6a9"},
		"b3c342c981169d1b8b348c81a8e850a8a87911ee": {bytes: 342, sha256: "48e18fca44ecc884e675d642cbd7800e105d964c19888cd30d4c44d55b328a5b"},
		"bcadd4aaa61c7a9c1068a493c948bb88bf5fa038": {bytes: 297, sha256: "5def94f6c0874461c531a6e2e71d490d9a33e57579795676a02b44b697951206"},
		"f7ef4c86843e1790bf0528975ab2c92ba3351702": {bytes: 403, sha256: "7ec816b4bb6c607ac2dc2105e65ff05800d3a3f593746d2ba657b2a47b8f4c83"},
		"f97fc084bb6a796dac41461a01f059a5845fbe47": {bytes: 226, sha256: "5f54b38bfc32fd492f8231be1d625f7648c2954b1906a1e04ef26229b97e194c"},
		"53e2f662adc245c9b63e06bb64977b0751dcff82": {bytes: 305, sha256: "5757cc94cc79fcc877e8ff8d0b8626f6835fb5efa29cbc6ca0cf188420424c65"},
		"1396868d29689c659ff7782760e89423aa478cf4": {bytes: 350, sha256: "f85df86a07aa8d4effc78cc3126ec493c2d5d9db5dc0da34fcd73628e4d04821"},
	}
	directory := filepath.Join("revisions", "commits")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	seen := make(map[string]struct{}, len(want))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".commit.b64") {
			t.Errorf("unexpected commit archive %q", entry.Name())
			continue
		}
		oid := strings.TrimSuffix(entry.Name(), ".commit.b64")
		expected, ok := want[oid]
		if !ok {
			t.Errorf("unexpected commit payload %q", entry.Name())
			continue
		}
		seen[oid] = struct{}{}
		encoded, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			t.Errorf("ReadFile(%q): %v", entry.Name(), err)
			continue
		}
		text := string(encoded)
		if !strings.HasSuffix(text, "\n") || strings.Count(text, "\n") != 1 {
			t.Errorf("%s is not one base64 line", entry.Name())
			continue
		}
		payload, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSuffix(text, "\n"))
		if err != nil {
			t.Errorf("decode %s: %v", entry.Name(), err)
			continue
		}
		if len(payload) != expected.bytes {
			t.Errorf("%s decoded bytes = %d, want %d", entry.Name(), len(payload), expected.bytes)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(payload)); got != expected.sha256 {
			t.Errorf("%s payload SHA-256 = %s, want %s", entry.Name(), got, expected.sha256)
		}
		hash := sha1.New()
		fmt.Fprintf(hash, "commit %d%c", len(payload), 0)
		hash.Write(payload)
		if got := fmt.Sprintf("%x", hash.Sum(nil)); got != oid {
			t.Errorf("%s commit object = %s, want %s", entry.Name(), got, oid)
		}
	}
	if len(seen) != len(want) {
		t.Errorf("commit payloads seen = %d, want %d", len(seen), len(want))
	}
}

func TestUnreachablePatchesReconstructExactTrees(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("repository path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repository, ".git")); os.IsNotExist(err) {
		t.Skip("exact reconstruction requires the monorepo Git object store")
	} else if err != nil {
		t.Fatalf("inspect repository: %v", err)
	}
	for _, revision := range []struct {
		name          string
		parent        string
		wantTree      string
		wantEventloop string
		wantGoja      string
		patch         string
		wantSHA256    string
	}{
		{
			name:          "wake 43be6122",
			parent:        "cd6a53322588420c9e2b5e19e5791b7b0696117f",
			wantTree:      "7d99eec56bf840cb4b832c29d1072b355a707910",
			wantEventloop: "57b88dedc97ec680e4915cbf7b0181f3def008b7",
			wantGoja:      "69d8cf81666942396704d3d4bdb75208a0e523c6",
			patch:         "0001-Temporary-WAKE-H2-candidate-43be6122.patch",
			wantSHA256:    "5a5d995a3988240248f8b6f2b60ad623b23a4ba66c209ed87d0f28be264bf7dd",
		},
		{
			name:          "wake 9e721d42",
			parent:        "cd6a53322588420c9e2b5e19e5791b7b0696117f",
			wantTree:      "93f2b311557b83249ca59b5f9646f9a2677a3944",
			wantEventloop: "35d7e1cfa4dc7d7485a13b39d7c55ad7fcffae83",
			wantGoja:      "69d8cf81666942396704d3d4bdb75208a0e523c6",
			patch:         "0001-Temporary-WAKE-H2-candidate-9e721d42.patch",
			wantSHA256:    "434858ef0d7c4940859f0b618c1ba62d0d85e5f56af1d0dcdec4f9bfc43cd0ee",
		},
		{
			name:          "wake f13eb33f",
			parent:        "cd6a53322588420c9e2b5e19e5791b7b0696117f",
			wantTree:      "7d387838addad98726792c47e30d4e3b7f824e88",
			wantEventloop: "115d23747d2fe938c88b440e822c9bb40e2f61fe",
			wantGoja:      "69d8cf81666942396704d3d4bdb75208a0e523c6",
			patch:         "0001-Temporary-WAKE-H2-candidate-f13eb33f.patch",
			wantSHA256:    "d8839deb295d0c82ab7a0b8f9f87eb71308efcd6006b5637fe2f1e1164f008f9",
		},
		{
			name:          "promise candidate",
			parent:        "9b77ad1d20f093759da7e0ff4a85fa50b5cf6f15",
			wantTree:      "7328ba8bdf18a3262a61761a2c13265e791659f4",
			wantEventloop: "3d57c2de57c10cac02ea7f05f9a83c10ca9846f1",
			wantGoja:      "69d8cf81666942396704d3d4bdb75208a0e523c6",
			patch:         "0001-index-on-no-branch-9b77ad1-Refresh-timer-poll-deadli.patch",
			wantSHA256:    "34f0f29d9ac9f581b5e076bb45fb43bb8b197d8a6f3e1f29f440915e6580e865",
		},
		{
			name:          "E001 candidate",
			parent:        "c8e744e4867c351d5b83e438fd2cb438c9b04898",
			wantTree:      "c28e4b2a8cc0acf2f7795f3c82666273ec5dd6ec",
			wantEventloop: "6eec43e57bfc858888a804e206b0d97335ba6d89",
			wantGoja:      "69d8cf81666942396704d3d4bdb75208a0e523c6",
			patch:         "0001-Test-E001-candidate-across-platforms.patch",
			wantSHA256:    "1ae62529924d910ba7f038a8afdd7d5bbf54c2876100b033ccc3e152ab80f48a",
		},
	} {
		t.Run(revision.name, func(t *testing.T) {
			patchPath, err := filepath.Abs(filepath.Join("revisions", revision.patch))
			if err != nil {
				t.Fatalf("patch path: %v", err)
			}
			patchData, err := os.ReadFile(patchPath)
			if err != nil {
				t.Fatalf("read patch: %v", err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(patchData)); got != revision.wantSHA256 {
				t.Fatalf("patch SHA-256 = %s, want %s", got, revision.wantSHA256)
			}
			reconstruction := t.TempDir()
			objectDirectory := filepath.Join(reconstruction, "objects")
			if err := os.Mkdir(objectDirectory, 0o700); err != nil {
				t.Fatalf("create object directory: %v", err)
			}
			configPath := filepath.Join(reconstruction, "global.gitconfig")
			if err := os.WriteFile(configPath, nil, 0o600); err != nil {
				t.Fatalf("create empty Git config: %v", err)
			}
			baseEnvironment := []string{
				"GIT_CONFIG_GLOBAL=" + configPath,
				"GIT_CONFIG_NOSYSTEM=1",
				"GIT_NO_REPLACE_OBJECTS=1",
				"HOME=" + reconstruction,
				"XDG_CONFIG_HOME=" + reconstruction,
			}
			commonDirectory := revisionGitOutput(t, repository, baseEnvironment, "rev-parse", "--path-format=absolute", "--git-common-dir")
			environment := append(baseEnvironment,
				"GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(commonDirectory, "objects"),
				"GIT_INDEX_FILE="+filepath.Join(reconstruction, "index"),
				"GIT_OBJECT_DIRECTORY="+objectDirectory,
			)
			runRevisionGit(t, repository, environment, "read-tree", revision.parent+"^{tree}")
			runRevisionGit(t, repository, environment, "apply", "--cached", "--binary", "--whitespace=error-all", patchPath)
			command := exec.Command("git", "-C", repository, "write-tree")
			command.Env = revisionGitEnvironment(environment)
			output, err := command.Output()
			if err != nil {
				t.Fatalf("write reconstructed tree: %v", err)
			}
			if got := strings.TrimSpace(string(output)); got != revision.wantTree {
				t.Fatalf("reconstructed tree = %s, want %s", got, revision.wantTree)
			}
			if got := revisionGitOutput(t, repository, environment, "rev-parse", revision.wantTree+":eventloop"); got != revision.wantEventloop {
				t.Fatalf("reconstructed eventloop tree = %s, want %s", got, revision.wantEventloop)
			}
			if got := revisionGitOutput(t, repository, environment, "rev-parse", revision.wantTree+":goja-eventloop"); got != revision.wantGoja {
				t.Fatalf("reconstructed goja-eventloop tree = %s, want %s", got, revision.wantGoja)
			}
		})
	}
}

func runRevisionGit(t *testing.T, directory string, environment []string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	command.Env = revisionGitEnvironment(environment)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func revisionGitOutput(t *testing.T, directory string, environment []string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	command.Env = revisionGitEnvironment(environment)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func revisionGitEnvironment(overrides []string) []string {
	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if strings.HasPrefix(key, "GIT_") || key == "HOME" || key == "XDG_CONFIG_HOME" {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, overrides...)
}
