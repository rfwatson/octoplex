# Contributing

Thanks for contributing to Octoplex!

## Development

### Mise

Octoplex uses [mise](https://mise.jdx.dev/installing-mise.html) as a task
runner and environment management tool.

Once installed, you can run common development tasks easily:

Command|Shortcut|Description
---|---|---
`mise run test`|`mise run t`|Run unit tests
`mise run test_integration`|`mise run ti`|Run integration tests
`mise run lint`|`mise run l`|Run linter
`mise run format`|`mise run f`|Run formatter
`mise run generate_mocks`|`mise run m`|Re-generate mocks

### Tests

#### Integration tests

The integration tests (mostly in `/internal/app/integration_test.go`) attempt
to exercise the entire app, including launching containers and rendering the
terminal output.

Sometimes they can be flaky. Always ensure there are no stale Docker containers
present from previous runs, and that nothing is listening or attempting to
broadcast to localhost:1935 or localhost:1936.

## Opening a pull request

Pull requests are welcome, please propose significant changes in a
[discussion](https://github.com/rfwatson/octoplex/discussions) first.

1. Fork the repo
1. Make your changes, including test coverage
1. Run the formatter (`mise run format`)
1. Push the changes to a branch
1. Ensure the branch is passing
1. Open a pull request
