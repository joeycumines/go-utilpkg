//go:build !unix && !windows

package main

func testRunEnvironmentPlatform() []string {
	return nil
}

func sortRunTestEnvironment(environment []string) []string {
	return environment
}
