package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

func hostCommand(arguments []string) int {
	flags := flag.NewFlagSet("host", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	prefix := flags.String("prefix", "host", "metadata key prefix")
	if err := flags.Parse(arguments); err != nil {
		return commandError(err)
	}
	if flags.NArg() != 0 || !metadataPrefixValid(*prefix) {
		return commandError(errors.New("host accepts only a lowercase metadata -prefix"))
	}
	return commandError(emitHostMetadata(os.Stdout, *prefix))
}

func emitHostMetadata(writer io.Writer, prefix string) error {
	values := [][2]string{
		{"goos", runtime.GOOS},
		{"goarch", runtime.GOARCH},
		{"os-version", hostOSVersion()},
		{"kernel", commandValue("uname", "-srvm")},
		{"cpu", hostCPU()},
		{"logical-cpus", strconv.Itoa(runtime.NumCPU())},
		{"cpu-quota", hostCPUQuota()},
	}
	for _, value := range values {
		if _, err := fmt.Fprintf(
			writer,
			"tournament: meta=%s-%s=%s\n",
			prefix,
			value[0],
			metadataValue(value[1]),
		); err != nil {
			return fmt.Errorf("write host metadata: %w", err)
		}
	}
	return nil
}

func metadataPrefixValid(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func metadataValue(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			return ' '
		}
		return character
	}, strings.TrimSpace(value))
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return "<unknown>"
	}
	return value
}

func commandValue(name string, arguments ...string) string {
	output, err := exec.Command(name, arguments...).Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func hostOSVersion() string {
	switch runtime.GOOS {
	case "darwin":
		return commandValue("sw_vers", "-productVersion")
	case "linux":
		file, err := os.Open("/etc/os-release")
		if err != nil {
			return ""
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			key, value, found := strings.Cut(scanner.Text(), "=")
			if found && key == "PRETTY_NAME" {
				return strings.Trim(value, `"`)
			}
		}
	case "windows":
		return commandValue("cmd", "/c", "ver")
	}
	return ""
}

func hostCPU() string {
	switch runtime.GOOS {
	case "darwin":
		return commandValue("sysctl", "-n", "machdep.cpu.brand_string")
	case "linux":
		return linuxCPU()
	case "windows":
		return os.Getenv("PROCESSOR_IDENTIFIER")
	}
	return ""
}

func linuxCPU() string {
	if file, err := os.Open("/proc/cpuinfo"); err == nil {
		value, parseErr := parseLinuxCPU(file)
		closeErr := file.Close()
		if parseErr == nil && closeErr == nil && value != "" {
			return value
		}
	}
	for _, path := range []string{
		"/sys/firmware/devicetree/base/model",
		"/proc/device-tree/model",
	} {
		if value, err := os.ReadFile(path); err == nil {
			value = bytesTrimNUL(value)
			if len(value) != 0 {
				return string(value)
			}
		}
	}
	output := commandValueEnvironment(
		[]string{"LC_ALL=C", "LANG=C"},
		"lscpu",
	)
	for line := range strings.SplitSeq(output, "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && strings.EqualFold(strings.TrimSpace(key), "Model name") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseLinuxCPU(reader io.Reader) (string, error) {
	keys := []string{
		"model name",
		"hardware",
		"cpu implementer",
		"cpu architecture",
		"cpu variant",
		"cpu part",
		"cpu revision",
	}
	values := make(map[string]string, len(keys))
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), ":")
		if !found {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, exists := values[key]; !exists {
			values[key] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan Linux CPU identity: %w", err)
	}
	for _, key := range keys[:2] {
		if values[key] != "" {
			return values[key], nil
		}
	}
	parts := make([]string, 0, len(keys)-2)
	for _, key := range keys[2:] {
		if values[key] != "" {
			parts = append(parts, strings.TrimPrefix(key, "cpu ")+"="+values[key])
		}
	}
	if len(parts) != 0 {
		return "ARM " + strings.Join(parts, " "), nil
	}
	return "", nil
}

func commandValueEnvironment(environment []string, name string, arguments ...string) string {
	command := exec.Command(name, arguments...)
	command.Env = append(os.Environ(), environment...)
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return string(output)
}

func bytesTrimNUL(value []byte) []byte {
	for len(value) != 0 && (value[len(value)-1] == 0 || unicode.IsSpace(rune(value[len(value)-1]))) {
		value = value[:len(value)-1]
	}
	return value
}

func hostCPUQuota() string {
	if runtime.GOOS != "linux" {
		return "not-applicable"
	}
	if value, err := os.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		return "cgroup-v2:" + string(value)
	}
	quota, quotaErr := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	period, periodErr := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if quotaErr == nil && periodErr == nil {
		return "cgroup-v1:" + strings.TrimSpace(string(quota)) + "/" + strings.TrimSpace(string(period))
	}
	return "unavailable"
}
