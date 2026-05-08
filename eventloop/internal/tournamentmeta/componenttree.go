package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"path"
	"slices"
	"strconv"
	"strings"
)

const (
	componentTreePolicy       = "os-root-complete-v1"
	componentTreeDigestDomain = "go-utilpkg-eventloop-tournament-component-tree-v1"
)

type componentTree struct { // betteralign:ignore canonical JSON field order
	Policy       string                `json:"policy"`
	RecordCount  int                   `json:"record_count"`
	PayloadBytes uint64                `json:"payload_bytes"`
	SHA256       string                `json:"sha256"`
	Records      []componentTreeRecord `json:"records"`
}

type componentTreeRecord struct { // betteralign:ignore canonical JSON field order
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	Size          uint64 `json:"size"`
	SHA256        string `json:"sha256"`
	SymlinkTarget string `json:"symlink_target"`
}

func newComponentTree(records []componentTreeRecord) (componentTree, error) {
	payloadBytes, err := validateComponentTreeRecords(records)
	if err != nil {
		return componentTree{}, err
	}
	digest, err := fingerprintComponentTree(records)
	if err != nil {
		return componentTree{}, err
	}
	return componentTree{
		Policy:       componentTreePolicy,
		RecordCount:  len(records),
		PayloadBytes: payloadBytes,
		SHA256:       digest,
		Records:      slices.Clone(records),
	}, nil
}

func validateComponentTree(tree componentTree) error {
	if tree.Policy != componentTreePolicy || tree.RecordCount != len(tree.Records) || tree.RecordCount == 0 {
		return errors.New("component tree has inconsistent policy or record count")
	}
	payloadBytes, err := validateComponentTreeRecords(tree.Records)
	if err != nil {
		return err
	}
	if tree.PayloadBytes != payloadBytes {
		return fmt.Errorf("component tree payload bytes = %d, want %d", tree.PayloadBytes, payloadBytes)
	}
	digest, err := fingerprintComponentTree(tree.Records)
	if err != nil {
		return err
	}
	if tree.SHA256 != digest {
		return fmt.Errorf("component tree SHA-256 = %q, want %q", tree.SHA256, digest)
	}
	return nil
}

func fingerprintComponentTree(records []componentTreeRecord) (string, error) {
	payloadBytes, err := validateComponentTreeRecords(records)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	writeFingerprintFrame(digest, []byte(componentTreeDigestDomain))
	writeFingerprintFrame(digest, []byte(componentTreePolicy))
	writeFingerprintFrame(digest, []byte(strconv.Itoa(len(records))))
	writeFingerprintFrame(digest, []byte(strconv.FormatUint(payloadBytes, 10)))
	for _, record := range records {
		writeFingerprintFrame(digest, []byte(record.Path))
		writeFingerprintFrame(digest, []byte(record.Mode))
		writeFingerprintFrame(digest, []byte(strconv.FormatUint(record.Size, 10)))
		writeFingerprintFrame(digest, []byte(record.SHA256))
		writeFingerprintFrame(digest, []byte(record.SymlinkTarget))
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func validateComponentTreeRecords(records []componentTreeRecord) (uint64, error) {
	if len(records) == 0 || records[0].Path != "." || records[0].Mode != "040000" {
		return 0, errors.New("component tree must begin with its root directory")
	}
	directories := make(map[string]struct{}, len(records))
	portable := make(map[string]string, len(records))
	previous := ""
	var payloadBytes uint64
	for index, record := range records {
		if index != 0 && record.Path <= previous {
			return 0, fmt.Errorf("component tree records are not strictly sorted at %q", record.Path)
		}
		previous = record.Path
		if record.Path != "." {
			if err := validateRelativePath(record.Path); err != nil {
				return 0, fmt.Errorf("component tree path: %w", err)
			}
			parent := path.Dir(record.Path)
			if _, exists := directories[parent]; !exists {
				return 0, fmt.Errorf("component tree path %q has no recorded parent directory %q", record.Path, parent)
			}
		}
		folded := strings.ToLower(record.Path)
		if prior, exists := portable[folded]; exists {
			return 0, fmt.Errorf("component tree paths %q and %q collide case-insensitively", prior, record.Path)
		}
		portable[folded] = record.Path
		switch record.Mode {
		case "040000":
			if record.Size != 0 || record.SHA256 != "" || record.SymlinkTarget != "" {
				return 0, fmt.Errorf("component directory %q has payload fields", record.Path)
			}
			directories[record.Path] = struct{}{}
		case "100644", "100755":
			if record.SymlinkTarget != "" {
				return 0, fmt.Errorf("component file %q has a symlink target", record.Path)
			}
			if err := validateCanonicalSHA256(record.SHA256, "component file"); err != nil {
				return 0, fmt.Errorf("component file %q: %w", record.Path, err)
			}
			if math.MaxUint64-payloadBytes < record.Size {
				return 0, errors.New("component tree payload byte count overflows uint64")
			}
			payloadBytes += record.Size
		case "120000":
			if record.SymlinkTarget == "" || record.Size != uint64(len(record.SymlinkTarget)) {
				return 0, fmt.Errorf("component symlink %q has inconsistent target size", record.Path)
			}
			if err := validateComponentSymlink(record.Path, record.SymlinkTarget); err != nil {
				return 0, err
			}
			digest := sha256.Sum256([]byte(record.SymlinkTarget))
			if record.SHA256 != hex.EncodeToString(digest[:]) {
				return 0, fmt.Errorf("component symlink %q has inconsistent target digest", record.Path)
			}
			if math.MaxUint64-payloadBytes < record.Size {
				return 0, errors.New("component tree payload byte count overflows uint64")
			}
			payloadBytes += record.Size
		default:
			return 0, fmt.Errorf("component tree path %q has unsupported mode %q", record.Path, record.Mode)
		}
	}
	return payloadBytes, nil
}

func validateComponentSymlink(relative, target string) error {
	if err := validateSymlinkTarget(target); err != nil {
		return fmt.Errorf("component symlink %q: %w", relative, err)
	}
	resolved := path.Clean(path.Join(path.Dir(relative), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("component symlink %q target %q escapes component root", relative, target)
	}
	if resolved != "." {
		if err := validateRelativePath(resolved); err != nil {
			return fmt.Errorf("component symlink %q target %q resolves nonportably: %w", relative, target, err)
		}
	}
	return nil
}
