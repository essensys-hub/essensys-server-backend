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
	// MaxExchangeIndex is the maximum valid index
	MaxExchangeIndex = 999
	// MaxParamsPerFirmwareAction is the BP_MQX_ETH limit (Json.c / sc_TraitementAction).
	MaxParamsPerFirmwareAction = 30
)
