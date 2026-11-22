# Política de seguridad

[English](SECURITY.md) · **Español**

## Versiones soportadas

Solo se corrigen vulnerabilidades en el último release publicado (hoy la serie 0.1.x) y en la rama `dev`, que es la línea de desarrollo activa. Mientras la versión mayor sea 0, una corrección llega en un release nuevo y no se publica en versiones anteriores.

## Cómo reportar una vulnerabilidad

**No abras un issue público ni un pull request para reportar una vulnerabilidad.**

Repórtala de forma privada con el formulario **Report a vulnerability** de la pestaña [Security](https://github.com/germanmunozs/ai-dev/security) del repositorio (GitHub private vulnerability reporting).

Incluye, si puedes:
- la versión afectada (`ai-dev version --format json`), el sistema operativo y el filesystem;
- los pasos mínimos para reproducirla;
- el impacto que observaste o que esperas.

No incluyas credenciales, tokens ni datos personales reales en el reporte.

El responsable confirmará la recepción y coordinará contigo la corrección y la fecha de divulgación. Es un proyecto mantenido por una persona, así que no hay plazos de respuesta garantizados.

## Alcance

Interesan especialmente los fallos de las garantías descritas en el [modelo de seguridad](README.es.md#modelo-de-seguridad):
- una señal automática que conceda autoridad, apruebe o mute sin una aprobación humana explícita;
- una mutación de Git sin el plan confirmado y su autorización emitida (`MutationPermit`);
- la ejecución de hooks, de un shell o de acceso a la red desde las invocaciones de Git;
- credenciales o secretos que aparezcan en la salida o en los registros duraderos;
- rutas que sigan enlaces simbólicos, o escrituras fuera del proyecto o del almacén de estado;
- informes de texto que, pegados en un shell, ejecuten o expandan algo;
- un éxito parcial reportado como éxito, en lugar de un refusal tipado;
- en el cockpit (`ai-dev tui`), una secuencia de escape, OSC o control bidireccional que llegue a la terminal sin escapar, o una acción que se ejecute sin la revisión y la confirmación explícita;
- una vulnerabilidad en una dependencia de terceros (Bubble Tea, Lip Gloss y sus módulos) que sea alcanzable desde ai-dev.

Quedan fuera de alcance las [limitaciones conocidas](README.es.md#limitaciones-conocidas) y los riesgos residuales aceptados que documentan las especificaciones (por ejemplo REC-06, REC-07 y DUR-01 en [`specs/008-git-management`](specs/008-git-management/)).

## Dependencias de terceros

Las dependencias se fijan por versión y checksum en `go.mod` y `go.sum`. Se elige la versión más reciente con al menos 14 días de publicada, y govulncheck se ejecuta en cada CI. La política completa está en el [README](README.es.md#dependencias-de-terceros).
