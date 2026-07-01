package armoire

import (
	"fmt"
	"strconv"
	"strings"
)

func parseByte(v string) (byte, bool) {
	if v == "" {
		return 0, false
	}
	if strings.Contains(v, "0") && strings.Contains(v, "1") && len(v) == 8 {
		var n byte
		for i := 0; i < 8; i++ {
			if v[i] == '1' {
				n |= 1 << uint(i)
			}
		}
		return n, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 255 {
		return 0, false
	}
	return byte(n), true
}

type statusFlags struct {
	HeuresCreuses bool
	Delestage     bool
	Secouru       bool
}

func decodeStatusByte(v string) (statusFlags, bool) {
	b, ok := parseByte(v)
	if !ok {
		return statusFlags{}, false
	}
	return statusFlags{
		HeuresCreuses: b&0x01 != 0,
		Delestage:     b&0x02 != 0,
		Secouru:       b&0x04 != 0,
	}, true
}

type alerteFlags struct {
	Alarme  bool
	FuiteLL bool
	FuiteLV bool
}

func decodeAlerteByte(v string) (alerteFlags, bool) {
	b, ok := parseByte(v)
	if !ok {
		return alerteFlags{}, false
	}
	return alerteFlags{
		Alarme:  b&0x01 != 0,
		FuiteLL: b&0x02 != 0,
		FuiteLV: b&0x04 != 0,
	}, true
}

func decodeInformationFaults(v string) string {
	b, ok := parseByte(v)
	if !ok || b == 0 {
		return ""
	}
	var faults []string
	labels := []struct {
		mask byte
		name string
	}{
		{0x01, "compteur ERDF"},
		{0x02, "IHM"},
		{0x04, "BA PDV"},
		{0x08, "BA CHB"},
		{0x10, "BA PDE"},
	}
	for _, l := range labels {
		if b&l.mask != 0 {
			faults = append(faults, l.name)
		}
	}
	return strings.Join(faults, ", ")
}

func decodeEthernetState(v string) string {
	b, ok := parseByte(v)
	if !ok {
		return "inconnu"
	}
	// Firmware www.c : bit=0 OK, bit=1 défaut (HS) pour câble, DHCP/IP, DNS, serveur.
	var okParts, faultParts []string
	if b&0x01 == 0 {
		okParts = append(okParts, "câble OK")
	} else {
		faultParts = append(faultParts, "câble déconnecté")
	}
	if b&0x02 == 0 {
		okParts = append(okParts, "IP OK")
	} else {
		faultParts = append(faultParts, "DHCP/IP HS")
	}
	if b&0x04 == 0 {
		okParts = append(okParts, "DNS OK")
	} else {
		faultParts = append(faultParts, "DNS HS")
	}
	if b&0x08 == 0 {
		okParts = append(okParts, "serveur joignable")
	} else {
		faultParts = append(faultParts, "serveur HS")
	}
	if len(faultParts) > 0 {
		return strings.Join(faultParts, ", ")
	}
	return strings.Join(okParts, ", ")
}

func decodeAlarmDetection(v string) string {
	b, ok := parseByte(v)
	if !ok || b == 0 {
		return ""
	}
	var parts []string
	if b&0x01 != 0 {
		parts = append(parts, "ouverture")
	}
	if b&0x02 != 0 {
		parts = append(parts, "présence 1")
	}
	if b&0x04 != 0 {
		parts = append(parts, "présence 2")
	}
	return strings.Join(parts, ", ")
}

func decodeAlarmFraud(v string) string {
	b, ok := parseByte(v)
	if !ok || b == 0 {
		return ""
	}
	var parts []string
	labels := []struct {
		mask byte
		name string
	}{
		{0x01, "tableau"},
		{0x02, "IHM"},
		{0x04, "détecteur"},
		{0x08, "sirène"},
		{0x40, "batterie"},
	}
	for _, l := range labels {
		if b&l.mask != 0 {
			parts = append(parts, l.name)
		}
	}
	return strings.Join(parts, ", ")
}

func decodeBPState(v string) string {
	b, ok := parseByte(v)
	if !ok {
		return ""
	}
	var parts []string
	if b&0x01 != 0 {
		parts = append(parts, "alarme activée")
	}
	if b&0x02 != 0 {
		parts = append(parts, "alarme déclenchée")
	}
	return strings.Join(parts, ", ")
}

func heatingModeLabel(v string) (consigne, mode string) {
	b, ok := parseByte(v)
	if !ok {
		return "inconnu", "inconnu"
	}
	consignes := []string{"OFF", "CONFORT", "ECO", "ECO+", "ECO++", "HORS GEL"}
	if int(b&0x0F) < len(consignes) {
		consigne = consignes[b&0x0F]
	} else {
		consigne = fmt.Sprintf("0x%02X", b&0x0F)
	}
	switch (b >> 4) & 0x03 {
	case 0:
		mode = "automatique"
	case 1:
		mode = "forcé"
	case 2:
		mode = "anticipé"
	default:
		mode = "inconnu"
	}
	if b&0x40 != 0 {
		mode = "reprendre mémorisé"
	}
	if b&0x80 != 0 {
		mode = "continuer actuel"
	}
	return consigne, mode
}

func alarmModeLabel(v string) string {
	b, ok := parseByte(v)
	if !ok {
		return "inconnu"
	}
	labels := map[byte]string{
		0x00: "non utilisée",
		0x01: "réglage",
		0x02: "indépendante",
		0x03: "scénario Je sors",
		0x04: "scénario Je vais me coucher",
		0x05: "scénario Je pars en vacances",
		0x06: "scénario personnalisé",
	}
	if l, ok := labels[b]; ok {
		return l
	}
	return fmt.Sprintf("0x%02X", b)
}

func alarmStepLabel(v string) string {
	b, ok := parseByte(v)
	if !ok {
		return "inconnu"
	}
	labels := map[byte]string{
		0x00: "départ",
		0x01: "pb alimentation",
		0x02: "pb intrusion/vandalisme",
		0x03: "procédure de sortie",
		0x04: "croisière",
		0x05: "procédure d'entrée",
		0x06: "intrusion",
	}
	if l, ok := labels[b]; ok {
		return l
	}
	return fmt.Sprintf("0x%02X", b)
}

func scenarioLabel(v string) string {
	b, ok := parseByte(v)
	if !ok || b == 0 {
		return "aucun"
	}
	names := []string{
		"", "serveur", "Je sors", "Je pars en vacances", "Je rentre",
		"Je vais me coucher", "Je me lève", "Personnalisé 1", "Personnalisé 2",
	}
	if int(b) < len(names) && names[b] != "" {
		return names[b]
	}
	return fmt.Sprintf("scénario %d", b)
}

func cumulusLabel(v string) string {
	b, ok := parseByte(v)
	if !ok {
		return "inconnu"
	}
	switch b {
	case 0:
		return "autonome (ON)"
	case 1:
		return "gestion HC"
	case 2:
		return "OFF"
	default:
		return fmt.Sprintf("0x%02X", b)
	}
}

func sprinklerLabel(v string) string {
	b, ok := parseByte(v)
	if !ok {
		return "inconnu"
	}
	switch {
	case b == 0:
		return "OFF"
	case b == 255:
		return "automatique"
	case b >= 1 && b <= 254:
		return fmt.Sprintf("forcé %d min", b)
	default:
		return fmt.Sprintf("0x%02X", b)
	}
}

func linkyTariffLabel(v string) string {
	b, ok := parseByte(v)
	if !ok || b == 0 || b == 0xFF {
		return ""
	}
	labels := map[byte]string{
		2: "BASE", 3: "HC/HP", 4: "EJP", 5: "Tempo",
	}
	if l, ok := labels[b]; ok {
		return l
	}
	return fmt.Sprintf("tarif %d", b)
}

func linkyPeriodLabel(v string) string {
	b, ok := parseByte(v)
	if !ok || b == 0 || b == 0xFF {
		return ""
	}
	labels := map[byte]string{
		3: "HC", 4: "HP", 5: "HN", 6: "PM",
	}
	if l, ok := labels[b]; ok {
		return l
	}
	return fmt.Sprintf("période %d", b)
}

func apparentPowerVA(lsb, msb string) *int {
	l, lok := parseByte(lsb)
	m, mok := parseByte(msb)
	if !lok && !mok {
		return nil
	}
	if l == 0xFF && m == 0xFF {
		return nil
	}
	v := int(l) | int(m)<<8
	return &v
}

func formatMAC(parts ...string) string {
	var hex []string
	for _, p := range parts {
		b, ok := parseByte(p)
		if !ok {
			continue
		}
		hex = append(hex, fmt.Sprintf("%02X", b))
	}
	if len(hex) != 6 {
		return ""
	}
	return strings.ToLower(strings.Join(hex, ":"))
}

func parseIntDecimal(v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, fmt.Errorf("empty")
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	return n, err
}
