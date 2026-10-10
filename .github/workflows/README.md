# Release Workflows

This directory contains GitHub Actions workflows for continuous integration and deployment.

## CI Workflow (`ci.yml`)

Runs on every pull request and push to main:
- Unit tests
- E2E tests with Playwright

**Note:** Pushing to main does NOT publish any artifacts or Docker images.

## Production Release Workflow (`release-goreleaser.yml`)

Runs when a version tag (e.g., `v1.0.0`) is pushed:
- Uses [GoReleaser](https://goreleaser.com/) for artifact generation
- Publishes cross-platform binaries for:
  - Linux (amd64, arm64)
  - macOS (amd64, arm64)
- Creates GitHub Release with changelog
- Publishes Docker images to `karloscodes/formlander` with semantic versioning
- Tags: `latest`, `v1.2.3`, `v1.2`, `v1`

**Trigger:** Push tag matching `v*` pattern

A tag with a hyphen (`v1.3.0-rc.1`) is a release candidate. It publishes
`karloscodes/formlander:1.3.0-rc.1` only. It does not move `latest`, `v1`, or
`v1.3`, so no server takes it on its nightly update.

### Docker Tag Strategy

When you release `v1.2.3`, the following tags are created:
- `karloscodes/formlander:latest` - Always points to newest stable
- `karloscodes/formlander:v1.2.3` - Exact version pin
- `karloscodes/formlander:v1.2` - Receives patch updates (1.2.x)
- `karloscodes/formlander:v1` - Receives minor + patch updates (1.x.x)

This allows users to choose their update strategy:
```bash
# Always get latest stable
docker pull karloscodes/formlander:latest

# Pin to major version (get features + patches)
docker pull karloscodes/formlander:v1

# Pin to minor version (get patches only)
docker pull karloscodes/formlander:v1.2

# Pin to exact version (no updates)
docker pull karloscodes/formlander:v1.2.3
```

### Creating a Release

```bash
# Tag the release
git tag -a v1.0.0 -m "Release v1.0.0"

# Push the tag
git push origin v1.0.0
```

GoReleaser will automatically:
1. Build binaries for all platforms
2. Generate checksums
3. Create GitHub release with changelog
4. Upload release artifacts
5. Build and push multi-arch Docker images

## Release Candidate Workflow (`release-candidate.yml`)

Runs every Monday. When main holds changes after the last stable release, it
tags the next release candidate and starts the release.

```bash
# Run the candidate on a server
chasen deploy --tag 1.3.0-rc.1

# Promote the candidate to a stable release
git tag v1.3.0 v1.3.0-rc.1 && git push origin v1.3.0
```

## Merge Security Updates Workflow (`merge-security-updates.yml`)

A Dependabot security update merges by itself when the tests pass on its pull
request. The merge starts no release: the fix ships with the next release
candidate. A person merges all other Dependabot updates.
