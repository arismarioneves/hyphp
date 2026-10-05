package render

// myIniLogError: no Windows o my.ini continua com log-error; o --console dos
// args tem precedência e o log real sai no stdout que o supervisor captura.
const myIniLogError = true

// myIniOpenFilesLimit 0 não emite a linha: o mysqld.exe não tem o limite de
// descritores do Unix e o golden do Windows fica igual.
const myIniOpenFilesLimit = 0
