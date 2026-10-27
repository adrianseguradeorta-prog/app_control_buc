# AGENTS.md — Control de Buceadores BAM-IS

Este archivo contiene reglas obligatorias para cualquier agente Codex que modifique el proyecto.

## 1. Objetivo
Mantener y evolucionar una aplicación web portátil para control de buceadores e inmersiones del BAM-IS. El usuario final debe poder ejecutarla en Windows sin instalar Python, Node, Java, .NET, bases de datos ni paquetes externos.

## 2. Tecnología obligatoria
- Go 1.23 o superior para desarrollo/compilación.
- Ejecutable estático Windows x64 y Windows ARM64 (`CGO_ENABLED=0`).
- Biblioteca estándar de Go salvo autorización expresa del usuario.
- Aplicación web servida desde localhost.
- Persistencia en JSON versionado dentro de `NO TOCAR/data`.

## 3. Estructura de distribución que NO debe romperse
```text
CONTROL_BUCEADORES/
├── EJECUTAR.bat
└── NO TOCAR/
    ├── CONTROL_BUCEADORES.exe
    ├── CONTROL_BUCEADORES_ARM64.exe
    ├── config.json
    └── data/
        ├── store.json
        ├── photos/
        └── backups/
```
`EJECUTAR.bat` es el único punto de entrada visible para el usuario.

## 4. Credenciales iniciales
En una instalación nueva:
- usuario: `admin`
- contraseña: `Zocotroco$01`
La contraseña debe almacenarse con salt y derivación de clave; nunca en texto claro.

## 5. Usuarios
- El registro público crea usuarios en estado `pending`.
- Un administrador debe aprobarlos antes de que puedan entrar.
- `admin` no puede quedar sin privilegios de administración ni rechazado.
- Cualquier formulario que modifica datos debe llevar protección CSRF.

## 6. Dato IB — regla crítica
El usuario selecciona el tipo de inmersión. El código se calcula AUTOMÁTICAMENTE en cliente y servidor; jamás se acepta como autoridad un código enviado por el navegador.

Tabla admitida:
- 61001 Autónomo Aire
- 62001 SDS Aire
- 62002 SDFS Heliox
- 62003 Autónomo Nitrox
- 62004 Autónomo Heliox/Trimix
- 62005 Semicerrado Nitrox (Buceo técnico)
- 62006 Semicerrado Trimix
- 63001 Cerrado Oxígeno
- 64001 Semicerrado Nitrox (Caza de minas)
- 66001 Control en Cámara
- 66002 Tratamiento O2

No reintroducir 62007, 63002 ni 65001 salvo instrucción expresa y fuente normativa nueva.

## 7. Mantenimiento de cualificaciones
La pantalla resumen debe mostrar EXACTAMENTE una fila por buceador activo y estas columnas de estado:
- Autónomo
- SDS Aire
- SDS HELIOX

Estados:
- OK
- A RECUPERAR
- SIN DATOS
- FUERA DE PLAZO
- NO ASIGNADA

Criterios actuales, derivados del Excel de referencia:
- Una inmersión computa si tiene fecha, cualificación, profundidad > 0, tiempo de fondo > 0 y `valid=true`.
- Autónomo: 3 inmersiones en 12 meses y al menos una entre 25 y 30 m.
- SDS Aire: 2 inmersiones en 12 meses y suministro desde superficie con aire.
- SDS HELIOX: 3 inmersiones en 12 meses, 2 con aire y 1 con mezcla a más de 40 m.
No modificar estos criterios sin una instrucción explícita o una fuente normativa que lo justifique.

## 8. Eliminaciones
- Solo administradores.
- Antes de cualquier eliminación definitiva de buceador o inmersión, crear backup automático.
- Si se elimina un buceador con inmersiones, la interfaz debe advertir claramente el efecto.

## 9. Informes
### Relación mensual
Formato base: `reference/source_materials/formato_relacion_mensual.png`.
Columnas del registro: NOMBRE, FECHA, EQUIPO, MEZCLA, CODIGO.
La tabla de inmersiones NO debe ser editable desde la hoja imprimible.
Cabeceras, textos, firmas y formato visual sí pueden editarse antes de imprimir.
Un administrador puede guardar esos cambios como un formato nuevo reutilizable.
Las plantillas deben usar variables (`{{MES}}`, `{{AÑO}}`, `{{UNIDAD}}`, `{{LOCALIDAD}}`, `{{FECHA_FIN_MES}}`) y no fijar un mes concreto.

