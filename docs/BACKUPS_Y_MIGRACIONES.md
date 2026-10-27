# Backups y migraciones

## Formato
ZIP con:
- `manifest.json`
- `config.json`
- `data/store.json`
- `data/photos/...`

`manifest.json` debe incluir:
- `app_id = control-buceadores-bamis`
- versión de aplicación
- `schema_version`
- fecha UTC
- motivo
- SHA-256 de cada archivo incluido

## Restauración
1. Validar ZIP, app_id y SHA-256.
2. Crear backup automático del estado actual.
3. Aplicar migraciones de esquema si son necesarias.
4. Restaurar datos/fotos/configuración.
5. Validar integridad lógica: IDs de buceador únicos, inmersiones con buceador existente y códigos IB admitidos.

## Compatibilidad
Nunca cambiar silenciosamente el significado de un campo. Si cambia el esquema, incrementar `schema_version` y añadir una migración + test.
