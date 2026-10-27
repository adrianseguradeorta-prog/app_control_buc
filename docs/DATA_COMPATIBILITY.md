# Datos y migraciones

Se conserva JSON y el esquema 4. Los registros se escriben bajo mutex, primero en un temporal, con sync y renombrado. Las lecturas no exponen slices mutables. Se conservan campos JSON desconocidos al guardar, también dentro de registros identificados por ID. Los registros eliminados permanecen eliminados.

La única migración autorizada e implementada es 3 -> 4, que cambia el marcador de esquema manteniendo campos desconocidos. Es explícita, validada, idempotente y atómica; el launcher crea backup antes. Los hashes legacy se conservan: no se adivina su algoritmo ni se cambian contraseñas automáticamente. La recuperación administrativa existente sigue disponible con `server.exe --base "ruta NO TOCAR" --reset-user admin --password-file "archivo local protegido"`, con servidor detenido.

No hay migrador genérico para futuros esquemas. Un manifest de otro esquema se rechaza y se arranca la versión local. Una futura versión deberá incorporar migraciones específicas y sus pruebas antes de anunciar ese esquema. Rollback requiere que el programa anterior valide el esquema actual; no se degrada arbitrariamente el contenido.

El directorio histórico `data/photos` y `data/backups` no se mueve al introducir el launcher. Mantenerlo evita rutas rotas y respalda tanto instalaciones anteriores como nuevas.
