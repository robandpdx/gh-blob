# gh-blob

This [GitHub CLI](https://cli.github.com/) extension makes it easy to do [GitHub owned blob storage](https://github.com/orgs/community/discussions/144948) operations.

## Installation

```bash
gh extension install robandpdx/gh-blob
```

## Authentication

You can authenticate in two ways:

### Option 1: Environment Variable (Traditional)

Set an environment variable with your PAT:

```bash
export GITHUB_TOKEN="<token>"
```

### Option 2: Command Line Flag (New)

Use the `--token` flag with any command:

```bash
gh blob <command> --token "<token>" [other-flags]
```

**Note**: The `--token` flag takes precedence over the `GITHUB_TOKEN` environment variable if both are provided.

## Configuration

### GitHub Enterprise Cloud with Data Residency

For GitHub Enterprise Cloud with Data Residency, use the `--hostname` flag:

```bash
gh blob <command> --hostname "enterprise.ghe.com" [other-flags]
```

**Default**: `github.com` (if not specified)

## Usage

### Upload

```bash
# Basic (defaults to 60m timeout)
gh blob upload --org <org> --archive-file-path <migration-archive>

# Using short flags
gh blob upload -o <org> -a <migration-archive>

# With token flag
gh blob upload --token "<token>" --org <org> --archive-file-path <migration-archive>

# With hostname for GitHub Enterprise Cloud with Data Residency
gh blob upload --hostname "enterprise.ghe.com" --org <org> --archive-file-path <migration-archive>

# Custom timeout (duration format: 10m, 45m, 1h30m, etc.)
gh blob upload --org <org> --archive-file-path <migration-archive> --timeout 45m
gh blob upload -o <org> -a <migration-archive> -t 45m

# Combined flags (token, hostname, timeout)
gh blob upload --token "<token>" --hostname "enterprise.ghe.com" -t 90m -o <org> -a <migration-archive>
```

The timeout applies to the entire upload operation (including multi-part uploads) and defaults to 60 minutes if not specified.

### Delete

```bash
# Long flag
gh blob delete --id <id>

# Short flag
gh blob delete -i <id>

# With token flag
gh blob delete --token "<token>" --id <id>

# With hostname for GitHub Enterprise Cloud with Data Residency
gh blob delete --hostname "enterprise.ghe.com" --id <id>

# Combined flags
gh blob delete --token "<token>" --hostname "enterprise.ghe.com" --id <id>
```

### Query all blobs

```bash
# Long flag
gh blob query-all --org <org>

# Short flag
gh blob query-all -o <org>

# With token flag
gh blob query-all --token "<token>" --org <org>

# With hostname for GitHub Enterprise Cloud with Data Residency
gh blob query-all --hostname "enterprise.ghe.com" --org <org>

# Combined flags
gh blob query-all --token "<token>" --hostname "enterprise.ghe.com" --org <org>
```

### Query blob

```bash
# Long flag
gh blob query --id <blob-id>

# Short flag
gh blob query -i <blob-id>

# With token flag
gh blob query --token "<token>" --id <blob-id>

# With hostname for GitHub Enterprise Cloud with Data Residency
gh blob query --hostname "enterprise.ghe.com" --id <blob-id>

# Combined flags
gh blob query --token "<token>" --hostname "enterprise.ghe.com" --id <blob-id>
```
