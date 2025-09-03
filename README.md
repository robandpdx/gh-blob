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
# Basic (defaults to 60m timeout)
gh blob upload --org <org> --archive-file-path <migration-archive>

# Custom timeout (duration format: 10m, 45m, 1h30m, etc.)
gh blob upload --org <org> --archive-file-path <migration-archive> --timeout 45m

# Short flag
gh blob upload -t 90m --org <org> --archive-file-path <migration-archive>
```

The timeout applies to the entire upload operation (including multi-part uploads) and defaults to 60 minutes if not specified.

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
