package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: giti <repo-url> <source|clipboard> <replace-path>")
		fmt.Println("Example: giti github.com/user/repo clipboard ./README.md")
		fmt.Println("Example: giti github.com/user/repo ./local-file.go ./main.go")
		fmt.Println("Example: giti github.com/user/repo ./local-dir ./remote-dir")
		os.Exit(1)
	}

	repoURL := os.Args[1]
	source := os.Args[2]
	replacePath := os.Args[3]

	var isDir bool

	// Check if source is a directory (unless it's the clipboard)
	if source != "clipboard" {
		info, err := os.Stat(source)
		if err != nil {
			fmt.Printf("Error locating source: %v\n", err)
			os.Exit(1)
		}
		isDir = info.IsDir()
	}

	gitURL := normalizeRepoURL(repoURL)
	repoName := extractRepoName(repoURL)
	if repoName == "" {
		fmt.Println("Error: could not extract repo name from URL")
		os.Exit(1)
	}

	cloneDir := findUniqueDir(repoName)
	fmt.Printf("Cloning %s into %s...\n", gitURL, cloneDir)

	if err := runCommand("git", "clone", gitURL, cloneDir); err != nil {
		fmt.Printf("Error cloning repo: %v\n", err)
		os.Exit(1)
	}

	targetPath := filepath.Join(cloneDir, replacePath)

	if isDir {
		fmt.Printf("Overwriting directory %s with %s...\n", replacePath, source)
		// Remove the old directory in the repo
		if err := os.RemoveAll(targetPath); err != nil {
			cleanup(cloneDir)
			fmt.Printf("Error removing old directory %s: %v\n", targetPath, err)
			os.Exit(1)
		}
		// Copy new directory in
		if err := copyDir(source, targetPath); err != nil {
			cleanup(cloneDir)
			fmt.Printf("Error copying directory: %v\n", err)
			os.Exit(1)
		}
	} else {
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

		// Ensure the directory exists (in case replacePath is deep)
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
	}

	// Change to the repo directory to run git commands
	originalDir, _ := os.Getwd()
	if err := os.Chdir(cloneDir); err != nil {
		cleanup(cloneDir)
		fmt.Printf("Error changing to repo directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Running git add...")
	if err := runCommand("git", "add", "."); err != nil {
		os.Chdir(originalDir)
		cleanup(cloneDir)
		fmt.Printf("Error running git add: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Running git commit...")
	commitMsg := "giti: updated " + replacePath
	if isDir {
		commitMsg = "giti: overwrote directory " + replacePath
	}

	if err := runCommand("git", "commit", "-m", commitMsg); err != nil {
		os.Chdir(originalDir)
		cleanup(cloneDir)
		fmt.Printf("Error running git commit: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Running git push...")
	if err := runCommand("git", "push"); err != nil {
		os.Chdir(originalDir)
		cleanup(cloneDir)
		fmt.Printf("Error running git push: %v\n", err)
		os.Exit(1)
	}

	os.Chdir(originalDir)
	cleanup(cloneDir)
	fmt.Println("Done! Changes pushed and local clone removed.")
}

func copyDir(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	info, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return os.Chmod(dst, info.Mode())
}

func normalizeRepoURL(url string) string {
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "git@") {
		return url
	}
	if strings.HasPrefix(url, "github.com") {
		return "https://" + url
	}
	return "https://github.com/" + url
}

func extractRepoName(url string) string {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func findUniqueDir(baseName string) string {
	if _, err := os.Stat(baseName); os.IsNotExist(err) {
		return baseName
	}
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
	// clipboard.ReadAll() handles the platform-specific logic (Windows API, macOS pbpaste, Linux X11)
	text, err := clipboard.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read clipboard: %w", err)
	}
	return []byte(text), nil
}

func cleanup(dir string) {
	fmt.Printf("Cleaning up %s...\n", dir)
	os.RemoveAll(dir)
}