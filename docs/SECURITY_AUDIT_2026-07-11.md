# Revisión de seguridad y diseño — Sorodeal

Fecha de inicio: 2026-07-11  
Cierre de validación: 2026-07-12  
Alcance: contrato Soroban, SDK Go/TypeScript, playground, Cloud API/console y
dependencias directas del workspace.  
Naturaleza: revisión interna asistida; no sustituye una auditoría independiente,
análisis formal ni pruebas de producción.

## Dictamen ejecutivo

**No-go para mainnet o valor real.** El contrato v0.1 desplegado en testnet es
inmutable y vulnerable a subpago por atribución parcial. El contrato v0.2
corrige los hallazgos on-chain principales y está compilado/probado localmente,
pero no está desplegado. Cloud continúa siendo una preview testnet sobre v0.1 y
carece de varios controles imprescindibles de producción.

No se encontró un administrador global ni una vía conocida para que un tercero
robe directamente campañas ajenas en el candidato v0.2. Sí se encontraron
fallas de integridad económica, concurrencia, custodia operativa, dependencias y
funcionalidades presentadas sin implementación real. Las de alta confianza se
detallan abajo.

## Hallazgos corregidos

### SD-CONTRACT-01 — Subpago mediante atribución parcial (alta)

- **Afectaba:** v0.1 desplegado y lógica inicial de `commit_tally`.
- **Explotación:** el owner podía comprometer `count=40` y acreditar sólo 30 al
  creador registrado. `settle` pagaba 30, aunque el total público decía 40.
- **Impacto:** pérdida económica del creador y ruptura de la propuesta central
  de atribución.
- **Corrección v0.2:** si existe `attributed_to`, la única entrada permitida
  debe pertenecer a esa dirección y su count debe ser exactamente el total.
  Existen pruebas para valores menores, mayores y dirección distinta.
- **Estado:** corregido en candidato; **no corregible en v0.1 desplegado**.

### SD-CONTRACT-02 — “Settlement automático” requería firma del owner (alta de diseño)

- **Afectaba:** v0.1 y documentación/UI.
- **Impacto:** un keeper no podía ejecutar el pago; automatizar exigía custodiar
  la clave del owner o solicitar una firma por período.
- **Corrección v0.2:** el owner aprueba al contrato como spender mediante el
  token SAC; cualquier fee payer puede llamar `settle(owner, ...)`. Se verifican
  allowance y balance, y `is_settled` permite evitar intentos duplicados.
- **Control recomendado:** allowance mínimo, con expiración corta y revocación
  del sobrante.

### SD-CONTRACT-03 — Interacción externa antes del guard de settlement (media)

- **Afectaba:** orden inicial de `settle`.
- **Riesgo:** superficie de reentrada/estado intermedio al llamar un contrato de
  token antes de guardar el estado settled.
- **Corrección:** checks-effects-interactions: el guard se persiste antes de
  cualquier llamada al token, incluidas `allowance`/`balance`; la atomicidad de
  Soroban lo revierte si un check o transferencia falla. Una prueba confirma
  que un fallo de allowance no deja el período bloqueado.

### SD-CONTRACT-04 — Índice de owner no acotado (media)

- **Afectaba:** `owner -> Vec<campaign_id>` reescrito en cada creación.
- **Impacto:** crecimiento de costo hasta bloquear owners prolíficos y reads sin
  límite.
- **Corrección:** slots O(1), `campaigns_page(cursor, limit<=100)`, helper legado
  `campaigns_of` sólo por compatibilidad y `bump_delegates` para TTL.

### SD-CLOUD-01 — Cobro posterior al efecto on-chain (alta)

- **Afectaba:** mutaciones metered.
- **Explotación:** agotar créditos o provocar carrera después de una transacción
  permitía acciones on-chain no cobradas.
