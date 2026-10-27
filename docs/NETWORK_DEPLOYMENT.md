# Servidor y clientes LAN

Copie la carpeta al disco del servidor, una unidad asignada o `\\SERVIDOR\Aplicaciones\Control Buceadores`. Ejecute el BAT desde el equipo que alojará el proceso. Los clientes abren `http://nombre-servidor:puerto` o su IP. No necesitan copiar ni ejecutar el programa.

El BAT usa `pushd`/`popd` y rutas entre comillas, sin rutas absolutas ni dependencia de una letra de unidad. El launcher obtiene su propia carpeta. Las unidades asignadas son específicas del usuario de Windows; use una UNC accesible si distintos usuarios no comparten asignaciones.

Por defecto el servidor escucha en `0.0.0.0:8765`; si otro programa ocupa el puerto, reserva otro entre los siguientes 100. El puerto efectivo y host se registran localmente. Para solo uso local configure `bind_address` como `127.0.0.1`. Puede definir `preferred_port` y `allowed_hosts` en `NO TOCAR/config.json`; se crea automáticamente al primer arranque. IPs locales y hostname se permiten automáticamente. Reinicie tras cambiar la red o configuración.

No se instala un servicio Windows ni se exige elevación del programa. La política corporativa de firewall/SMB puede requerir intervención del administrador de red. No se abre el firewall automáticamente.

Un bloqueo de fichero exclusivo de Windows (`LockFileEx`, no PID como autoridad) protege la carpeta compartida. En Unix se usa flock. Los archivos lock pueden permanecer tras cerrar: el sistema libera el bloqueo al terminar o morir el proceso; un archivo antiguo sin bloqueo no impide arrancar. Se mantienen bloqueos de despliegue y servidor. Un segundo launcher no migra ni actualiza datos activos, y abre/muestra la instancia existente. Durante un arranque sin respuesta espera un plazo acotado y avisa en lugar de iniciar un segundo escritor.

En SMB se requiere un servidor que respete bloqueos de rango de Windows. No se certifican NAS con semántica de bloqueo incompatible ni particiones de red. Valide el recurso real antes del despliegue. No arranque copias independientes de la carpeta que luego sincronice: deben compartir exactamente los mismos ficheros de bloqueo y datos.
