package render

// myIniLogError: no macOS não há --console (só existe no Windows); com
// log-error o mysqld escreveria no mesmo log/mysql.log que o supervisor grava
// (dois escritores num arquivo) e a saída do init iria para lá em vez do
// mysql-init.log. Sem ele o servidor escreve no stderr.
const myIniLogError = false

// myIniOpenFilesLimit: apps de GUI abertos pelo launchd herdam limite soft de
// 256 descritores, e com isso o mysqld reduz max_connections e
// table_open_cache. Com a opção o mysqld sobe o próprio limite soft ao
// iniciar (setrlimit), dentro do limite hard.
const myIniOpenFilesLimit = 4096
