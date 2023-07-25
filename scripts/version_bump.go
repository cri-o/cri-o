package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"strconv"
)

const versionFile = "../internal/version/version.go"

func main() {
	var bumpType string
	if len(os.Args) > 1 {
		bumpType = os.Args[1]
	}

	// Read the current version from the version.go file
	currentVersion, err := getCurrentVersion()
	if err != nil {
		fmt.Printf("Error reading current version: %s\n", err)
		os.Exit(1)
	}

	// Bump the version based on the specified type (major, minor, or patch)
	newVersion := bumpVersion(currentVersion, bumpType)

	// Update the version in the version.go file
	if err := updateVersion(newVersion); err != nil {
		fmt.Printf("Error updating version: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Version bumped from %s to %s\n", currentVersion, newVersion)
}

func getCurrentVersion() (string, error) {
	versionPattern := `const\s+Version\s+=\s+"(.+)"`

	// Read the content of the version.go file
	content, err := os.ReadFile(versionFile)
	if err != nil {
		return "", err
	}

	// Find the version string using regex
	re := regexp.MustCompile(versionPattern)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return "", fmt.Errorf("unable to find version in %s", versionFile)
	}

	return matches[1], nil
}

func bumpVersion(version string, bumpType string) string {
	parts := strings.Split(version, ".")

	switch bumpType {
	case "major":
		parts[0] = incrementVersionPart(parts[0])
		parts[1] = "0"
		parts[2] = "0"
	case "minor":
		parts[1] = incrementVersionPart(parts[1])
		parts[2] = "0"
	case "patch":
		parts[2] = incrementVersionPart(parts[2])
	default:
		parts[2] = incrementVersionPart(parts[2])
	}

	return strings.Join(parts, ".")
}

func incrementVersionPart(part string) string {
	// Convert the part to an integer, increment it, and convert back to string
	num, err := strconv.Atoi(part)
	if err != nil {
		return "0"
	}
	num++
	return strconv.Itoa(num)
}

func updateVersion(newVersion string) error {
	versionPattern := `const\s+Version\s+=\s+".+"`

	// Read the content of the version.go file
	content, err := os.ReadFile(versionFile)
	if err != nil {
		return err
	}

	// Replace the version string with the new version using regex
	re := regexp.MustCompile(versionPattern)
	newContent := re.ReplaceAll(content, []byte(fmt.Sprintf(`const Version = "%s"`, newVersion)))

	// Write the updated content back to the version.go file
	if err := os.WriteFile(versionFile, newContent, 0644); err != nil {
		return err
	}

	return nil
}
