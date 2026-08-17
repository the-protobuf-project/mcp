# Publishing to the Bazel Central Registry (BCR)

This directory holds the templates that
[`publish-to-bcr`](https://github.com/bazel-contrib/publish-to-bcr) reads to
open a pull request against the [Bazel Central
Registry](https://github.com/bazelbuild/bazel-central-registry) every time we
cut a release.

| File | Purpose |
| --- | --- |
| [`metadata.template.json`](metadata.template.json) | Module homepage, maintainers, repo. `versions` is auto-populated per release. |
| [`source.template.json`](source.template.json) | Where BCR fetches the release source archive. `{OWNER}`/`{REPO}`/`{TAG}`/`{VERSION}` and `integrity` are filled in automatically. |
| [`presubmit.yml`](presubmit.yml) | The build/test matrix BCR's CI runs to validate the module. |

The workflow that drives this is
[`.github/workflows/publish-bcr.yaml`](../.github/workflows/publish-bcr.yaml).
It runs automatically when a GitHub release is published (and can be triggered
manually for a tag via **Actions → Publish to BCR → Run workflow**).

## One-time maintainer setup

These steps only need to be done once for the org. Until they're done, the
publish workflow will fail.

### 1. Fork the registry

Create **`the-protobuf-project/bazel-central-registry`** as a fork of
[`bazelbuild/bazel-central-registry`](https://github.com/bazelbuild/bazel-central-registry).
This is the `registry_fork` the workflow pushes the entry branch to before
opening the PR upstream.

### 2. Create a publish token

Generate a **Classic** Personal Access Token with the `repo` and `workflow`
scopes (these are what let it push a branch to the fork and open a PR).

Save it as a repository or organization secret named **`BCR_PUBLISH_TOKEN`**
(Settings → Secrets and variables → Actions → New secret).

> Use a Classic PAT, not a fine-grained one — `publish-to-bcr` needs the
> `workflow` scope, which fine-grained tokens don't expose.

### 3. Verify the presubmit matrix

[`presubmit.yml`](presubmit.yml) currently tests Bazel `7.x` and `8.x`. Our
repo pins Bazel `9.1.1` (see [`.bazelversion`](../.bazelversion)) and uses
`protobuf 35.0-rc1`. If the proto target ever needs a newer Bazel than the
matrix lists, the BCR PR's CI will fail — bump the `bazel:` list in
`presubmit.yml` accordingly.

You can sanity-check the published target locally:

```bash
bazel build //protobuf/mcp/v1:mcppb_proto
```

## What happens on release

1. The [Release workflow](../.github/workflows/release.yaml) publishes a
   GitHub release for the `vX.Y.Z` tag.
2. That fires [`publish-bcr.yaml`](../.github/workflows/publish-bcr.yaml), which
   calls the `publish-to-bcr` reusable workflow.
3. It fills in `source.json` (archive URL + integrity) and `metadata.json`
   from these templates, pushes a branch to the fork, and opens a PR against
   `bazelbuild/bazel-central-registry`.
4. A BCR maintainer reviews and merges the PR. The module then resolves via
   `bazel_dep(name = "mcp", version = "X.Y.Z")`.

## Notes

- **Version stamping.** The `module(version = ...)` in
  [`MODULE.bazel`](../MODULE.bazel) is overridden by `publish-to-bcr` using the
  release tag, so you don't need to bump it per release.
- **Attestations are off.** `publish-bcr.yaml` sets `attest: false`. Turning
  them on requires the source archive to be attached as a release asset in the
  same run; the Release workflow currently uploads only binaries. Enable
  `attest: true` once a source archive is added to the release.
- **Dependencies must exist in BCR.** All non-dev deps (`protobuf`,
  `googleapis`) are already in the registry. `rules_go`/`gazelle` are
  `dev_dependency` and are stripped for consumers, so the only consumer-facing
  buildable target is the proto library — which is what the presubmit verifies.
