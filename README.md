# GitHub Pull Request Files

This utility produces a list of files that have changed in a pull request. This is useful for continuous integration and other scripts that need to know which files have changed.

## Usage

- Support for one or more pull requests.
- Ensure 3000 API files limit is not exceeded; if so, the script will exit with an error.
- Parrallel processing of pull requests.
- Fetches file changes and deletions for specified pull requests from a GitHub repository.
- Saves results into separate text files: one for all files (including empty commits), one for changed files, and one for deleted files.
- Only generates files for changed and deleted files if there is content.

## Dependencies
- Go 1.18 or higher
- GitHub Personal Access Token with repository access

## Usage

```bash
Usage of .\github-pr-files:
  -output-dir string
        Directory to save output files (default is current directory) (default ".")
  -pulls string
        Comma-separated list of pull request numbers
  -repo string
        Full name of the repository in the format 'owner/name'
  -token string
        GitHub API token
```

## Examples

```bash
.\github-pr-files --token "${{ secrets.GH_PAT }}" --repo "torvalds/linux" --pulls "882,832,630" --output-dir dist
```