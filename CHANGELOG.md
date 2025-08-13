# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] - 2025-08-XX

Complete rewrite of the service in go.
Due to perforamnce issues related to spiky traffic patterns, the decision was made to rewrite the service in go due to better concurrency support and the ability to scale vertically when running in a cloud environment.

The rewrite is API compatible, meaning that there should be no difference running the new version as a docker image with the same setup as the node.js versions. However, the requirements for developing and runing the service locally has changed due to the new language and runtime required.

Benchmarking has shown a noticeable performance improvement when handling spiky traffic, and no noticeable drops for other use cases.

### Added

- ...

### Changed

- ...

### Removed

- ...

## [0.1.0] - 2024-01-15

### Added

- initial version of the repo

[Unreleased]: https://github.com/Eyevinn/mp2ts-tools/releases/tag/v0.1.0...HEAD
[0.1.0]: https://github.com/Eyevinn/mp2ts-tools/releases/tag/v0.1.0
