# arc-git

Git integration with AI-powered features for the Arc toolkit.

## Features

- **annotate** - Add AI-generated annotations to commits

## Installation

```bash
go install github.com/mtreilly/arc-git@latest
```

## Usage

```bash
# Annotate recent commits
arc-git annotate --since 10

# Annotate a specific commit range
arc-git annotate --from HEAD~5 --to HEAD

# View annotations in git log
git log --show-notes=ai

# Search AI-generated notes
git log --grep "refactor" --notes=ai
```

## License

MIT
