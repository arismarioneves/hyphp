# HyPHP website

<p><b>English</b> | <a href="README.pt-BR.md">Português</a></p>

The website at `https://ae8.com.br/hyphp/` is built from this folder. The design is open: layout, sections, typography, colors, illustrations, motion and the way the product is presented can all be rethought. Send your proposal as a pull request; the maintainer validates changes to the look of the site. The base rules below do not change.

## Base rules

1. **No build step.** Hand-written HTML, CSS and JavaScript that opens with a double click on `index.html`. No framework, bundler, `package.json`, TypeScript, Sass or ES modules. Under `file://` browsers block modules and `fetch()`, so scripts are classic and data comes in through `<script src>`.
2. **Nothing loaded from elsewhere.** Fonts, images and scripts live in this folder, with their licenses. Links to other sites are fine; CDNs are not.
3. **HyPHP brand.** The name, logo and icon (`assets/marca/`) stay as they are. Changes to the brand's main colors go through the maintainer's validation.
4. **Two languages.**
   - English is the default and Portuguese the second option.
   - Every text exists in both dictionaries in `assets/textos.js` and says the same thing in both.
   - Keys ending in `Html` carry markup and are fixed strings only, never outside data.
5. **Facts only.** Every sentence about HyPHP has proof in this repository's code ([Facts and proof](#facts-and-proof)). For text with a new fact, point to the proof in the PR. The site talks only about HyPHP and the competitors in the comparison, and the price is "Free", nothing else.
6. **Comparison** (`assets/comparacao.js`):
   - primary sources only: the official site, documentation, repository or changelog, or a maintainer's post on the project's channel;
   - a competitor block that states something has `fonte` (source) and `em` (date read);
   - without a source, the block states nothing (`suporte: null` with a `nota`);
   - neutral wording, no adjectives.

   How the comparison is shown is up to you.
7. **Download and history.**
   - The data comes from `releases/releases.js` (`window.HYPHP_RELEASES`), which already exists on the published site.
   - The Download button and the history link to this repository's release assets (`https://github.com/arismarioneves/hyphp/releases/download/v<version>/<asset>`).
   - Each published platform keeps what it needs: the Windows installer and the SmartScreen notice; on macOS, the dmg, the `install.sh` command and the Gatekeeper notice; on Linux, the `.deb`, the `apt install` command and the unsigned-package notice.
8. **Data files.**
   - Texts live in `assets/textos.js` and the comparison in `assets/comparacao.js`, in their current format.
   - Screenshots live in `assets/telas/en` and `assets/telas/pt`, with the same files in both folders, and use example projects and data only.
   - The `releases/`, `catalogo/` and `docs/` folders do not live in `web/`.
9. **Accessible and light.** With `prefers-reduced-motion`, nothing moves. No horizontal scrolling on phones (390×844). No console errors or warnings.

## Free to propose

Within the base rules, feel free to explore and send for validation:
- layout, sections and content order;
- colors, typography and local fonts;
- illustrations and animations;
- new screenshots;
- the gallery and how the comparison is presented;
- the behavior at each screen size.

Today, from a 1200×600 window up, everything fits without scrolling. That is a reference from the current design, not a rule.

## Previewing

Double-click `web/index.html`. To see the Download button and the history, download the published history (git ignores this copy):

```sh
curl -o web/releases/releases.js https://ae8.com.br/hyphp/releases/releases.js
```

## Before opening a pull request

```sh
node .github/scripts/site.mjs conferir
```

It needs only Node. The check rejects:
- build files;
- the `releases/`, `catalogo/` and `docs/` folders;
- resources loaded from elsewhere or missing;
- translation keys missing in one language;
- screenshots present in only one language;
- comparison blocks without a source.

In the PR, attach screenshots of the page on a desktop screen and at 390×844, in both languages (`?lang=en` and `?lang=pt`), and say what changed and why.

## Facts and proof

