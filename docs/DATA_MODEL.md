# Modelo de datos

Persistencia principal: `NO TOCAR/data/store.json`.

## Store
- `schema_version`
- `next_dive_id`
- `users[]`
- `divers[]`
- `dives[]`
- `report_templates[]`
- `default_report_template_id`

## User
- id
- username
- display_name
- password_salt
- password_hash
- hash_iterations
- status: pending / approved / rejected
- is_admin
- created_at / approved_at

## Diver
- id
- rank
- name
- entry_date
- qual_basic
- qual_sds_air
- qual_sds_heliox
- active
- notes
- photo (opcional)

## Dive
- id
- source_record (opcional, para trazabilidad de importación)
- diver_id
- date
- qualification
- breathing_mode
- gas
- surface_supply
- max_depth
- bottom_time
- dive_type
- ib_code (derivado; nunca autoridad de entrada)
- purpose
- notes
- valid
- created_at

## ReportTemplate
Campos textuales de cabecera/columnas/firmas, fuente, tamaños y espaciado. Usar variables de plantilla en textos dependientes del mes/año/unidad.
