# Cómo contribuir

Gracias por tu interés en ai-dev. Este documento resume cómo se trabaja en el proyecto. Para vulnerabilidades, sigue [SECURITY.md](SECURITY.md) y no abras un issue público.

## Proceso

El proyecto sigue **Spec Kit** y su [constitución](.specify/memory/constitution.md). Cada cambio de comportamiento parte de una especificación:

1. `spec.md`, aprobada por el responsable;
2. `plan.md` y, si hace falta, `contracts/` y `data-model.md`;
3. `tasks.md`, con las pruebas antes de la implementación;
4. la implementación, con cada tarea marcada `[X]` al terminar;
5. una revisión independiente antes del pull request.

Las especificaciones viven en `specs/<NNN>-<nombre>/`. Un cambio pequeño que no altera el comportamiento, como documentación o CI, no necesita una feature nueva.

Un agente de IA nunca registra ni afirma una aprobación humana en nombre de una persona (Constitución IX). Las decisiones del responsable se anotan con su nombre y sus palabras, como afirmación declarada y no autenticada.

## Ramas y pull requests

Se usa Git Flow:
- `dev` es la rama de integración y `main` la de releases;
- las ramas de trabajo salen de `dev` con el prefijo `feature/` o `bugfix/`;
- los pull requests apuntan a `dev` y se integran con merge commit;
- `dev` y `main` están protegidas: exigen un pull request, los checks `gates (macos-latest)`, `gates (ubuntu-latest)` y `git-minimum` en verde, y las conversaciones resueltas. No se permite force push.

Los mensajes de commit siguen [Conventional Commits](https://www.conventionalcommits.org/), por ejemplo `feat(cli): add version and help commands`.

## Requisitos

- Go, en la versión que declara `go.mod`;
- Git 2.45 o posterior;
- macOS o Linux. Las pruebas que aplican cambios reales con Git necesitan un directorio temporal en APFS (macOS) o ext4 local (Linux); en tmpfs se saltan.

## Antes de abrir un pull request

```sh
gofmt -l .                               # no debe listar nada
go vet ./...
go test ./...
go test -race -timeout 60m ./internal/gitstore ./internal/gitmanager ./internal/app ./cmd/ai-dev
```

El CI ejecuta además cross-builds, fuzzing y las suites con Git real contra Git 2.45.0 (ver [`.github/workflows/ci.yml`](.github/workflows/ci.yml)).

Criterios de revisión:
- solo la biblioteca estándar de Go; una dependencia nueva necesita justificación;
- cada requisito funcional tiene su prueba;
- no se cambian APIs públicas ni textos de uso existentes salvo que la especificación lo pida;
- fallo cerrado: ante la duda, un refusal tipado y nunca un éxito supuesto;
- nada de secretos, credenciales ni datos personales en el código, las pruebas o los registros.

## Licencia

Al contribuir aceptas que tu contribución se distribuya bajo la [Apache License 2.0](LICENSE), según su sección 5.
