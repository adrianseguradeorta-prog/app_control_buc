# Control de Buceadores — portátil, servidor LAN y actualizaciones

Aplicación Go autocontenida para Windows x64 y ARM64. El usuario solo descomprime el paquete de su arquitectura y abre `CONTROL_BUCEADORES/EJECUTAR.bat`. No necesita Go, Git, Python, Node, base de datos ni instalador. Un único servidor utiliza los datos; los clientes acceden con su navegador.

## Distribución y privacidad

Código y Releases: https://github.com/adrianseguradeorta-prog/app_control_buc (repositorio público elegido por el usuario). No contiene documentos originales ni datos operativos. Las fixtures de `app/testdata` y los dos archivos de `reference` son **sintéticos**, con personas e inmersiones ficticias. Los materiales originales se conservan fuera de este checkout y no se publican.

El launcher consulta `NO TOCAR/updater/config.json`. Solo hace GET de manifests y ejecutables: nunca envía datos, fotos, configuración, usuarios o backups. No lleva tokens. Si el código pasa a privado, configure un repositorio público distinto de distribución siguiendo `docs/RELEASES.md`.

## Primer uso

1. Elija el ZIP PORTABLE de su arquitectura, x64 (`windows-amd64`) o ARM64 (`windows-arm64`). Son paquetes iniciales, no actualizaciones para descomprimir encima de una instalación existente.
2. Extraiga en la carpeta local o compartida del servidor, incluidas rutas UNC o con espacios.
3. Ejecute `EJECUTAR.bat` **en el equipo servidor**. Los clientes normales solo necesitan el navegador. Al ejecutar el BAT desde otro equipo sobre la misma carpeta se reconoce la instancia existente; si no existe, ese equipo se convertiría en servidor. No se puede iniciar remotamente un proceso de Windows sin infraestructura adicional.
4. La instalación nueva crea `admin` con la contraseña inicial acordada: `Zocotroco$01`. Cambie la contraseña y el nombre visible desde **Mi cuenta**.
5. El servidor escucha en la LAN; Windows puede solicitar autorización de firewall. Consulte `docs/NETWORK_DEPLOYMENT.md` antes de habilitar acceso. No exponga el puerto a Internet.

## Comandos

- `EJECUTAR.bat`: comprobar versiones y arrancar, o abrir instancia existente.
- `EJECUTAR.bat actualizar`: comprobar el canal; pregunta antes de instalar. Si hay servidor, lo reutiliza: cierre primero para actualizar.
- `EJECUTAR.bat cerrar`: cierre ordenado desde el equipo servidor. Los usuarios LAN pueden cerrar desde Administración.
- `EJECUTAR.bat prueba`: autocomprobación del programa instalado.
- `EJECUTAR.bat version`: versiones de launcher, aplicación y esquema.
- `EJECUTAR.bat rollback`: intentar versión anterior con backup, validación y health check. No requiere borrar los datos.

El servidor también acepta `CERRAR` en su consola. La ausencia de Internet o un error del canal no impiden arrancar la versión local. La confirmación de actualización es `s`; INTRO conserva la versión actual.

## Carpetas

Todo salvo el BAT permanece bajo `NO TOCAR`: `launcher.exe`, `current/active.json`, `versions/<versión>/server.exe`, `updater/`, `logs/`, `config.json`, `data/store.json`, `data/photos/`, `data/backups/`. Se mantiene la ubicación histórica de fotos y backups dentro de `data` para no mover ni duplicar datos existentes. `current` contiene un puntero JSON, sin enlaces simbólicos o unidades fijas.

Los paquetes de actualización contienen **solo** `server.exe` y `release.json`. No contienen datos, configuración ni launcher. Nunca extraiga un paquete PORTABLE encima de una instalación existente para actualizar.

## Desarrollo y publicación

Go 1.23+ solo para desarrollo. Desde `app`: `go test ./...`, `go test -race ./...`, `go run -buildvcs=false ./cmd/control-buceadores --base /ruta/de/pruebas`. No use la carpeta real de datos para pruebas.

Para generar paquetes limpios: `bash scripts/build_windows.sh -version 8.2.0 -public-base https://github.com/adrianseguradeorta-prog/app_control_buc/releases/download/v8.2.0` (o PowerShell). El resultado se escribe en `release-output`; el compilador nunca borra ni empaqueta la instalación.

Los tests de integración del launcher ejecutan versiones reales y un canal HTTPS temporal. En Linux usan el compilador Go y datos ficticios; las comprobaciones específicas de UNC/SMB y Windows limpio requieren Windows real. Consulte `docs/VALIDATION.md` para resultados, alcance y pendientes.

## Documentación

- [Actualizaciones](docs/UPDATES.md)
- [Publicación](docs/RELEASES.md)
- [Compatibilidad](docs/DATA_COMPATIBILITY.md)
- [Seguridad](docs/SECURITY.md)
- [Despliegue LAN](docs/NETWORK_DEPLOYMENT.md)
- [Backups](docs/BACKUPS.md)
- [Rollback](docs/ROLLBACK.md)

La fuente sigue siendo la reconstrucción mantenible de la V7 revisada. No se afirma paridad visual certificada con un binario V7 sin fuente.
