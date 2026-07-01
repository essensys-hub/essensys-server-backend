package armoire

// Dashboard exchange-table index groups (firmware limit: 30 indices per serverinfos cycle).
// See essensys-doc/archi/exchange-table.md and TableEchange.h (099-37).

// DefaultCommandIndices are polled on the default rotation slot (inject / shutter config).
var DefaultCommandIndices = []int{
	613, 607, 615, 590, 349, 350, 351, 352, 363, 425, 426, 920,
	566, 567, 568, 569, 570, 571, 572,
	574, 575, 576, 577, 578,
	582, 583, 584, 585,
}

// IdentityIndices: firmware version, RTC, Ethernet health, MAC.
var IdentityIndices = []int{
	0, 1, 5, 6, 7, 8, 9,
	945,
	947, 948, 949, 950, 951, 952,
}

// HealthIndices: system status, alarm state, comm faults.
var HealthIndices = []int{
	10, 11, 12,
	408, 413, 414, 415,
	920,
}

// ComfortEnergyIndices: heating, cumulus, sprinkler, scenario, Linky, wind.
var ComfortEnergyIndices = []int{
	349, 350, 351, 352, 353,
	363, 459, 591,
	460, 461, 463, 464,
	940,
}

// SnapshotIndices is the union of dashboard-readable keys (excludes scenario light masks 605–616).
var SnapshotIndices = []int{
	0, 1, 5, 6, 7, 8, 9,
	10, 11, 12,
	349, 350, 351, 352, 353,
	363, 408, 413, 414, 415, 459, 591,
	460, 461, 463, 464,
	920, 940, 945,
	947, 948, 949, 950, 951, 952,
}
