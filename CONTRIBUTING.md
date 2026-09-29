# Contributing to HyPHP

Thanks for your interest. Issues and pull requests are welcome.

## Licensing of contributions

By submitting a pull request, you agree to the [CLA](CLA.md) (Contributor License Agreement, based on the Apache ICLA) for that contribution and for your future ones. There is no form to fill in and no box to tick.

You remain the owner of what you wrote. What the CLA does is authorize the maintainer to distribute your contribution, including in the paid parts of HyPHP (the `enterprise/` directory). Without it, third-party code could not go into the product without risk.

## How the license works

- **Outside `enterprise/`**: [PolyForm Shield 1.0.0](LICENSE). Anyone, individuals or companies, can use, study, modify and share it, including at work. What is not allowed is offering a product that competes with HyPHP or with its paid features, whether paid or free. That includes selling HyPHP itself.
- **Inside `enterprise/`**: [its own license](enterprise/LICENSE). This is where the paid features will live, and using them requires a subscription. The code is visible, and you may modify it to develop and test contributions.

This summary does not replace the license texts.

## Before opening a pull request

```powershell
go build . ./cmd/... ./services/... ./internal/...
go test . ./cmd/... ./services/... ./internal/...
npx --prefix frontend tsc --noEmit -p frontend
```

In code comments, explain why each decision was made; the code already says what it does.
