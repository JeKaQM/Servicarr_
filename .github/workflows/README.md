# GitHub Actions workflows

The repository validates every pull request before changes reach `main`, then publishes a release only after the same commit passes CI on `main`.

## Workflows

### CI (`ci.yml`)

Runs Go formatting and vet checks, verifies version sources, executes Go and JavaScript tests, builds the application, and verifies a Docker container can start. It runs for pull requests and configured branch pushes, including every push to `main`.

### Pull request checks (`pr-checks.yml`)

Validates the PR title and merge state, lists the changed files, and runs the Go security scan.

### Status check (`status-check.yml`)

Publishes a concise summary of the checks expected before merge.

### Release (`release.yml`)

Runs after `CI` succeeds for a push to `main`. Each run:

1. Verifies the tested commit is still in `main`.
2. Calculates a deterministic semantic version from the release-line version and first-parent commit distance.
3. Builds and publishes immutable version and commit tags to GHCR.
4. Verifies the published container reports the calculated version.
5. Updates floating container tags only when releasing the current `main` commit.
6. Creates a GitHub Release with generated notes.

Release jobs use commit-specific concurrency and immutable tags, so qualifying commits are not discarded when merges happen close together. Failed or cancelled CI runs do not create releases.

### Docker snapshot (`docker-build.yml`)

Provides a manual workflow for publishing explicitly marked snapshot images. Snapshots never replace stable `latest` or semantic-version tags.

## Version policy

`app/internal/buildinfo/VERSION` defines the current release-line base. `package.json` and `package-lock.json` must match it. The release calculator advances the patch once per first-parent commit after that version was established.

To start a new major or minor line, update all three version sources in one pull request. The merge of that pull request releases the exact requested version; later merges advance its patch number automatically.

## Local validation

```bash
go test ./...
npm test -- --runInBand
go vet ./...
docker build --build-arg APP_VERSION=1.0.1 --build-arg VCS_REF=local -f deploy/Dockerfile -t servicarr:test .
docker run --rm --entrypoint status servicarr:test --version
```
