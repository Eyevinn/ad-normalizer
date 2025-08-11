# go-default

This is a default setup for Go projects.

Use `internal` directories for code that should not be exported.
Use `testdata` directories for test files.

## Get started

1. Run "go mod init github/Eyevinn/name"
2. Edit the installed files appropriately
3. Place code for executables in directories with proper names under cmd/cmd1 etc
   and update the Makefile
4. To run the license check program `wwhrd`. Check its web site for how to install
5. The readme.md file requires quite some work to be updated. Check [mp4ff][mp4ff]
   for an example how to update it.
6. For code coverage (open source projects), you can use coveralls.io in the same
   way as [mp4ff][coveralls].
   

## Included

The defaults for all go projects include:

- A .gitignore file
- Github actions for running tests and golang-ci-lint
- A Makefile for running tests, coverage, and update dependencies.
  Exchange cmd1, cmd2 with the names of your binaries.
- A README skeleton (update badges to Go, see e.g. mp4ff)
- A CHANGELOG.md file that should be changed manually
- A config file for pre-commit (see https://pre-commit.com)

[mp4ff]: https://github.com/Eyevinn/mp4ff
[coveralls]: https://coveralls.io/github/Eyevinn/mp4ff