- **Corrección:** reserva atómica mediante `UPDATE ... balance >= amount
  RETURNING`, refund en fallo y cobro parcial exacto para batches aplicados.
  La alerta de saldo bajo se emite sólo después de Commit/CommitUsed, nunca por
  una reserva luego reembolsada. Pruebas confirman concurrencia y alertas.

### SD-CLOUD-02 — Idempotencia no reservaba ni vinculaba parámetros (alta)

- **Afectaba:** reintentos/concurrencia de POST.
- **Explotación:** dos requests simultáneos con la misma key podían ejecutar; la
  key también podía reutilizarse con otro body/path/environment.
- **Corrección:** reserva previa, HMAC de método+URI+body, scope por org/env/
  endpoint, `409` para mismatch o in-progress, replay de status/body/content
  type y retención de 24 horas. La respuesta del handler se mantiene en buffer:
  sólo se reconoce al cliente después de persistir el replay durable. Si ese
  update falla, la key queda reservada/fail-closed y se devuelve `500`.

### SD-CLOUD-03 — Carreras y rewards duplicados/perdidos en loyalty (alta)

- **Afectaba:** punches simultáneos y cruce de thresholds.
- **Impacto:** rewards duplicados, faltantes o DB adelantada a una emisión fallida.
- **Corrección:** lock por programa, cálculo `floor(total/threshold)-existing`,
  máximo de 100 rewards por request, emisión on-chain antes del commit local y
  transacción local única para punch/receipt/rewards. Códigos CSPRNG de 12
  caracteres.

### SD-CLOUD-04 — Merkle roots sin receipts verificables (alta de integridad)

- **Afectaba:** implementación inicial de Tally/loyalty.
- **Impacto:** el root no permitía a un tercero reconstruir ni verificar hojas;
  la interfaz prometía una auditoría que no existía.
- **Corrección:** receipts JSON firmados Ed25519 por org/env, compromisos HMAC
  para referencias, hojas `SHA-256(payload)`, proofs publicados y endpoint
  público que verifica el count/root global y payload/firma/proof de cada página.
- **Límite explícito:** el signer sigue atestiguando que el evento real ocurrió.

### SD-CLOUD-05 — Manejo de llaves podía destruir o intercambiar identidad (alta)

- **Afectaba:** KEK local y ciphertexts de seeds.
- **Impacto:** un KEK corrupto podía ser reemplazado silenciosamente, haciendo
  irrecuperables cuentas existentes; un ciphertext cambiado no se contrastaba
  con la public key declarada.
- **Corrección:** longitud inválida falla cerrada, creación `O_EXCL`+0600+fsync,
  HMAC key separada de la KEK, signer de receipts separado del custodial y
  verificación `derived public key == stored public key` al descifrar.

### SD-CLOUD-06 — Webhooks/recharges presentados sin backend real (media/alta de diseño)

- **Afectaba:** promesas del console.
- **Corrección webhooks:** CRUD real, secret mostrado una vez y cifrado, HMAC
  timestamp+raw body, delivery IDs, hasta ocho reintentos y controles SSRF
  (HTTPS:443, sin redirects/proxy, resolución y dial sólo a IP pública).
- **Corrección recharges:** endpoint `501 Not Implemented`; ya no devuelve la
  propia cuenta custodial como falso destino de pago ni acredita saldo.

### SD-CLOUD-07 — Allowance innecesaria contra el contrato legado (media)

- **Afectaba:** el flujo Cloud preparaba el modelo v0.2 aunque ambos entornos
  siguen usando v0.1.
- **Riesgo:** v0.1 exige firma del owner y usa transferencia directa; no consume
  allowance. Cloud aprobaba igualmente al contrato por ~29 días, dejando una
  autoridad de gasto innecesaria tras el settlement.
- **Corrección:** Cloud fija explícitamente el ID legado y su flujo v0.1 no crea
  allowance. Migrar a v0.2 requiere un gate deliberado que apruebe el monto
  exacto del período inmediatamente antes del settlement.

