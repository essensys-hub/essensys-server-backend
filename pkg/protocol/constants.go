package protocol

const (
	// IndexScenario is the scenario trigger index
	IndexScenario = 590
	// IndexLightStart is the first light/shutter index
	IndexLightStart = 605
	// IndexLightEnd is the last light/shutter index
	IndexLightEnd = 622
	// IndexSecurityOutlets is Scenario_Securite of scenario 1 (offset 31 of the
	// 592..632 block): 0 = nothing, 1 = cut, 2 = restore.
	IndexSecurityOutlets = 623
	// Light auto-off timers (minutes, 0..255, 0 = never): Lampes_*_Temps cells.
	// lumieres-temporisees-2026-11-003. Outside 605..623: no block completion.
	IndexLightTimerDressing  = 521
	IndexLightTimerWC1       = 555
	IndexLightTimerWC2       = 556
	IndexLightTimerPieceServ = 557
	IndexLightTimerAnnexe1   = 561
	IndexLightTimerAnnexe2   = 562
	// MaxExchangeIndex is the maximum valid index
	MaxExchangeIndex = 999
	// MaxParamsPerFirmwareAction is the BP_MQX_ETH limit (Json.c / sc_TraitementAction).
	MaxParamsPerFirmwareAction = 30
)