| Site text | Proof in this repository |
|---|---|
| Windows 10 and 11 · x64 · macOS 15+ · Apple Silicon | NSIS installer with WebView2 (`build/windows/nsis/project.nsi`); `LSMinimumSystemVersion` 15.0 in `build/darwin/Info.plist`; `build/darwin/install.sh` requires arm64 and macOS 15 |
| Mac notice: not notarized; if macOS blocks it, Privacy & Security › Open Anyway | ad hoc signature (`codesign --sign -`) in `build/darwin/Taskfile.yml`, no notarization; the command above the notice installs through `build/darwin/install.sh`, which downloads with `curl`, and `curl` sets no quarantine flag |
| macOS tab command (`curl -fsSL …/install.sh \| sh`) | header of `build/darwin/install.sh`; each release's `install.sh` asset comes from the `publicar` job in `.github/workflows/build.yml` (`cmd/hyphp-release`, `stageAssets`) |
| Ubuntu 24.04+ · Debian 13+ · Mint 22+ · x64 | `arch: "amd64"` and `depends` (`libwebkitgtk-6.0-4`, `libgtk-4-1`) in `build/linux/nfpm/nfpm.yaml`; the Linux package is built by a `runs-on: ubuntu-24.04` job in `.github/workflows/build.yml` |
| Linux notice: the `.deb` is not signed; `apt` installs the dependencies | `build/linux/nfpm/nfpm.yaml` has no signature section and lists the `depends` |
| Linux tab command (`sudo apt install ./hyphp-<version>-linux-amd64.deb`) | `mostrar` in `assets/site.js`, with the asset name from the release's `linux_amd64` (`internal/update/manifest.go`) |
| PHP environment declared per project | PHP version in `hyphp.yaml` (`internal/project/manifest.go`, `Manifest.PHP`) |
| Apache or nginx vhost, .test domain, local HTTPS, MySQL or MariaDB, and queue processes | `internal/webserver/*`, `internal/netcfg/hosts.go`, `internal/netcfg/mkcert.go`, `internal/stack/mysql.go` (`DBRuntime`), `internal/stack/desired.go` (`procSpecs`) |
| No Docker | supervised native processes (`internal/supervisor/process_windows.go`, `internal/supervisor/process_unix.go`) |
| Includes: PHP 7.2 to 8.5, Apache or nginx, MySQL or MariaDB, Mailpit, phpMyAdmin, local HTTPS, automatic updates | `internal/pkgmgr/catalog.json`, `internal/update/updater.go` |
| Includes: php.ini per version | the user's directives per series in `state.PHPIni`, written at the end of php.ini by `internal/render/phpini.go` (`RenderPHPIni`); panel in `frontend/src/screens/PhpIniPanel.tsx` |
| Includes: CLI | `cmd/hyphp`, `internal/cli`, `services/cli_service.go` |
| Screens › Stack, projects, ports and warnings on one screen; one pool of php-cgi workers per PHP version | `frontend/src/screens/Dashboard.tsx`; `internal/stack/desired.go` (`php:<series>:<n>` specs, `PoolSize`) |
| Screens › Projects: PHP version, domain, docroot, extensions, database and processes | `internal/project/manifest.go` (`Manifest`), `internal/stack/desired.go` (`procSpecs`) |
| Screens › PHP 7.2 to 8.5 from the official builds, checked by SHA-256; directives per version | `internal/pkgmgr/catalog.json`, `internal/pkgmgr/download.go` (`verifySHA256`), `frontend/src/screens/PhpIniPanel.tsx` |
| Screens › MySQL or MariaDB: credentials, connection command, databases and phpMyAdmin | `frontend/src/screens/Database.tsx`, `services/database_service.go` |
| Screens › mail() lands in Mailpit, inside the app | `internal/render/phpini.go` (`SMTP`, `smtp_port`), `frontend/src/screens/Mail.tsx` |
| Screens › Switch Apache or nginx, MySQL or MariaDB; if the new one fails to start, the previous one stays up | `internal/stack/stack.go` (`SwitchWebServer`, `SwitchDatabase`) |
| Screens › A CLI for the terminal and AI agents, with `--json` and exit codes; the tab shows who called what | `cmd/hyphp/main.go`, `services/cli_handlers.go`, `services/cli_service.go` (`Activity`), `internal/cli/protocol.go` (`HeaderCaller`), `frontend/src/screens/Cli.tsx` |

The one exception is the release notes in the history: they appear as published, in the chosen language.

## License

Contributions are licensed under this repository's [MIT License](../LICENSE) ([CONTRIBUTING.md](../CONTRIBUTING.md)). The fonts keep their own licenses (`assets/fontes/LICENSE-*`).
