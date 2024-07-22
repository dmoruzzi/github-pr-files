package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	githubAPIURL     = "https://api.github.com"
	acceptHeader     = "application/vnd.github+json"
	userAgentHeader  = "dmoruzzi/github-pr-info@0.0.0"
	apiVersionHeader = "2022-11-28"
	maxChangedFiles  = 3000
	perPage          = 100
)

func githubHeaders(token string) map[string]string {
	return map[string]string{
		"Accept":               acceptHeader,
		"Authorization":        "Bearer " + token,
		"User-Agent":           userAgentHeader,
		"X-GitHub-Api-Version": apiVersionHeader,
	}
}

func doGitHubRequest(url string, token string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range githubHeaders(token) {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status: %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func filesInPR(repo string, pr int, token string) (map[string]string, error) {
	filesMap := make(map[string]string)
	page := 1

	for {
		url := fmt.Sprintf("%s/repos/%s/pulls/%d/files?page=%d&per_page=%d", githubAPIURL, repo, pr, page, perPage)
		bodyText, err := doGitHubRequest(url, token)
		if err != nil {
			return nil, err
		}

		var files []struct {
			Filename string `json:"filename"`
			Status   string `json:"status"`
		}
		if err := json.Unmarshal(bodyText, &files); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if len(files) == 0 {
			break
		}

		for _, file := range files {
			switch file.Status {
			case "modified", "added":
				filesMap[file.Filename] = "changed"
			case "deleted":
				filesMap[file.Filename] = "deleted"
			}
			log.Printf("[DEBUG] File in PR %d: %s (Status: %s)", pr, file.Filename, file.Status)
		}
		page++
	}

	return filesMap, nil
}

func writeFile(filePath string, filenames []string) error {
	data := strings.Join(filenames, "\n")
	return os.WriteFile(filePath, []byte(data), 0644)
}

func filesChangedCount(repo string, pr int, token string) (int, error) {
	url := fmt.Sprintf("%s/repos/%s/pulls/%d", githubAPIURL, repo, pr)
	bodyText, err := doGitHubRequest(url, token)
	if err != nil {
		return -1, err
	}

	var body map[string]interface{}
	if err := json.Unmarshal(bodyText, &body); err != nil {
		return -1, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if changedFilesFloat, ok := body["changed_files"].(float64); ok {
		return int(changedFilesFloat), nil
	}

	log.Printf("[WARN] No changed files in pull request %d", pr)
	return 0, nil
}

func processPR(repo string, pr int, token string, outputDir string, wg *sync.WaitGroup, results chan<- map[string][]string) {
	defer wg.Done()
	log.Printf("[INFO] Processing pull request %d", pr)

	count, err := filesChangedCount(repo, pr, token)
	if err != nil || count > maxChangedFiles {
		log.Printf("[ERROR] Failed to process PR %d: %v", pr, err)
		results <- nil
		return
	}

	filesMap, err := filesInPR(repo, pr, token)
	if err != nil {
		log.Printf("[ERROR] Failed to get files in PR %d: %v", pr, err)
		results <- nil
		return
	}

	var changedFiles, deletedFiles, allFiles []string
	for file, status := range filesMap {
		switch status {
		case "changed":
			changedFiles = append(changedFiles, file)
		case "deleted":
			deletedFiles = append(deletedFiles, file)
		}
		allFiles = append(allFiles, file)
	}

	files := map[string][]string{
		"all": allFiles,
	}
	if len(changedFiles) > 0 {
		files["chg"] = changedFiles
	}
	if len(deletedFiles) > 0 {
		files["del"] = deletedFiles
	}

	for name, content := range files {
		filePath := filepath.Join(outputDir, fmt.Sprintf("%d_%s.txt", pr, name))
		if err := writeFile(filePath, content); err != nil {
			log.Printf("[ERROR] Failed to write file %s: %v", filePath, err)
		}
	}

	log.Printf("[INFO] Files in pull request %d saved to %s", pr, outputDir)
	results <- files
}

func main() {
	repo := flag.String("repo", "", "Full name of the repository in the format 'owner/name'")
	pullRequests := flag.String("pulls", "", "Comma-separated list of pull request numbers")
	token := flag.String("token", "", "GitHub API token")
	outputDir := flag.String("output-dir", ".", "Directory to save output files (default is current directory)")
	flag.Parse()

	if *repo == "" || *pullRequests == "" || *token == "" {
		log.Println("[ERROR] Missing required flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("[ERROR] Failed to create output directory: %v", err)
	}

	var prs []int
	for _, p := range strings.Split(*pullRequests, ",") {
		pr, err := strconv.Atoi(p)
		if err != nil {
			log.Fatalf("[ERROR] Invalid pull request number: %s", p)
		}
		prs = append(prs, pr)
	}
	log.Printf("[DEBUG] Repository: %s, Pull Requests: %v", *repo, prs)

	var wg sync.WaitGroup
	results := make(chan map[string][]string, len(prs))

	for _, pr := range prs {
		wg.Add(1)
		go processPR(*repo, pr, *token, *outputDir, &wg, results)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allFiles, allChangedFiles, allDeletedFiles []string
	for filesMap := range results {
		if filesMap != nil {
			allFiles = append(allFiles, filesMap["all"]...)
			allChangedFiles = append(allChangedFiles, filesMap["chg"]...)
			allDeletedFiles = append(allDeletedFiles, filesMap["del"]...)
		}
	}

	for name, content := range map[string][]string{
		"all": allFiles,
		"chg": allChangedFiles,
		"del": allDeletedFiles,
	} {
		if err := writeFile(filepath.Join(*outputDir, fmt.Sprintf("all_%s.txt", name)), content); err != nil {
			log.Fatalf("[ERROR] Failed to create all_%s.txt: %v", name, err)
		}
	}

	log.Println("[INFO] All files saved to all.txt, all_chg.txt, and all_del.txt")
}
