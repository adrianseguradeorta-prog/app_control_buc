# Edición navegador · 9.0.0-browser

Edición separada autorizada para **trabajo por turnos** desde una carpeta compartida, sin servidor, ejecutables, BAT, instalador, runtime adicional ni descarga de programas. Se abre `ABRIR.html` con Edge o Chrome actualizado. No se reutiliza un ejecutable bloqueado ni se cambian protecciones: la edición contiene únicamente HTML, JavaScript, CSS y texto.

El usuario selecciona cada sesión una carpeta concreta y concede lectura/escritura con File System Access API. Puede seleccionar el `NO TOCAR` de una copia de una instalación anterior o el `NO TOCAR` vacío del nuevo paquete. Las políticas corporativas pueden impedirlo; la aplicación lo informa y no cambia políticas ni promete aceptación por Cortex.

## Funciones

- Cuentas: registro pendiente, aprobación y roles de interfaz, cambio de nombre y contraseña, recuperación explícita de admin legacy con backup previo y restablecimiento de otras cuentas por administrador.
- Buceadores e inmersiones: alta, edición, eliminación administrada, fotos opcionales, validaciones y Dato IB derivado del tipo (11 opciones).
- Mantenimiento con una fila por buceador activo y las tres cualificaciones; filtros de fecha y requisitos conservados.
- Informes: relación mensual, histórico completo, ficha individual con/sin foto, mantenimiento e impresión del navegador. Cabeceras y firmas editables, estilos y nuevos formatos reutilizables con variables; filas de inmersiones no editables.
- Importación del XLSX documentado, con deduplicación y rechazo de colisiones de IDs.
- Backups ZIP con manifest y SHA-256, creados antes de cada guardado; restauración compatible con esquemas 3 y 4. Fotos restauradas con nombres nuevos, sin borrar las anteriores. Límite de ZIP: 128 MB.
- Aviso de turno con renovación y caducidad de diez minutos, comparación de hash antes de guardar y registro de recuperación tras guardado incompleto.
- Sin peticiones de red desde la aplicación; CSP `connect-src 'none'`, sin librerías, fuentes, analytics, servicios externos ni actualización automática. Los informes y backups quedan locales.

## Límites aceptados

El aviso de turno es cooperativo, **no un bloqueo distribuido atómico** sobre SMB. La comprobación de cambios reduce sobrescrituras, pero no resuelve todas las carreras entre dos equipos que escriban a la vez. No abrir la edición Go ni dos sesiones de edición simultáneamente. Si alguien abandona la pestaña sin terminar turno, esperar a que venza y confirmar con esa persona que está cerrada antes de recuperarlo.

Las cuentas organizan la interfaz; no son una frontera de seguridad contra alguien con acceso a los archivos, al código JavaScript o a herramientas de desarrollo. Los permisos efectivos de datos dependen de Windows/SMB. No se oculta esta diferencia respecto al servidor.

El navegador usa `createWritable` y cierre para confirmar escrituras y mantiene copias previas. No se certifica atomicidad frente a cortes físicos o todos los NAS. Un registro de recuperación permite reconocer un guardado completo por hashes o recuperar explícitamente la copia previa. No se reinitializa un JSON dañado. Se conservan los campos desconocidos al editar objetos existentes.

No se cambia el esquema de datos a 9; la versión del programa es independiente. JSON 3/4, fotos y backups siguen en `data`. El ZIP de distribución no contiene ninguno de esos datos ni documentos de referencia originales. Actualizar consiste en reemplazar únicamente HTML/JS/CSS, con el turno cerrado y copia previa de los datos; nunca sobrescribir `data`.

## Pruebas de desarrollo

Python 3 y Playwright solo para desarrollo/CI (no se entregan al usuario). `python browser-edition/tests/test_browser.py` usa Chromium y almacenamiento privado aislado OPFS para probar métodos nativos de directorio/escritura; se sustituye únicamente el selector de carpeta del sistema. Esto prueba la aplicación, no un recurso SMB real ni el diálogo de permisos de vuestro equipo.

El caso directo `file://` se ejecuta sin esa sustitución y comprueba carga, recursos y criptografía. Una política del Chromium del entorno Linux bloquea URLs de archivos, de modo que ese caso se registra como omitido localmente; no se permite omitirlo por esa razón en Windows CI. El resto de pruebas comprueba reglas/hash PBKDF2, CRUD/fotos, IB automático, informes/formatos, usuarios, turnos/conflictos, Excel duplicado, backups corruptos, recuperación legacy, fallos de escritura y JSON dañado.

`python scripts/package_browser.py` empaqueta una allowlist explícita, comprueba integridad ZIP y genera SHA-256. La entrega conserva la estructura `CONTROL_BUCEADORES_NAVEGADOR/ABRIR.html` y `NO TOCAR/assets`. La edición Go se mantiene sin sustituirla.