### SD-CLOUD-08 — Auditoría pública con costo cuadrático y respuesta sin límite (media)

- **Afectaba:** `/v1/audit/tallies/...` reconstruía el árbol completo por cada
  proof y devolvía todos los receipts de un período en una sola respuesta.
- **Impacto:** un tally grande o requests públicos repetidos podían consumir CPU,
  memoria y ancho de banda de forma desproporcionada.
- **Corrección:** niveles Merkle construidos una sola vez, páginas de máximo 100,
  límite independiente de 30 requests/IP/min y máximo de 10,000 receipts por
  tally. El commit procesa el primer batch, reporta cuántos quedan y usa sufijos
  `YYYYWW01..99` para anclar inmediatamente batches adicionales de la semana.

### SD-AUTH-01 — Sesiones y abuso de endpoints (media)

- **Correcciones:** passwords 15–72 bytes (límite bcrypt), bearer de sesión
  hasheado, duración 8h/máximo 5, cookie HttpOnly/SameSite/Secure bajo TLS,
  verificación same-origin, login con dummy bcrypt y límites IP/cuenta, signup
  con límite IP, rate limits autenticados y headers HTTP defensivos.
- **Pendiente:** MFA, email verification, recovery y un proxy de confianza
  configurado explícitamente.

### SD-INPUT-01 — Bounds y conversiones inseguras (media)

- **Correcciones:** batch generado se limita antes de reservar memoria, JSON
  estricto/1 MiB, períodos ISO válidos, códigos/nombres acotados, enteros
  u32/u64/i128 exactos en ambos SDKs, sin wrap de i128, CSPRNG sin sesgo módulo y
  compromisos Go/TypeScript/Cloud con encoding interoperable. El cliente
  TypeScript ahora aplica tasa `0` cuando se omite el payout opcional, en lugar
  de fallar al codificar `undefined` como i128. Agregados monetarios usan
  `big.Int`/`BigInt`; el low-level browser rechaza un u64 que perdería precisión
  al convertirse a `Number`, y la UI convierte decimales a base-units sin float.

### SD-BIZ-01 — Archive/expiry no detenían nuevos eventos (media)

- **Afectaba:** Cloud aceptaba issue/redeem/shared events y loyalty punches aun
  cuando la campaña estaba archivada; también firmaba nuevos eventos Tally tras
  `valid_until`.
- **Corrección:** archive bloquea nuevas operaciones Cloud pero deja commits
  retrospectivos/settlements; expiry bloquea nuevos eventos y punches. El
  contrato sigue siendo la autoridad final para expiry de Burn.

### SD-BIZ-02 — Métrica `events_30d` no medía 30 días ni pendientes (baja)

- **Afectaba:** API y console mostraban una suma histórica bajo un nombre de 30
  días y la pantalla de settlement la interpretaba como trabajo sin commit.
- **Corrección:** campos separados `events_total` y `pending_events`, calculados
  según su significado; los toasts distinguen conversiones, receipts anclados y
  receipts restantes.

### SD-BIZ-03 — Referencias de negocio duplicables con otra idempotency key (media)

- **Afectaba:** el mismo `order_ref` podía volver a contarse usando una key HTTP
  nueva; loyalty no tenía una referencia de evento deduplicable.
- **Impacto:** inflar tallies/punches y, al cruzar thresholds, emitir rewards de
  más por reintentos mal implementados o abuso.
- **Corrección:** tabla de deduplicación con referencias HMAC por scope,
  inserción transaccional para shared events y `event_ref` opcional en loyalty.
  Duplicados devuelven `409`; idempotencia HTTP sigue siendo obligatoria para un
  retry seguro de toda la respuesta.

### SD-MIGRATION-01 — Datos previos conservaban sesiones/order refs inseguros (media)

- **Afectaba:** bases creadas antes de esta revisión: session bearer raw,
  `order_ref` en texto y eventos sin firma verificable.
