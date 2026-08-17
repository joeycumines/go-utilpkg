package tournament

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var materializationArchiveWindowsVolumePattern = regexp.MustCompile(`^[A-Za-z]:`)

type materializationArchivePatchSnapshot struct {
	payload []byte
	info    os.FileInfo
}

func readMaterializationArchivePatch(filename string) (materializationArchivePatchSnapshot, error) {
	before, err := os.Lstat(filename)
	if err != nil {
		return materializationArchivePatchSnapshot{}, err
	}
	if !before.Mode().IsRegular() {
		return materializationArchivePatchSnapshot{}, fmt.Errorf("patch %q is not a physical regular file", filename)
	}
	file, err := os.Open(filename)
	if err != nil {
		return materializationArchivePatchSnapshot{}, err
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return materializationArchivePatchSnapshot{}, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		_ = file.Close()
		return materializationArchivePatchSnapshot{}, fmt.Errorf("patch %q changed before open", filename)
	}
	payload, readErr := io.ReadAll(file)
	afterOpen, statErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil {
		return materializationArchivePatchSnapshot{}, readErr
	}
	if statErr != nil {
		return materializationArchivePatchSnapshot{}, statErr
	}
	if closeErr != nil {
		return materializationArchivePatchSnapshot{}, closeErr
	}
	afterPath, err := os.Lstat(filename)
	if err != nil {
		return materializationArchivePatchSnapshot{}, err
	}
	if !afterOpen.Mode().IsRegular() || !afterPath.Mode().IsRegular() ||
		!os.SameFile(opened, afterOpen) || !os.SameFile(afterOpen, afterPath) ||
		before.Mode() != afterPath.Mode() || before.Size() != afterPath.Size() ||
		!before.ModTime().Equal(afterPath.ModTime()) || int64(len(payload)) != afterPath.Size() {
		return materializationArchivePatchSnapshot{}, fmt.Errorf("patch %q changed while reading", filename)
	}
	return materializationArchivePatchSnapshot{payload: payload, info: afterPath}, nil
}

func validateMaterializationArchivePatchStable(
	before,
	after materializationArchivePatchSnapshot,
) error {
	if before.info == nil || after.info == nil || !os.SameFile(before.info, after.info) ||
		before.info.Mode() != after.info.Mode() || !bytes.Equal(before.payload, after.payload) {
		return fmt.Errorf("patch artifact changed during verification")
	}
	return nil
}

func materializationArchiveGit(repository string, environment []string, input []byte, arguments ...string) materializationArchiveGitOutput {
	command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
	command.Env = environment
	command.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return materializationArchiveGitOutput{stdout: stdout.Bytes(), stderr: stderr.Bytes(), err: err}
}

func materializationArchiveGitEnvironment(temporary string) ([]string, error) {
	blank := filepath.Join(temporary, "blank")
	if err := os.WriteFile(blank, nil, 0o600); err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(os.Environ())+12)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "GIT_") || upper == "HOME" || upper == "USERPROFILE" ||
			upper == "XDG_CONFIG_HOME" || upper == "LANG" || upper == "LANGUAGE" || upper == "LC_ALL" {
			continue
		}
		filtered = append(filtered, value)
	}
	return append(filtered,
		"GIT_CONFIG_GLOBAL="+blank,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_INDEX_FILE="+filepath.Join(temporary, "index"),
		"HOME="+temporary,
		"USERPROFILE="+temporary,
		"XDG_CONFIG_HOME="+temporary,
		"LC_ALL=C",
		"LANG=C",
	), nil
}

func requireMaterializationArchiveGit(
	t *testing.T,
	id string,
	repository string,
	environment []string,
	input []byte,
	arguments ...string,
) materializationArchiveGitOutput {
	t.Helper()
	output := materializationArchiveGit(repository, environment, input, arguments...)
	if output.err != nil {
		t.Fatalf("materialization archive %q git %v: %v\nstdout:\n%s\nstderr:\n%s", id, arguments, output.err, output.stdout, output.stderr)
	}
	if len(output.stderr) != 0 {
		t.Fatalf("materialization archive %q git %v wrote stderr:\n%s", id, arguments, output.stderr)
	}
	return output
}

