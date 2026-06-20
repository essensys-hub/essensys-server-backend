package cloudsync

import "github.com/essensys-hub/essensys-server-backend/internal/core"

const scenariosProfileName = "Scénarios"

// filterExcludedIndices removes keys listed in exclude (e.g. trigger 590).
func filterExcludedIndices(indices []int, exclude []int) []int {
	if len(exclude) == 0 {
		return indices
	}
	blocked := make(map[int]struct{}, len(exclude))
	for _, k := range exclude {
		blocked[k] = struct{}{}
	}
	out := make([]int, 0, len(indices))
	for _, k := range indices {
		if _, skip := blocked[k]; skip {
			continue
		}
		out = append(out, k)
	}
	return out
}

// pushIndicesFromProfilesList unions push keys from enabled profiles with PushToCloud.
func pushIndicesFromProfilesList(profiles []cloudSyncProfile) []int {
	seen := make(map[int]struct{})
	var out []int
	for _, p := range profiles {
		if !p.Enabled || !p.PushToCloud {
			continue
		}
		keys := filterExcludedIndices(core.FlattenIndexRanges(p.IndexRanges), p.ExcludeIndices)
		for _, k := range keys {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

func findScenariosProfile(profiles []cloudSyncProfile) *cloudSyncProfile {
	for i := range profiles {
		if profiles[i].Name == scenariosProfileName {
			return &profiles[i]
		}
	}
	return nil
}