### Otros informes
- Histórico completo de todas las inmersiones.
- Registro individual por buceador.
- Registro individual imprimible con foto o sin foto.
- Control de mantenimiento imprimible.

## 10. Fotografías
Son opcionales. La ausencia de fotografía jamás debe impedir registrar, consultar, respaldar ni imprimir un buceador.

## 11. Servidor
- Antes de arrancar, comprobar que el puerto preferido está libre.
- Si está ocupado, buscar automáticamente otro puerto disponible.
- Debe poder cerrarse de tres formas:
  1. escribiendo `CERRAR` en la terminal que ejecuta el servidor;
  2. `EJECUTAR.bat cerrar` desde otra terminal;
  3. desde Administración en la aplicación web.
- La ruta interna de cierre debe estar protegida por token aleatorio y limitarse a localhost.

## 12. Backups y migraciones
- Backup ZIP con `manifest.json`, `schema_version`, versión de aplicación y SHA-256 por archivo.
- Debe contener al menos `data/store.json`, `config.json` y fotos.
- Antes de restaurar, crear backup automático del estado actual.
- Nunca romper la capacidad de importar backups de versiones anteriores sin incluir una migración explícita.
- Al cambiar el modelo de datos, incrementar `schema_version` y añadir prueba de migración.

## 13. Fuentes y datos
- Los datos precargados de referencia deben proceder del Excel, NO del documento Word.
- `app/testdata/store_from_excel.json` contiene una fixture con los 11 buceadores y 15 inmersiones del Excel para pruebas.
- El documento Word se conserva solo como referencia histórica de un formato anterior; NO usarlo para precargar personal ni inmersiones.

## 14. Calidad mínima antes de dar una tarea por terminada
Ejecutar siempre:
```bash
cd app
go test ./...
```
Y, si se modifica comportamiento de servidor o UI:
- probar alta/edición de buceador;
- probar alta/edición/eliminación de inmersión;
- comprobar Dato IB automático en servidor;
- comprobar mantenimiento;
- comprobar informe mensual y tabla no editable;
- comprobar backup/restore;
- comprobar puerto alternativo;
- comprobar cierre web y cierre externo.

## 15. Seguridad de datos durante pruebas
- Nunca ejecutar tests contra `NO TOCAR/data` de una instalación real.
- Usar `t.TempDir()` o `app/testdata`.
- No sobrescribir el Excel ni los documentos de `reference/source_materials`.

## Distribución 8.2 y posteriores — reglas que sustituyen las rutas anteriores

La nueva distribución tiene launcher portátil, `current/active.json` y versiones independientes bajo `NO TOCAR/versions`. El servidor se ejecuta una sola vez para clientes LAN. Se conservan datos, fotos y backups en sus rutas históricas bajo `NO TOCAR/data`. No empaquetar instalaciones ni copiar documentos originales a GitHub. En este checkout todas las fixtures son sintéticas; las fuentes operativas originales están fuera del repositorio y no son datos de pruebas publicables.

Releases por allowlist, HTTPS y SHA-256 obligatorios, confirmación de actualización y fallback offline. No incrustar credenciales. Probar bloqueo y concurrencia, migraciones, actualización real simulada y rollback. Ninguna migración futura sin prueba y backup. No afirmar pruebas nativas Windows/SMB si no se ejecutaron. El usuario final no instala runtimes.

## Edición navegador autorizada para trabajo por turnos

La instrucción posterior del usuario autoriza una edición HTML/JavaScript independiente en `browser-edition/`, sin servidor ni ejecutables, para uso cooperativo por turnos. Esta excepción sustituye el requisito Go/servidor/BAT únicamente para esa edición; no obliga a reescribir ni eliminar la edición Go existente. Mantener JSON 3/4, backup previo, compatibilidad y privacidad. No prometer bloqueo distribuido fiable ni seguridad de roles equivalente a servidor. No modificar políticas del navegador/antivirus ni crear vías para ejecutar el binario bloqueado. No empaquetar datos de instalación. Validar flujos en navegador y apertura file:// en Windows; documentar por separado cualquier comprobación no ejecutada sobre SMB real.
