# Actualizaciones

Launcher 1.0.0; aplicación 8.2.0; esquema 4. Son versiones independientes.

Configuración externa `NO TOCAR/updater/config.json`:

```json
{"manifest_url":"https://github.com/adrianseguradeorta-prog/app_control_buc/releases/latest/download/update.json","timeout_seconds":5,"channel":"stable","owner":"adrianseguradeorta-prog","repository":"app_control_buc"}
```

Se usa `manifest_url` como origen efectivo; owner/repository/channel describen el canal. Para cambiarlo, edite la URL HTTPS en ese archivo con el servidor detenido. No incluya credenciales en URL ni archivo. El cliente tiene timeout total, límite de descarga y redirecciones exclusivamente HTTPS. Sin canal accesible se usa la copia local.

El manifest incluye `manifest_version:1`, `application:"control-buceadores"`, `version` semver estable, `schema_version`, `minimum_launcher_version`, `published_at`, `notes` y `architectures`. Cada clave `windows-amd64`/`windows-arm64` contiene `file` (URL HTTPS completa) y `sha256` (64 caracteres hexadecimales). El ZIP incluye un ejecutable PE de arquitectura comprobada y `release.json` con version/architecture/schema_version coincidentes.

Se descarga en `updater`, se comprueban tamaño, SHA y contenido, se prepara una carpeta nueva en `versions`. No se sustituye ninguna carpeta activa. Antes de validar/migrar/arrancar se crea backup local. Solo después de validar el candidato y su health check se escribe atómicamente `current/active.json`. Se conservan la versión anterior y su carpeta. Una descarga rechazada se elimina; una carpeta de versión ya existente no se sobrescribe.

Una actualización incompatible con el launcher pide renovar este manualmente, con el servidor detenido. No existe autoactualización insegura del launcher en ejecución. El canal es de confianza: SHA-256 sobre HTTPS verifica integridad, no sustituye la seguridad de la cuenta de publicación.

La versión candidata devuelve 503 a formularios y pantallas durante la comprobación inicial; solo habilita usuarios después del health check y del cambio atómico de versión activa. Así una recuperación no descarta escrituras nuevas. Si se interrumpe el launcher y queda un servidor en preparación, ciérrelo con `EJECUTAR.bat cerrar` y reinicie; no borre los locks ni los datos.

Para un canal HTTPS corporativo con CA propia puede indicarse opcionalmente `ca_file` en la configuración externa. Se añade ese certificado a las raíces del sistema y se mantiene la verificación TLS; un archivo inválido impide consultar el canal, con arranque local como alternativa.
