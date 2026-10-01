package render

// cgiImpersonate: fastcgi.impersonate só existe no php-cgi do Windows (IIS);
// escrevê-lo desligado evita que o worker tente assumir a identidade do cliente.
const cgiImpersonate = true
