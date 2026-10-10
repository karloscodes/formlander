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
`karloscodes/formlander:1.3.0-rc.1` and moves `rc`. It does not move `latest`,
`v1`, or `v1.3`, so no customer server takes it on its nightly update.

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

The release train. Every Monday:

1. The candidate that waited 6 days becomes the stable release (`v1.3.0-rc.2` becomes `v1.3.0`).
2. When main holds changes after the stable release, main becomes the next candidate (`v1.4.0-rc.1`).

Servers update each night from one of two image tags:

| Tag | Holds | Who follows it |
|---|---|---|
| `latest` | the newest stable release | customer servers |
| `rc` | the newest candidate | your own server |

```bash
# A candidate now
gh workflow run "Release candidate"

# Promote a candidate now, without the wait
gh workflow run "Release candidate" -f promote=v1.3.0-rc.1

# Stop a bad candidate: fix main, then make a candidate now.
# The new one replaces it and waits its own 6 days.

# Stop the train
gh workflow disable "Release candidate"
```

## Merge Security Updates Workflow (`merge-security-updates.yml`)

A Dependabot security update merges by itself when the tests pass on its pull
request. The merge starts no release: the fix ships with the next release
candidate. A person merges all other Dependabot updates.
