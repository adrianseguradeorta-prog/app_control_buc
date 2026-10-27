# Revisión de alerta antivirus — 8.2.0

## Estado y evidencia

Se ha notificado una alerta de **Cortex XDR** que bloquea `launcher.exe`: «Detectado ejecutable sospechoso», con publicador «Desconocido». No se dispone todavía del identificador de regla/incidente ni de un dictamen del equipo que gestiona esa protección. La captura no se publica: no es necesaria para distribuir los hashes y contiene información del entorno del usuario.

Los ejecutables de esta versión **no tienen firma Authenticode**. Este hecho puede ser relevante para políticas de confianza y reputación, pero no demuestra la causa exacta de la detección ni permite declararla falso positivo. Tampoco las pruebas funcionales o un SHA-256 correcto certifican ausencia de malware.

Los hashes se han calculado sobre los ZIP publicados, descargados de nuevo después de su compilación en GitHub Actions. Corresponden a esos archivos exactos; no se han recompilado para cambiar su huella como respuesta a la alerta.

| Arquitectura | Archivo | SHA-256 |
|---|---|---|
| amd64 | CONTROL_BUCEADORES_PORTABLE_windows-amd64.zip | e3c3071b99ed810272a4d3e03f1ce027e4a0ea3da0906f108bc81e961df0ed6c |
| amd64 | launcher.exe | 4df9733e1dcc35820e200368125c52ffe16dfcb2934a5136f7b07316eca61c5d |
| amd64 | server.exe | 18c8cfdbc1016e0f602def4a1eb0c6ac19fc0ed3083f0cd6ffb014cb9571ccbf |
| arm64 | CONTROL_BUCEADORES_PORTABLE_windows-arm64.zip | d7562a0942ef9639be023a4ac68cede24ea9729806e30a274df8431d94dfbf64 |
| arm64 | launcher.exe | f3d7b48f4d116d6a8415b716de32619fc619c3a884f77ace519f80f53573e765 |
| arm64 | server.exe | 99457c0119f4326bc3861b0e8cb18baec4e70fa4584f7aad8a9bd4f28932b151 |

## Procedencia verificable

- Código: https://github.com/adrianseguradeorta-prog/app_control_buc/tree/v8.2.0
- Compilación y pruebas de distribución: https://github.com/adrianseguradeorta-prog/app_control_buc/actions/runs/40486953908
- Script de empaquetado: `app/cmd/package/main.go`.
- Compilación Go 1.23.x para Windows x64/ARM64 con CGO desactivado, sin UPX, ofuscadores ni instalador. Se utilizan las opciones habituales `-trimpath` y `-s -w`; la aplicación se compila con su versión explícita.
- Paquetes de programa compuestos por `server.exe` y `release.json`; el paquete inicial incluye además launcher, BAT y configuración sin datos operativos.

## Comportamiento esperado para revisión corporativa

`launcher.exe` consulta mediante HTTPS/GET un manifest de GitHub Releases configurable fuera del binario y, previa confirmación del usuario, descarga una versión compilada. Verifica su SHA-256, metadatos y arquitectura. Mantiene las versiones en carpetas independientes, crea un backup local antes de actualizar los datos y arranca `server.exe` como proceso hijo. No hay subida ni sincronización de usuarios, inmersiones, fotografías o backups.

El launcher puede abrir la URL de la instancia existente con el manejador de navegador de Windows mediante `rundll32 url.dll,FileProtocolHandler`. El servidor también abre su navegador y escucha por defecto en la LAN en el puerto 8765 o uno de los siguientes 100, según disponibilidad. El servidor ofrece endpoints internos de control limitados a loopback y protegidos por token.

Los datos se guardan en la carpeta local o compartida elegida por el administrador, fuera de las carpetas de programa. No se instala un servicio, driver, tarea programada ni inicio automático. No se solicitan permisos de administrador para ejecutar la aplicación. No hay PAT ni credenciales de GitHub incorporados en los binarios.

Estas capacidades pueden interesar al analista de seguridad; no se presupone cuál de ellas produjo la alerta.

## Próximo paso requerido

El equipo que administra Cortex debe revisar el ejecutable exacto y el evento, contrastar su hash y procedencia, y emitir un dictamen antes de autorizarlo en equipos gestionados. Es útil aportar el nombre de regla, ID de incidente, hash y ruta que muestra «Mostrar detalles», sin compartir datos operativos innecesarios.

No se recomienda desactivar Cortex, crear exclusiones amplias, renombrar el ejecutable para evitar el bloqueo ni ejecutar directamente otro componente como vía alternativa. Si la organización exige firma de código, se necesita su proceso autorizado y un certificado de firma confiable; no se dispone de él en este proyecto. Una firma legítima tampoco garantiza por sí sola que Cortex lo autorice.

Se ha preparado un workflow separado (`antivirus.yml`) que verifica hashes y analiza los paquetes publicados con Microsoft Defender en Windows, sin exclusiones ni desactivar TLS. Su resultado se debe consultar en Actions: no sustituye el dictamen de Cortex ni acredita la política del equipo afectado.
