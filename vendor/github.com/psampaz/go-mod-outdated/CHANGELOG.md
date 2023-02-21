# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)

## [0.9.0]  2023-02-19
### Added
- Goreleaser
- Tests for Go 1.19 and 1.20
### Changed
- Skip rendering the table if there are no updates to display https://github.com/psampaz/go-mod-outdated/pull/46
- Updated dependencies to latest versions
- Update ci config
### Removed
- Tests for Go 1.16 and 1.17

## [0.8.0] 2021-04-12
### Added
- Tests for Go 1.16

### Changed
- Updated docker base image to 1.16.3
- Updated dependencies to latest versions
- Updated version of golangci-lint to 1.37.1

### Removed
- Tests for Go 1.13

## [0.7.0] 2020-09-26
### Added
- Run tests for Go 1.15

### Removed
- Tests for Go 1.11 and Go 1.12

### Changed
- Updated docker base image to 1.15.2
- Updated version of golangci-lint to 1.31

## [0.6.0] 2020-04-09
### Added
- Added -style markdown option
- Added tests for Go 1.14

### Changed
- Updated docker base image to 1.14.2
- Reduced docker image size
- Updated version of golangci-lint to 1.24

## [0.5.0] 2019-09-27 
### Added
- Run tests on Go 1.13

### Changed
- Updated docker base image to 1.13.1
- Replaced Travis with Github Actions
- Updated version of golangci-lint to 1.18

## [0.4.0] 2019-08-12
### Added
- Run go-mod-outdated using Docker

## [0.3.0] 2019-05-01
### Added
- Flag '-ci' to exit with non-zero exit code when an outdated dependency is found
- osx in travis

### Removed
- tip version in travis

## [0.2.0] - 2019-04-22
### Added
- Extra column 'VALID TIMESTAMPS' which indicates if the timestamp of the new version is
actually newer that the current one 

### Changed
- Packages are now internal

## [0.1.0] - 2019-04-22
### Added
- Initial release