- **Corrección:** migración versionada one-shot invalida sesiones, HMACa order
  refs y reconstruye su índice de deduplicación. Eventos históricos sin receipt
  se preservan con sentinel `committed_period=-1`, fuera de nuevos commits; no se
  inventan firmas retroactivas. Tallies legados sin set verificable responden
  `410 Gone`, y loyalty migra el hash antiguo a HMAC cuando reaparece el cliente.

### SD-SUPPLY-01 — Dependencias vulnerables (alta)

- **Correcciones:** Go 1.25.12 para advisories del stdlib, Soroban SDK 27.0.0,
  Stellar JS SDK 16.0.1 y Vite 6.4.3 donde aplicaba. Los lockfiles se mantienen.
- **RustSec:** 0 vulnerabilidades y dos warnings transitivos: `paste 1.0.15`
  sin mantenimiento y `rand 0.8.5` con RUSTSEC-2026-0097. Ambos llegan por el
  host/test tooling de Soroban; `cargo tree --target wasm32v1-none -i ...` no
  los incluye y el contrato no usa esas APIs directamente. Deben seguirse con
  futuras versiones del SDK, no silenciarse sin justificación.
- **Validación requerida por release:** repetir `govulncheck`, `npm audit`,
  `go mod verify`, Clippy y auditoría de crate dependencies en CI.

### SD-TRACE-01 — Hashes de transacción ficticiamente vacíos (media de diseño)

- **Afectaba:** columnas y enlaces `tx_hash` del Cloud console.
- **Impacto:** la UI prometía trazabilidad a Stellar, pero el SDK descartaba el
  hash confirmado y todos los enlaces quedaban vacíos.
- **Corrección:** el SDK conserva el hash de la última escritura exitosa y Cloud
  lo persiste/devuelve para campaign, shared registration, batches, redemption,
  tally, settlement y reward issuance. El client sigue siendo secuencial.

## Inventario de funcionalidades mock o no implementadas

| Elemento | Estado real después de la revisión |
|---|---|
| Campañas/códigos demo en memoria | Eliminados; playground inicia vacío y lee chain |
| Merkle root aleatorio | Eliminado; UI exige 32 bytes hex reales |
| Recibos auditables | Implementados en Cloud con firma/proofs |
| Webhooks | Implementados; requieren endpoint público HTTPS |
| Recarga card/USDC | No implementada; falla cerrada con 501 |
| `LIVE` | UI/keys nuevas usan METERED; `live` queda interno por compatibilidad |
| Webhook `livemode` | Siempre false; envelope declara testnet/production=false |
| Mainnet | No implementado |
| TTL keep-alive automático | No implementado; copy/cobro retirado |
| Auto-commit semanal | No implementado; commits son manuales |
| KMS / export de keys | No implementado; key files locales sólo para preview |
| Reward de lealtad editable pero ignorado | Corregido; `reward_type` y `reward_value` viajan por API y se validan/persisten |
| Datos decorativos del login | Marcados como ilustración sin tx/métrica ficticia |
| Footer `#` | Eliminado; muestra rutas locales y sólo enlaza al explorer real |
| Guías/copy con “trustless”, mainnet o E2E actual | Corregidos; `CLAUDE.md`, README y UI separan legado, candidato y planned |

## Riesgos residuales / bloqueadores de producción

1. **Contrato default legado.** SDK, playground y Cloud conservan v0.1 por
   compatibilidad hasta que exista un ID v0.2 real. Nunca usar con valor.
2. **No hay atomicidad distribuida chain+SQLite.** Si chain confirma y el update
   local falla, queda drift. Los mensajes ya lo admiten y los hashes ayudan a
   reconciliar, pero hace falta outbox/reconciler idempotente.
3. **Custodia local.** AES-GCM con key file no reemplaza KMS/HSM, rotación,
   separación de duties, backups ni recuperación.
