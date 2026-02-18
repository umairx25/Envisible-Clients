# Envisible Clients

  - [Overview](#overview)
  - [Local Development](#local-development)
  - [Publishing](#publishing)

### Overview
Client SDKs and CLI for Envisible, a secure secret management platform.

- Website and Docs: [envisible.dev](https://envisible.dev)
- SDKs and CLI documentation:
  - [Python SDK](python-sdk/README.md)
  - [Node.js SDK](node-sdk/README.md)
  - [CLI](cli/README.md)

### Local Development

#### Node.js SDK

1. **Install dependencies**
   ```bash
   cd node-sdk
   npm install
   npm link
   ```
2. **Run locally**
   ```bash
   # In an arbitrary folder run (afterwards, use normally):
   npm link envis-node
   ```
   ```bash
   # To unlink the folder:
   npm unlink envis-node
   ```

#### CLI

1. **Build**
   ```bash
   cd cli
   go build -o envis .
   ```
2. **Run**
   ```bash
   ./envis login
   ```

#### Python SDK (PyPI)
Always publish to TestPyPI first, verify the package, then upload the exact same artifacts to the real PyPI.

1. **Bump version** in `python-sdk/envis/src/envis/__about__.py`.
2. **Clean & build**
   ```bash
   cd python-sdk/envis
   rm -rf dist build *.egg-info
   python3 -m build
   python3 -m twine check dist/*
   ```
3. **Install**
   ```bash
   python3 -m pip install -e .
   ```

### Publishing

This repo uses Git tags to trigger GitHub Actions release workflows. Each workflow listens for a different tag prefix.

**Tag Prefixes**

- CLI: `vX.Y.Z` triggers `.github/workflows/cli-release.yml`
- Python SDK: `python/vX.Y.Z` triggers `.github/workflows/python-release.yml`
- Node SDK: `node/vX.Y.Z` triggers `.github/workflows/node-release.yml`

**General Tag Workflow**

1. Make your code changes.
2. Bump the version in the relevant package file:
- CLI: no version file enforced by workflow.
- Python: `python-sdk/envis/pyproject.toml` must match the tag.
- Node: `node-sdk/package.json` must match the tag.
3. Commit changes.
4. Create the tag at the commit you want to release.
5. Push the commit and the tag.

**Create and Push a New Tag**

Example for each workflow:

```bash
# CLI
git tag v1.2.3
git push origin v1.2.3

# Python
git tag python/v1.2.3
git push origin python/v1.2.3

# Node
git tag node/v1.2.3
git push origin node/v1.2.3
```

**Delete and Recreate a Tag (Re-release)**

Use this when a tag already exists and you need to republish it to point at a new commit.

1. Delete the local tag.
2. Delete the remote tag.
3. Recreate the tag on the correct commit.
4. Push the tag again.

```bash
# Delete local
git tag -d <tag>

# Delete remote
git push origin :refs/tags/<tag>

# Recreate tag on current commit
git tag <tag>

# Push recreated tag
git push origin <tag>
```

**Force Move a Tag to a Specific Commit**

If you want to point a tag at a specific commit SHA:

```bash
git tag -f <tag> <commit_sha>
git push origin :refs/tags/<tag>
git push origin <tag>
```

**Common Errors**

- Tag/version mismatch: update the version file or change the tag.
- Tag already exists: delete and recreate the tag as shown above.
- Workflow not running: ensure the tag prefix matches the workflow.
