package brew

// InstallCommand é o comando oficial de instalação do Homebrew, exibido para o
// usuário copiar, precedido dos pacotes que a documentação do Homebrew pede no
// Debian e no Ubuntu. O HyPHP não o executa: pede senha de administrador.
const InstallCommand = `sudo apt-get install -y build-essential procps curl file git && /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
