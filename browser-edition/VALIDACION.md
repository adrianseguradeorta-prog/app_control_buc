# Validación de la edición navegador 9.0.0

- Windows/Chromium en GitHub Actions: diez pruebas terminadas correctamente, sin omisiones. Incluyen reglas y PBKDF2, CRUD y fotos, usuarios, cambios de cuenta, informes/formatos, importación XLSX comprimida repetida, backups comprimidos y corruptos, recuperación legacy, conflicto de turno, cambio externo de archivo, fallo de escritura con recuperación y rechazo de un JSON dañado.
- Caso real `file://`: prueba el ZIP empaquetado, descomprimido en una ruta con espacios; carga HTML, CSS y JavaScript, comprueba el contexto seguro y llama al selector nativo desde una pulsación real, sin seleccionar ni escribir en carpetas reales. Una cancelación o diálogo pendiente es normal en la prueba; un rechazo del origen por SecurityError hace fallar el caso. No se desactiva la política de contenido.
- Ejecución más reciente de esa comprobación: https://github.com/adrianseguradeorta-prog/app_control_buc/actions/runs/40496822199
- Compilación del ZIP y publicación: https://github.com/adrianseguradeorta-prog/app_control_buc/actions/runs/40496179259
- Las pruebas funcionales usan métodos reales FileSystemDirectoryHandle/createWritable sobre OPFS aislado y sustituyen únicamente el selector del sistema por ese directorio de pruebas. No representan un recurso SMB real.
- Linux del entorno de desarrollo: nueve pruebas aprobadas y una omitida porque la política del Chromium administrado bloquea URLs file://. La prueba omitida sí se ejecutó correctamente en Windows.
- Comprobación independiente con Go: un backup creado por el navegador se validó y restauró con core.ValidateBackup/RestoreBackup; las contraseñas PBKDF2 se verificaron con Go y el store después de restaurar fotos pasó core.ValidateStore.
- El Excel original se leyó únicamente en una comprobación local aislada: 11 buceadores y 15 inmersiones; no se modificó ni se incluyó en el ZIP o en el repositorio. Los tests publicados usan solo fixtures ficticias.
- Las pruebas Go existentes siguen aprobadas. No se modificó su lógica de aplicación.

## Auditoría de la entrega

La descarga publicada contiene seis archivos estáticos: HTML, tres JavaScript, CSS e instrucciones. No hay ejecutables, DLL, BAT, scripts de consola, datos, fotos, credenciales operativas ni backups. SHA-256 del ZIP publicado:

`768a16da8e85bf0f0f062eb798690c1d20139693b9ce9b7036b4f6ca37fc9139`

El programa no hace peticiones de red y su política de contenido bloquea conexiones. Los tests rechazan salidas a destinos externos.

## Pendientes de aceptación en destino

No se ha probado la política de vuestro Edge/Chrome, la aceptación de Cortex para estos archivos HTML/JS, la selección/permisos de vuestra carpeta SMB ni las garantías físicas del NAS/servidor de archivos. La edición informa de permisos denegados y no altera protecciones. Los turnos son cooperativos: no se promete exclusión distribuida atómica ni protección de roles equivalente a un servidor. Probar primero con una copia y conservar copias externas de respaldo.
