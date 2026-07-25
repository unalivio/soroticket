# Oportunidades de producto para Soroticket

Estado: ideas de producto, **no funcionalidades implementadas**. La prioridad
considera cuánto reutilizan los primitives actuales (Burn, Tally, recibos,
settlement, webhooks y loyalty) y cuánto contrato nuevo requieren.

## Recomendación de secuencia

| Prioridad | Producto | Qué reutiliza | Falta construir | Modelo comercial |
|---|---|---|---|---|
| 1 | Referidos y afiliados | Tally, atribución exacta, receipts, settlement | Anti-fraude, reversos, ventana de maduración y panel del afiliado | comisión por conversión + plan SaaS |
| 2 | Membresías y pases | Burn para credencial, campañas y verificación | renovación, suspensión, roles y control de acceso | por miembro activo / mensualidad |
| 3 | Gift cards y vouchers regalables | códigos únicos, QR, verificación, webhooks | saldo parcial, recarga, refund y transferencia segura | emisión + uso + white label |
| 4 | Loyalty entre varios comercios | loyalty, HMAC de cliente, settlement | ledger de puntos compartido, reglas/cambios y clearing | fee por comercio + clearing |
| 5 | Tickets con inventario | Burn y canje en puerta | asientos, propiedad/transferencia, reventa y reemisión | fee por entrada |

Los tres primeros que probaría comercialmente son **referidos/afiliados**,
**membresías** y **vouchers regalables**: tienen compradores claros y aprovechan
la infraestructura actual sin exigir primero una economía completa de puntos.

## 1. Referidos y afiliados verificables

Cada creador, vendedor o cliente recibe un código; Soroticket publica receipts y
ancla el total, espera una ventana de devolución y liquida el monto aprobado.

Valor diferencial: el afiliado puede verificar inclusión y términos de pago,
pero la documentación debe mantener el límite correcto: el merchant/signer aún
atestigua que la venta es real.

Antes de venderlo hacen falta:

- estados `pending`, `approved`, `reversed` y `paid`;
- ventana de maduración para devoluciones/chargebacks;
- hacer obligatorio el `order_ref` ya deduplicado, agregar lifecycle de orden y
  detección de auto-referidos;
- límites de payout y aprobación manual para anomalías;
- portal de sólo lectura para el afiliado.

## 2. Membresías, abonos y pases de acceso

Un código/credencial representa una membresía vigente: gimnasio, coworking,
club, transporte, eventos recurrentes o contenido premium. El lector consulta
vigencia y registra accesos por webhook.

Extensión necesaria: un entitlement con `valid_from`, renovación, suspensión y
revocación. No conviene simularlo creando tickets infinitos; debe existir un
estado de membresía explícito y auditable.

## 3. Gift cards, saldo promocional y vouchers regalables

Dos niveles:

- MVP seguro: voucher de valor fijo, canje total una sola vez (Burn actual);
- producto completo: saldo parcial, múltiples consumos, recarga y devolución.

El segundo requiere un contrato/ledger de saldo nuevo, conciliación contable y
reglas de transferencia. También puede activar regulación de dinero
almacenado, vencimiento y fondos no reclamados según el país; necesita revisión
legal antes de mainnet.

## 4. Loyalty de coalición

Varios comercios comparten un identificador opaco de cliente y una unidad de
puntos. Un comercio emite, otro acepta y un proceso de clearing liquida entre
participantes.

Es una evolución natural del loyalty actual, pero requiere:

- ledger de puntos por cliente (no sólo totales agregados);
- reglas versionadas de earn/burn y límites;
- consentimiento y separación de datos entre comercios;
- reservas, disputas y settlement neto entre miembros;
- gobernanza sobre inflación y salida de un comercio.

## 5. Ticketing profesional

Agregar inventario de asientos, categorías, check-in offline con sincronización,
transferencia controlada, reemisión por pérdida y reglas de reventa. Burn ya
resuelve el uso único, pero **no** modela propietario ni transferencia; esas
capacidades deben ser una extensión explícita, no una promesa de UI.

## 6. Cashback y rebates posteriores a compra

El usuario registra una compra; tras validación y ventana antifraude recibe un
voucher o token. Reutiliza receipts, Tally, webhooks y settlement. Es atractivo
para CPG, distribuidores y campañas con retailers donde el emisor no controla
el POS.

Primitivas faltantes: evidencia adjunta fuera de cadena, revisión/score,
deduplicación fuerte de factura, reversos y lista de assets permitidos.

## 7. Garantías y pasaportes de producto

Una credencial acompaña un equipo, repuesto o artículo premium y enlaza compra,
vigencia, reparaciones y transferencia. Útil para electrónica, maquinaria,
lujo y repuestos auténticos.

Debe evitar PII on-chain; documentos y seriales sensibles quedan cifrados fuera
de cadena, con sólo compromisos y eventos mínimos en Stellar.

## 8. Depósitos retornables y reciclaje

Emitir una unidad por envase/activo retornable; al devolverlo se quema y se
otorga cashback/puntos. Aplicable a botellas, pallets, baterías y equipos en
préstamo. Requiere identidad del activo (QR/NFC), operadores delegados y
controles contra clonación física del tag.

## 9. Beneficios corporativos y subsidios dirigidos

Empresas u ONG asignan vouchers por categoría, período y comercio: alimentos,
transporte, salud, formación. Burn resuelve canje único; una variante con
presupuesto recurrente necesita políticas, límites y reportes de privacidad.

Es un producto sensible: no debe inferir diagnóstico, salario u otra condición
personal desde datos públicos.

## 10. Credenciales y certificados verificables

Asistencia a cursos, certificaciones cortas, voluntariado o acceso a una
comunidad. Se parece a un entitlement, no a un cupón; necesitaría revocación,
emisor verificable y posiblemente estándares de credenciales fuera de
Soroticket. Puede venderse como módulo white-label a academias y eventos.

## Capacidades horizontales que multiplican todos los productos

- constructor de reglas versionadas con simulación antes de publicar;
- widgets/QR white-label y SDK móvil/POS offline;
- portal público de receipts, proofs y settlement para terceros;
- importación/exportación y reconciliación con Shopify, WooCommerce y POS;
- alertas antifraude, límites, aprobaciones y reversos;
- analytics de cohortes sin exponer PII;
- marketplace de plantillas de campaña;
- administración de equipos/RBAC, auditoría interna y SSO para planes Pro.

## Regla de producto recomendada

Cada nueva tarjeta o botón del console debe estar clasificado como
`implemented`, `preview/testnet` o `planned`. Si una acción depende de pagos,
oráculos, KMS, cron o mainnet no configurados, debe fallar cerrada y mostrar el
prerrequisito; nunca devolver una dirección, transacción, receipt o balance
inventado.
