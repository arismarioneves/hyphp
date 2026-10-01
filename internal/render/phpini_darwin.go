package render

// cgiImpersonate: no macOS os workers são php-fpm, que não conhece
// fastcgi.impersonate; a diretiva só geraria ruído no php.ini.
const cgiImpersonate = false
