# gh-blob

This [GitHub CLI](https://cli.github.com/) extension makes it easy to do [GitHub owned blob storage](https://github.com/orgs/community/discussions/144948) operations.

## Installation
```bash
gh extension install robandpdx/gh-blob
```

Set an environment variable with your PAT:
```bash
export GITHUB_TOKEN="<token>"
```

## Usage
### Upload
```bash
gh blob upload --org <org> --archive-file-path <migration-archive>
```

### Delete
```bash
gh blob delete --id <id>
```

### Query all blobs
```bash
gh blob query-all --org <org>
```

### Query blob 
```bash
gh blob query --id <blob-id>
```
