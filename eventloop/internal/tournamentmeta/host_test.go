package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestHostMetadataIsSingleLineAndNamespaced(t *testing.T) {
	var output bytes.Buffer
	if err := emitHostMetadata(&output, "executor"); err != nil {
		t.Fatalf("emitHostMetadata: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 7 {
		t.Fatalf("host metadata lines = %d, want 7:\n%s", len(lines), output.String())
	}
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		if !strings.HasPrefix(line, "tournament: meta=executor-") || strings.ContainsAny(line, "\r\n") {
			t.Errorf("invalid host metadata line %q", line)
		}
		if _, exists := seen[line]; exists {
			t.Errorf("duplicate host metadata line %q", line)
		}
		seen[line] = struct{}{}
	}
}

func TestMetadataPrefix(t *testing.T) {
	for _, value := range []string{"host", "executor-linux", "cpu2"} {
		if !metadataPrefixValid(value) {
			t.Errorf("metadataPrefixValid(%q) = false", value)
		}
	}
	for _, value := range []string{"", "Host", "2cpu", "host_name", "host/cpu"} {
		if metadataPrefixValid(value) {
			t.Errorf("metadataPrefixValid(%q) = true", value)
		}
	}
}

func TestParseLinuxCPU(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{
			name: "model name",
			data: "processor : 0\nmodel name : Example CPU\n",
			want: "Example CPU",
		},
		{
			name: "hardware",
			data: "processor : 0\nHardware : Example Board\n",
			want: "Example Board",
		},
		{
			name: "arm tuple",
			data: "processor : 0\nCPU implementer : 0x41\nCPU architecture : 8\nCPU variant : 0x0\nCPU part : 0xd49\nCPU revision : 0\n",
			want: "ARM implementer=0x41 architecture=8 variant=0x0 part=0xd49 revision=0",
		},
		{
			name: "ordinal is not identity",
			data: "processor : 0\nprocessor : 1\n",
			want: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLinuxCPU(strings.NewReader(test.data))
			if err != nil {
				t.Fatalf("parseLinuxCPU: %v", err)
			}
			if got != test.want {
				t.Fatalf("CPU = %q, want %q", got, test.want)
			}
		})
	}
}
