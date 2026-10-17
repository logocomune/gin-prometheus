# Changelog

## Unreleased

### Added

- Add regression, property, and fuzz tests for streamed request-body measurement and characterization tests for collector registration.
- Add authentication characterization, property, and fuzz tests, including malformed headers and credential length mismatches.

### Fixed

- Count unknown-length request bodies during handler reads instead of buffering the remaining body after request handling.
- Correct request-size and path-aggregator documentation and document explicit collector registration.

### Changed

- Align README requirements with Go 1.26.0 and the current Gin and Prometheus dependency versions.
- Document dependency verification procedures, Go 1.26 compatibility, and the remaining dependency advisories from the targeted audit.
- Record consumed body bytes for unknown-length requests, excluding estimated headers; partial reads report only consumed bytes and unread bodies report zero.
- Harden Basic Auth comparisons with fixed-length SHA-256 digests and unconditional constant-time checks for both credentials, preserving authentication responses.

### Removed

- Remove unused private path and metric wrappers, request buffering helpers, response-size helpers, and legacy collector variables without changing the public API.
- Remove tests dedicated solely to the deleted private helpers; retain active middleware regression coverage.
