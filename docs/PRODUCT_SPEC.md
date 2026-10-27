# Especificación funcional consolidada

## Aplicación
Aplicación web local para Windows, servida por un ejecutable autocontenido. La distribución visible contiene `EJECUTAR.bat` y la carpeta `NO TOCAR`.

## Funciones
1. Registro de usuarios y aprobación por administrador.
2. Usuario inicial `admin / Zocotroco$01` en instalaciones nuevas.
3. Maestro de buceadores con empleo, nombre, cualificaciones, estado, observaciones y foto opcional.
4. Registro de inmersiones con código Dato IB calculado automáticamente.
5. Edición y eliminación administrada de buceadores e inmersiones.
6. Mantenimiento de cualificaciones con resumen de una fila por buceador.
7. Informes imprimibles: mensual, todas las inmersiones, individual y mantenimiento.
8. Plantillas de informe mensual modificables y guardables como formatos nuevos.
9. Backups ZIP versionados y restaurables en versiones futuras.
10. Selección automática de puerto libre.
11. Cierre desde terminal actual, terminal externa o web.

## Fuente de datos de referencia
Los datos de buceadores e inmersiones de prueba proceden exclusivamente de `CONTROL BUCEADORES.xlsx`.
No precargar datos del Word histórico.
