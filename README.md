<p align="center">
  <img src=".github/assets/banner.png" alt="HyPHP window running Apache, PHP 7.4, 8.3 and 8.4, MySQL and Mailpit">
</p>

<h1 align="center">HyPHP</h1>

<p align="center">PHP development environment for Windows.<br>Multiple PHP versions side by side, each project declares its own. No Docker.</p>

<p align="center"><b>English</b> | <a href="README.pt-BR.md">Português</a></p>

A PHP development environment with its own orchestrator: several PHP versions serving
different domains **at the same time**, local HTTPS, supervised workers and a reproducible
environment per project.

Download the installer from the [Releases](https://github.com/arismarioneves/hyphp/releases/latest)
page (Windows 10/11 x64). It is not digitally signed yet: if Windows warns you, choose
**More info → Run anyway**.

## Why it exists

| Tool | Limitation |
|---|---|
| **XAMPP** | `mod_php`: one PHP version per installation. No local domains, no local HTTPS, orphan processes after a crash. |
| **Laragon** | Sound architecture, but closed, with no published source. The PHP version is switched from a menu or a *Profile*, applies to the whole installation, and is not declared in the project. |
| **WampServer** | PHP per VirtualHost through FCGI since 3.2.8 (official changelog), but Apache only, and the configuration lives in the installation's menus and files, not in the project. |
| **DDEV / Devilbox / Lando** | Solid and open source, but they require Docker — 2–4 GB of RAM before the first request. |

HyPHP brings together on Windows, without Docker, what the others deliver separately:
**several PHP versions serving at the same time**, the version **declared in the project
itself** (`hyphp.yaml`, committable), Apache or nginx, and supervised workers.

## What it does differently

- **Real simultaneous multi-version** — `php-cgi` in external FastCGI mode, one pool per
  version, a single Apache (or nginx) in front. Version declared per project.
- **Worker pool** — on Windows, `php-cgi` serves one request at a time; without a pool, one
  slow request locks up the site. Measured: 2 requests of 3 s took 6.2 s with 1 worker and
  3 s with 2.
- **Zero orphan processes** — guaranteed by the kernel, through a Job Object with
  kill-on-close.
- **Real status** — every service has a readiness probe; the UI shows `degraded` when the
  process is alive but not responding.
- **Reproducible environment** — `hyphp.yaml` committed to the project's repository.
- **Config is output, not input** — `etc/` is generated and validated (`httpd -t` /
  `nginx -t`) before being applied; an invalid config never takes the environment down.
- **MySQL or MariaDB** — one active at a time on the same port, each with its own data
  directory. Switching is immediate and rolls back if the new one does not come up.
- **php.ini per version** — directives such as `max_input_vars` are edited in the app for
  each PHP version, showing the effective value and where it comes from.
- **English or Portuguese**, dark or light theme (or the same as Windows).

## Platforms

The target is **Windows 10/11 x64** — deliberately the hardest case, since `php-fpm` does
not exist on this platform. The architecture isolates OS-specific code in files with build
tags; porting to macOS and Linux is simpler, because `php-fpm` exists there and removes the
most complex piece of the design.

## `hyphp.yaml`

```yaml
name: acme
domain: acme.test          # default: <name>.test
wildcard: false            # true enables *.acme.test through the local DNS resolver
php: "8.1"                 # missing = global default version
docroot: public            # default: public if it exists, otherwise the root
extensions: [pdo_mysql, intl, zip, gd]
database: acme_dev         # created if missing
processes:
  queue: php artisan queue:work --tries=3
  scheduler: php artisan schedule:work
```

## Stack

Go 1.26 · Wails v3 (WebView2) · React 18 + TypeScript · Tailwind v4 · Phosphor Icons

Orchestrated components, downloaded from their official sources, each under its own license:

| Component | License | Source |
|---|---|---|
| PHP | PHP License 3.01 | windows.php.net |
| Apache httpd | Apache-2.0 | apachelounge.com |
| nginx | BSD-2-Clause | nginx.org |
| MySQL Community | GPL-2.0 | dev.mysql.com |
| MariaDB Server | GPL-2.0 | mariadb.org |
| Mailpit | MIT | github.com/axllent/mailpit |
| mkcert | BSD-3-Clause | github.com/FiloSottile/mkcert |

HyPHP does not redistribute these binaries: it downloads them on demand, verifies the
SHA-256 and extracts them into `bin/`. Any compatible build placed manually in `bin/` is
recognized as well.

## Development

Requires Go 1.26+, Node/npm and the Wails v3 CLI:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.23
wails3 doctor     # checks the environment
wails3 dev        # development with hot reload
wails3 build      # binary in bin/
```

Building the installer requires [NSIS](https://nsis.sourceforge.io/) on `PATH`
(`winget install NSIS.NSIS` installs it into `C:\Program Files (x86)\NSIS`, which the
installer does not add to `PATH` automatically):

```powershell
$env:PATH = 'C:\Program Files (x86)\NSIS;' + $env:PATH
wails3 task windows:package   # bin/hyphp-amd64-installer.exe
```

The installer ships `hyphp.exe` and `hyphp-helper.exe`. The helper is the binary with a
`requireAdministrator` manifest that performs the elevated network actions (writing to
`hosts`, installing the local root certificate, the wildcard DNS rule); without it next to
the main executable, those actions fail. The fourth and last action that asks for UAC is
installing an update.

### Installation

The installer supports both NSIS scopes:

```powershell
wails3 task windows:package                      # machine (Program Files, asks for UAC)
wails3 task windows:package INSTALL_SCOPE=user   # user (no UAC)
hyphp-amd64-installer.exe /S /D=C:\path          # silent, directory of your choice
```

The data root (`bin/`, `etc/`, `var/`, `log/`) lives **next to the executable if that
directory is writable**, otherwise in `%LOCALAPPDATA%\HyPHP`. That is what allows installing
into Program Files without the app needing privileges to work. `HYPHP_ROOT` overrides the
rule.

Uninstalling removes the installation directory, but does **not** undo what the user
applied to the system: the `hosts` block, the `.test` DNS rule and the autostart entry are
removed through the interface itself (the **Permissões** card in **Configurações**, the
Settings screen, and the autostart toggle), before uninstalling.

### Updates

The app checks for the newest release at
`https://github.com/arismarioneves/hyphp/releases/latest/download/latest.json`
one minute after it opens and every 6 hours (can be turned off in **Configurações ›
Atualizações**), and downloads the new installer in the background. The manifest is signed
with ed25519 (public key in `internal/update/key.go`) and carries the installer's SHA-256;
the app rejects a manifest or an installer that does not match. Installation only happens
when you click **Atualizar e reiniciar** (update and restart): the services stop, Windows
asks for permission once, and the app comes back on its own in the new version. Development
builds (without `-tags production`) do not take part.

### Publishing a version

The version lives in four places, which `hyphp-release` checks before publishing:
`internal/version/version.go`, `info.version` in `build/config.yml`,
`build/windows/info.json` and `INFO_PRODUCTVERSION` in
`build/windows/nsis/wails_tools.nsh`. With the `v<version>` tag already on GitHub:

```powershell
wails3 task windows:package
go run ./cmd/hyphp-release -nota "What changed" -nota "Another change"
```

The command signs `latest.json` with the release key and creates the release with the
installer, the manifest and the signature (via `gh`). The release is immutable: the notes
must be right before publishing.

## Support the project

HyPHP is free. If it saves you time, you can support its development:

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-arismarioneves-FFDD00?logo=buymeacoffee&logoColor=000000)](https://buymeacoffee.com/arismarioneves)

## License

The code is open to read, following the *open core* model of Chatwoot and GitLab:

- **Everything outside `enterprise/`** is under the [PolyForm Shield 1.0.0](LICENSE).
  Anyone, individuals or companies, can use HyPHP for free, including at work, and study,
  modify and share the code. What it does not allow is offering a product that competes
  with HyPHP or with its paid features, whether paid or free, which includes selling HyPHP
  itself.
- **`enterprise/`** will hold the paid features, under [its own license](enterprise/LICENSE)
  and subscription-based use. Today it only contains the license: there are no paid
  features yet.

By submitting a pull request, you agree to the [CLA](CLA.md); see
[CONTRIBUTING.md](CONTRIBUTING.md).

The components HyPHP downloads (table in [Stack](#stack)) each follow their own license.
