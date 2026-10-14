# Contributing to HyPHP

Thanks for your interest. Issues and pull requests are welcome.

## How the license works

- **Outside `enterprise/`**: [MIT](LICENSE). Anyone, individuals or companies, can use, study, modify, share and sell it.
- **Inside `enterprise/`**: [its own license](enterprise/LICENSE). This is where the paid features live, and using them requires a subscription. The code is visible, and you may modify it to develop and test contributions.

The name "HyPHP" and its logo are not covered by these licenses: a fork must use another name and logo.

This summary does not replace the license texts.

## Licensing of contributions

- **Outside `enterprise/`**: your contribution is licensed under the MIT License, like the code it changes. There is nothing to sign.
- **Inside `enterprise/`**: by submitting a pull request, you agree to the [CLA](CLA.md) (Contributor License Agreement, based on the Apache ICLA) for that contribution and for your future ones in `enterprise/`. There is no form to fill in and no box to tick. You remain the owner of what you wrote; the CLA authorizes the maintainer to distribute it under the `enterprise/` license.

## Before opening a pull request

```powershell
go build . ./cmd/... ./services/... ./internal/...
go test . ./cmd/... ./services/... ./internal/...
npx --prefix frontend tsc --noEmit -p frontend
```

In code comments, explain why each decision was made; the code already says what it does. The full project rules (comments, tests, commits, checks) are in [AGENTS.md](AGENTS.md), in Portuguese.

## Website

The website lives in [`web/`](web/): plain HTML, CSS and JavaScript, with no build step. Read [web/README.md](web/README.md) for its rules and checks before opening a pull request.
