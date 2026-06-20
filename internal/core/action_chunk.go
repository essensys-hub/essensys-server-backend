package core

import "github.com/essensys-hub/essensys-server-backend/pkg/protocol"

func chunkExchangeParams(params []protocol.ExchangeKV, max int) [][]protocol.ExchangeKV {
	if max <= 0 || len(params) <= max {
		if len(params) == 0 {
			return nil
		}
		return [][]protocol.ExchangeKV{params}
	}
	out := make([][]protocol.ExchangeKV, 0, (len(params)+max-1)/max)
	for i := 0; i < len(params); i += max {
		end := i + max
		if end > len(params) {
			end = len(params)
		}
		out = append(out, params[i:end])
	}
	return out
}
