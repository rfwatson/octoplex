# Contributing

Thanks for contributing to Octoplex!

## Development

### Quick start

1. Install Mise

See https://mise.jdx.dev/installing-mise.html

```shell
curl https://mise.run | sh
```

2. Clone repo

```shell
git clone https://github.com/rfwatson/octoplex.git
cd octoplex
mise install
go generate ./...
```

3. Run Octoplex

Run the Go server:

```shell
go run . server start --server-url http://localhost:5173
```

In a different terminal, run the Vite dev server:

```
cd frontend/
pnpm run dev
```

Now you can visit http://localhost:5173 to launch the web interface.

4. Run the TUI client:

In yet another terminal:

```shell
go run . client start --tls-skip-verify
```

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

See `mise/config.yml` and `mise/tasks` for more.

## Opening a pull request

Pull requests are welcome, please propose significant changes in a
[discussion](https://github.com/rfwatson/octoplex/discussions) first.

1. Fork the repo
1. Make your changes, including test coverage
1. Run the formatter (`mise run format`)
1. Push the changes to a branch
1. Ensure the branch is passing
1. Open a pull request
