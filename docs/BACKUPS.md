# Backups locales

Se conservan bajo `NO TOCAR/data/backups`. El backup preupdate incluye bytes originales de store, configuración y fotos, manifest con motivo, fecha, aplicación, esquema y hashes SHA-256. Nunca se envía a GitHub. Si falla el backup no se actualizan los datos.

Desde Administración puede descargar o restaurar un backup. Restaurar valida hashes, esquema y referencias y genera copia previa del estado actual; invalida sesiones. Mantenga además una copia externa controlada en su red: un backup en el mismo disco no protege de perder ese disco.

No haga pruebas sobre la instalación real. Las pruebas usan directorios temporales y fixtures sintéticas.