4. **Un solo proceso.** Locks, rate limiter y worker de webhooks viven en
   memoria; un despliegue multi-instancia puede duplicar o competir.
5. **TTL y tallies manuales.** Sin scheduler/alertas, el estado puede archivarse
   o acumular eventos sin commitment.
6. **Políticas prometibles pero ausentes.** No hay cap acumulado para shared
   codes, once-per-user, geofence/KYC, `valid_from`, transferencia de tickets,
   refunds ni revenue split.
7. **Auth incompleto para producción.** Sin MFA, verificación de email, reset,
   recovery, RBAC/SSO ni revocación global de sesiones.
8. **Billing inexistente.** Los créditos son preview sin dinero real.
   Un refund que no pueda escribirse en SQLite sólo queda en logs; falta retry
   durable/reconciliación antes de cobrar dinero.
9. **Auditoría pública futura multi-red.** La ruta usa `chain_id/code/period`;
   antes de mainnet debe incluir contract/network para evitar ambigüedad.
10. **Artefacto binario versionado.** `cloud/api/sorodeal-cloud` es un build
    grande rastreado por Git; debe eliminarse del historial/árbol y generarse en
    CI con provenance, previa decisión del maintainer.
11. **Allowance SAC compartida por owner/token en v0.2.** No queda aislada por
    campaña. Si el owner aprueba un monto agregado, terceros pueden decidir qué
    obligación ya comprometida se liquida primero y agotar la allowance para
    otras. Mitigar con aprobación exacta por período o un vault/escrow con
    contabilidad por campaña.

## Validación ejecutada

- Contrato: `cargo fmt --check`, Clippy con warnings como errores y 34 pruebas
  unitarias pasaron. `stellar contract build` produjo 25 funciones, 31,245
  bytes y SHA-256
  `1c6c74f2f43c60aa06939d6e63c49a1809c98a7cebd9555453a4297c5f04c94b`;
  ABI Rust y binding TypeScript se regeneraron desde ese WASM.
- Rust dependencies: `cargo audit` encontró 0 vulnerabilidades y los dos
  warnings transitivos descritos en SD-SUPPLY-01; ninguno está en el grafo
  `wasm32v1-none`.
- Go Cloud/SDK: `go test`, `go vet` y `go test -race` pasaron; `go mod verify`
  verificó todos los módulos. `govulncheck` no encontró llamadas vulnerables
  (Cloud reporta 15 advisories en módulos requeridos, ninguno alcanzable).
- Consumer E2E Go: compila con `go test ./...`, pero no contiene unit tests y no
  se ejecutó contra red en esta revisión.
- TypeScript/React: SDK (`tsc`), playground y console (`vite build`) pasaron con
  Node 22. Los tres `npm audit --audit-level=low` reportaron 0 vulnerabilidades.
  El smoke test confirmó el vector Go/TS del commitment y los límites i128.
- Integridad: `git diff --check`, JSON del candidato, hash y tamaño del WASM
  coinciden. El playground conserva un warning no bloqueante por un chunk
  minificado de ~563 kB.
- QA visual: el navegador integrado no estuvo disponible en esta sesión; no se
  declara una revisión visual interactiva. Los builds de producción sí pasaron.

No se desplegó contrato ni se ejecutó ninguna escritura externa en testnet o
mainnet.

## Criterio mínimo de release

- desplegar v0.2 con autorización explícita y verificar hash/ABI on-chain;
- E2E live de owner, delegate, stranger, exact tally, allowance y keeper;
- audit externo independiente del contrato y threat model Cloud;
- KMS/HSM, MFA/recovery, reconciler/outbox y operación multi-instancia;
- cron TTL/tally con observabilidad y dry-run;
- billing real sólo después de confirmar pagos de forma verificable;
- backup/restore probado, rotación, alertas, incident runbook y retención de datos.
