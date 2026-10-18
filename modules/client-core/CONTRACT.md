# CONTRACT — `client-core`

interface PlatformAdapter { id(): string; signedAuth(): Promise<AuthPayload>; purchase(sku): Promise<Receipt>; share(p): Promise<void>; storage: KV; lifecycle: Emitter }.

_Изменения контракта — отдельным PR с меткой `contract-change`; версия контракта указывается ниже._

**Версия контракта:** 0.1