func materializationArchiveScalar(
	t *testing.T,
	id string,
	repository string,
	environment []string,
	arguments ...string,
) string {
	t.Helper()
	output := requireMaterializationArchiveGit(t, id, repository, environment, nil, arguments...)
	return materializationArchiveOutputScalar(t, id, arguments, output.stdout)
}

func materializationArchiveOutputScalar(t *testing.T, id string, arguments []string, output []byte) string {
	t.Helper()
	if len(output) < 2 || output[len(output)-1] != '\n' || bytes.Contains(output[:len(output)-1], []byte{'\n'}) {
		t.Fatalf("materialization archive %q git %v scalar output = %q", id, arguments, output)
	}
	return string(output[:len(output)-1])
}

func materializationArchivePatchArguments(spec materializationArchiveSpec) []string {
	arguments := []string{"diff", "--binary"}
	if spec.patchFormat == materializationArchivePatchFullIndex {
		arguments = append(arguments, "--full-index")
	} else {
		arguments = append(arguments, "--abbrev=7")
	}
	return append(arguments,
		"--no-renames",
		"--no-ext-diff",
		"--no-textconv",
		"--no-color",
		"--src-prefix=a/",
		"--dst-prefix=b/",
		spec.archive.EmptyTree,
		spec.archive.ReconstructedTree,
		"--",
	)
}

func materializationArchiveBlobSHA1(payload []byte) string {
	hash := sha1.New()
	_, _ = fmt.Fprintf(hash, "blob %d%c", len(payload), byte(0))
	_, _ = hash.Write(payload)
	return hex.EncodeToString(hash.Sum(nil))
}

func validateMaterializationArchiveFSCK(stdout, stderr []byte) error {
	if len(stdout) != 0 {
		return fmt.Errorf("unexpected fsck stdout %q", stdout)
	}
	var unexpected []byte
	for line := range bytes.SplitSeq(stderr, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte("notice: HEAD points to an unborn branch ")) {
			continue
		}
		unexpected = append(unexpected, line...)
		unexpected = append(unexpected, '\n')
	}
	if len(unexpected) != 0 {
		return fmt.Errorf("unexpected fsck stderr %q", unexpected)
	}
	return nil
}

func validateMaterializationArchivePath(value string) error {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if value == "" || value == "." || clean != value || filepath.IsAbs(value) || filepath.VolumeName(value) != "" ||
		materializationArchiveWindowsVolumePattern.MatchString(value) || strings.HasPrefix(value, "//") ||
		!utf8.ValidString(value) || strings.ContainsAny(value, "\\:<>\"|?*") ||
		strings.IndexFunc(value, unicode.IsControl) >= 0 || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("repository path %q is invalid", value)
	}
	for component := range strings.SplitSeq(value, "/") {
		if component == "" || strings.TrimRight(component, ". ") != component {
			return fmt.Errorf("repository path %q has a nonportable component", value)
		}
		stem, _, _ := strings.Cut(component, ".")
		upper := strings.ToUpper(stem)
		if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" ||
			(len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) &&
				upper[3] >= '1' && upper[3] <= '9') {
			return fmt.Errorf("repository path %q uses a reserved Windows component", value)
		}
	}
	return nil
}

func validMaterializationArchiveHex(value string, size int) bool {
	if len(value) != size || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func assertMaterializationArchiveNoAlternates(t *testing.T, id, repository string) {
	t.Helper()
	path := filepath.Join(repository, "objects", "info", "alternates")
	if _, err := os.Lstat(path); err == nil {
		t.Fatalf("materialization archive %q isolated repository has alternates", id)
	} else if !os.IsNotExist(err) {
		t.Fatalf("materialization archive %q alternates: %v", id, err)
	}
}
