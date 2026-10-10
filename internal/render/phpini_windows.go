package render

// cgiImpersonate: fastcgi.impersonate só existe no php-cgi do Windows (IIS);
// escrevê-lo desligado evita que o worker tente assumir a identidade do cliente.
const cgiImpersonate = true

// osManagedDirectives: no Windows o php.ini aponta curl.cainfo e
// openssl.cafile para o bundle de CAs que o HyPHP mantém. Os *.default_socket
// não são escritos (o mysqld.exe não tem socket Unix) e continuam livres para
// o usuário.
var osManagedDirectives = []string{"curl.cainfo", "openssl.cafile"}
