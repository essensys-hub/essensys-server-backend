package scenario_test

import (
	"testing"

	"github.com/essensys-hub/essensys-server-backend/internal/scenario"
	"github.com/essensys-hub/essensys-server-backend/pkg/protocol"
)

func TestSlotBaseIndex(t *testing.T) {
	tests := []struct {
		slot int
		want int
	}{
		{1, 592},
		{2, 633},
		{8, 879},
	}
	for _, tt := range tests {
		got, err := scenario.SlotBaseIndex(tt.slot)
		if err != nil {
			t.Fatalf("slot %d: %v", tt.slot, err)
		}
		if got != tt.want {
			t.Errorf("slot %d base = %d, want %d", tt.slot, got, tt.want)
		}
	}
}

func TestAbsoluteIndex_AllumerCHBLSB(t *testing.T) {
	got, err := scenario.AbsoluteIndex(1, scenario.OffsetAllumerCHBLSB)
	if err != nil {
		t.Fatal(err)
	}
	if got != 613 {
		t.Errorf("Allumer CHB LSB = %d, want 613", got)
	}
}

func TestLaunchParams_JeSors(t *testing.T) {
	params, err := scenario.LaunchParams(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 1 || params[0].K != 590 || params[0].V != "2" {
		t.Fatalf("got %+v", params)
	}
	if !scenario.IsMemorizedLaunch(params) {
		t.Error("expected memorized launch")
	}
}

func TestLaunchParams_RejectsSlot1(t *testing.T) {
	if _, err := scenario.LaunchParams(1); err == nil {
		t.Error("expected error for slot 1")
	}
}

func TestExpandModeB_lightBlock(t *testing.T) {
	in := []protocol.ExchangeKV{{K: 613, V: "64"}}
	out := scenario.ExpandModeB(in, false)
	byK := map[int]string{}
	for _, p := range out {
		byK[p.K] = p.V
	}
	if byK[590] != "1" {
		t.Errorf("590 = %q", byK[590])
	}
	for i := 605; i <= 622; i++ {
		if _, ok := byK[i]; !ok {
			t.Errorf("missing index %d", i)
		}
	}
	if byK[613] != "64" {
		t.Errorf("613 = %q", byK[613])
	}
}

func TestExpandModeB_fullBlock(t *testing.T) {
	in := []protocol.ExchangeKV{{K: 593, V: "1"}}
	out := scenario.ExpandModeB(in, true)
	byK := map[int]string{}
	for _, p := range out {
		byK[p.K] = p.V
	}
	for i := 592; i <= 632; i++ {
		if _, ok := byK[i]; !ok {
			t.Errorf("missing index %d", i)
		}
	}
}

func TestWriteDefinitionChunks_twoActions(t *testing.T) {
	chunks, err := scenario.WriteDefinitionChunks(7, map[int]string{838: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks for 41 params, got %d", len(chunks))
	}
	if len(chunks[0]) != 30 || len(chunks[1]) != 11 {
		t.Errorf("chunk sizes: %d + %d", len(chunks[0]), len(chunks[1]))
	}
	for _, chunk := range chunks {
		for _, p := range chunk {
			if p.K == 590 {
				t.Error("definition chunks must not include trigger 590")
			}
		}
	}
}

func TestWriteDefinitionChunks_rejectsSlot1(t *testing.T) {
	_, err := scenario.WriteDefinitionChunks(1, map[int]string{592: "0"})
	if err != scenario.ErrSlot1ServerReserved {
		t.Errorf("got %v", err)
	}
}

func TestRestorePresetParams(t *testing.T) {
	params, err := scenario.RestorePresetParams(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(params) != 1 || params[0].K != 673 || params[0].V != "2" {
		t.Fatalf("got %+v", params)
	}
}

func TestValidateDefinition_rejectsOutOfRange(t *testing.T) {
	err := scenario.ValidateDefinition(2, map[int]string{592: "1"})
	if err == nil {
		t.Error("expected error for index outside slot 2")
	}
}

func TestValidateDefinition_rejectsOverflow(t *testing.T) {
	err := scenario.ValidateDefinition(2, map[int]string{633: "256"})
	if err == nil {
		t.Error("expected error for value > 255")
	}
}
