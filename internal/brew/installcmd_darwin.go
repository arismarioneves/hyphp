package brew

// InstallCommand é o comando oficial de instalação do Homebrew, exibido para o
// usuário copiar. O HyPHP não o executa: pede senha de administrador e as
// Command Line Tools do Xcode.
const InstallCommand = `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
