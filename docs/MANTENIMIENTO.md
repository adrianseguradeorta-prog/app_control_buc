# Mantenimiento de cualificaciones

## Presentación
Una fila por buceador activo:
- Empleo
- Nombre y apellidos
- Autónomo
- SDS Aire
- SDS HELIOX

Estados: `OK`, `A RECUPERAR`, `SIN DATOS`, `FUERA DE PLAZO`, `NO ASIGNADA`.
La tabla y el detalle individual son imprimibles.

## Cómputo
Una inmersión cuenta si:
- fecha válida;
- cualificación compatible;
- profundidad máxima > 0;
- tiempo de fondo > 0;
- registro marcado como válido.

### Autónomo
3 inmersiones en los últimos 12 meses y al menos una entre 25 y 30 m.

### SDS Aire
2 inmersiones en los últimos 12 meses y suministro desde superficie con aire.

### SDS HELIOX
3 inmersiones en los últimos 12 meses: 2 con aire y 1 con mezcla a más de 40 m.

Estos criterios proceden del Excel de referencia. Si se incorporan reglas normativas nuevas, documentar fuente y migración.
