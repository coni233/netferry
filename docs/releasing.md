# Releasing NetFerry

Releases are built by `.github/workflows/release.yml` when a version tag is
pushed.

## Create a release

```bash
git tag v0.1.0
git push origin v0.1.0
```

The workflow:

1. Runs `go vet` and all tests.
2. Builds Windows, Linux, and macOS binaries for amd64 and arm64.
3. Injects the tag version into `netferry --version`.
4. Packages each binary with `README.md`, `LICENSE`, and platform installer
   scripts.
5. Generates `checksums.txt`.
6. Creates a GitHub Release with generated release notes, prefixed by
   `.github/release-notes.md`, which explains how to choose the right archive.

The version in the tag must use the `vX.Y.Z` form. Release archives use the
following names:

```text
netferry-windows-amd64.zip
netferry-windows-arm64.zip
netferry-linux-amd64.tar.gz
netferry-linux-arm64.tar.gz
netferry-darwin-amd64.tar.gz
netferry-darwin-arm64.tar.gz
checksums.txt
```

If a workflow is retried after a partial publish, existing assets are replaced
with `gh release upload --clobber`.
