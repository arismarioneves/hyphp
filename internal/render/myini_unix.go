//go:build darwin || linux

package render

// myIniLogError: fora do Windows não há --console; com
// log-error o mysqld escreveria no mesmo log/mysql.log que o supervisor grava
// (dois escritores num arquivo) e a saída do init iria para lá em vez do
// mysql-init.log. Sem ele o servidor escreve no stderr.
const myIniLogError = false

// myIniOpenFilesLimit: app de GUI pode herdar um limite soft baixo (256 no
// macOS), e com isso o mysqld reduz max_connections e
// table_open_cache. Com a opção o mysqld sobe o próprio limite soft ao
// iniciar (setrlimit), dentro do limite hard.
const myIniOpenFilesLimit = 4096
