# Publicar versiones

El repositorio proporcionado es público y puede servir código y Releases sin credenciales en clientes. No se ha cambiado su visibilidad. El workflow prueba Linux y Windows en pushes a main; solo publica cuando se crea un tag estable `vX.Y.Z`. No use el mismo número para binarios distintos.

1. Revise los cambios, tests y fixture sintética. Nunca añada la carpeta de instalación ni documentos operativos.
2. Cree y suba un tag, por ejemplo `git tag v8.2.1` y `git push origin v8.2.1` desde el entorno de desarrollo (los usuarios finales no usan Git).
3. Actions ejecuta los tests antes de compilar Windows x64/ARM64 estáticos con `CGO_ENABLED=0`. Construye ZIPs desde una lista explícita de archivos, calcula hashes y manifest y crea la Release.
4. Revise los paquetes y permisos del canal. `releases/latest/download/update.json` solo encuentra Releases publicadas, no borradores.

Para publicar en otro repositorio defina la variable de Actions `DISTRIBUTION_REPOSITORY=owner/repo` y el secreto `DISTRIBUTION_TOKEN` en el entorno de GitHub Actions llamado `distribution`, con permisos mínimos de escritura de Releases en ese repositorio. El secreto permanece exclusivamente en Actions; no se copia al programa ni a la configuración. En el mismo repositorio se utiliza `GITHUB_TOKEN` con `contents:write` solo en el job de publicación. Es posible proteger el entorno distribution con revisión antes de publicar.

Si el código se hace privado, cree primero el canal público independiente y cambie tanto esa variable como la URL de manifest distribuida. El código privado no debe publicarse por automatismos de esta tarea. El canal público solo necesita las Releases, no checkout del código ni ficheros de datos.
