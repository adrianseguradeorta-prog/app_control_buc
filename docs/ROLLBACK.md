# Recuperación y rollback

`EJECUTAR.bat rollback`, con servidor cerrado, valida el programa anterior contra los datos actuales y crea backup antes de cualquier cambio. La versión anterior debe soportar el esquema actual; no se elimina la carpeta actual ni se realiza una degradación de datos sin migración explícita.

Si falla validación, arranque o health check durante una actualización, el launcher conserva el puntero activo anterior, recupera el backup cuando corresponde y arranca el programa anterior. Si no puede recuperar los archivos (por ejemplo, recurso SMB desconectado), se detiene e informa: no oculta una recuperación fallida ni insiste en escribir.

Las versiones fallidas se conservan para diagnóstico y no se sobrescriben con el mismo número. Para reintentar use una versión nueva o retire exclusivamente la carpeta fallida bajo versions con servidor cerrado y habiendo comprobado que no es actual/anterior. Nunca borre data para resolver una actualización.

Un corte brusco de alimentación o desconexión SMB entre operaciones no equivale a un error controlado. El puntero usa temporal+sync+rename y los backups mantienen los datos recuperables, pero debe comprobarse la atomicidad del recurso compartido real. No se promete funcionamiento durante una partición de red ni recuperación automática de cualquier fallo físico.
