package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: giti <repo-url> <source|clipboard> <replace-path>")
		fmt.Println("Example: giti github.com/user/repo clipboard ./README.md")
		fmt.Println("Example: giti github.com/user/repo ./local-file.go ./main.go")
		os.Exit(1)
	}

	repoURL := os.Args[1]
	source := os.Args[2]
	replacePath := os.Args[3]

	// Normalize repo URL to full git URL
	gitURL := normalizeRepoURL(repoURL)

	// Extract repo name from URL
	repoName := extractRepoName(repoURL)
	if repoName == "" {
		fmt.Println("Error: could not extract repo name from URL")
		os.Exit(1)
	}

	// Find a unique directory name for cloning
	cloneDir := findUniqueDir(repoName)

	// Clone the repo
	fmt.Printf("Cloning %s into %s...\n", gitURL, cloneDir)
	if err := runCommand("git", "clone", gitURL, cloneDir); err != nil {
		fmt.Printf("Error cloning repo: %v\n", err)
		os.Exit(1)
	}

	// Get the content to copy
	var content []byte
	var err error

	if source == "clipboard" {
		content, err = getClipboardContent()
		if err != nil {
			cleanup(cloneDir)
			fmt.Printf("Error reading clipboard: %v\n", err)
			os.Exit(1)
		}
	} else {
		content, err = os.ReadFile(source)
		if err != nil {
			cleanup(cloneDir)
			fmt.Printf("Error reading source file %s: %v\n", source, err)
			os.Exit(1)
		}
	}

	// Write content to the target file in the cloned repo
	targetPath := filepath.Join(cloneDir, replacePath)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		cleanup(cloneDir)
		fmt.Printf("Error creating directories: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(targetPath, content, 0644); err != nil {
		cleanup(cloneDir)
		fmt.Printf("Error writing to %s: %v\n", targetPath, err)
		os.Exit(1)
	}

	fmt.Printf("Replaced %s with content from %s\n", replacePath, source)

	// Change to cloned repo and run git commands
	originalDir, _ := os.Getwd()

	if err := os.Chdir(cloneDir); err != nil {
		cleanup(cloneDir)
		fmt.Printf("Error changing to repo directory: %v\n", err)
		os.Exit(1)
	}

	// Git add
	fmt.Println("Running git add...")
	if err := runCommand("git", "add", "."); err != nil {
		os.Chdir(originalDir)
		cleanup(cloneDir)
		fmt.Printf("Error running git add: %v\n", err)
		os.Exit(1)
	}

	// Git commit
	fmt.Println("Running git commit...")
	if err := runCommand("git", "commit", "-m", "giti: updated "+replacePath); err != nil {
		os.Chdir(originalDir)
		cleanup(cloneDir)
		fmt.Printf("Error running git commit: %v\n", err)
		os.Exit(1)
	}

	// Git push
	fmt.Println("Running git push...")
	if err := runCommand("git", "push"); err != nil {
		os.Chdir(originalDir)
		cleanup(cloneDir)
		fmt.Printf("Error running git push: %v\n", err)
		os.Exit(1)
	}

	// Change back and cleanup
	os.Chdir(originalDir)
	cleanup(cloneDir)

	fmt.Println("Done! Changes pushed and local clone removed.")
}

func normalizeRepoURL(url string) string {
	// If it's already a full URL, return as-is
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "git@") {
		return url
	}

	// If it starts with github.com, add https://
	if strings.HasPrefix(url, "github.com") {
		return "https://" + url
	}

	// Assume it's a GitHub shorthand like "user/repo"
	return "https://github.com/" + url
}

func extractRepoName(url string) string {
	// Remove trailing .git if present
	url = strings.TrimSuffix(url, ".git")

	// Split by / and get the last part
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return ""
	}

	return parts[len(parts)-1]
}

func findUniqueDir(baseName string) string {
	// If the directory doesn't exist, use the base name
	if _, err := os.Stat(baseName); os.IsNotExist(err) {
		return baseName
	}

	// Otherwise, append a number until we find a unique name
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d", baseName, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getClipboardContent() ([]byte, error) {
	// Try xclip first (Linux)
	if content, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output(); err == nil {
		return content, nil
	}

	// Try xsel (Linux alternative)
	if content, err := exec.Command("xsel", "--clipboard", "--output").Output(); err == nil {
		return content, nil
	}

	// Try wl-paste (Wayland)
	if content, err := exec.Command("wl-paste").Output(); err == nil {
		return content, nil
	}

	// Try pbpaste (macOS)
	if content, err := exec.Command("pbpaste").Output(); err == nil {
		return content, nil
	}

	// Try PowerShell (Windows)
	if content, err := exec.Command("powershell", "-command", "Get-Clipboard").Output(); err == nil {
		return content, nil
	}

	return nil, fmt.Errorf("no clipboard tool available (tried xclip, xsel, wl-paste, pbpaste, powershell)")
}

func cleanup(dir string) {
	fmt.Printf("Cleaning up %s...\n", dir)
	os.RemoveAll(dir)
}
